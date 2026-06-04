// Semantic-Map embedded UI controller.
//
// Vanilla JS, no framework. Cytoscape.js owns the graph state; the only
// extra side-state is the currently selected edge (selectedEdge below) so
// the side panel and modal know what they are mutating.
//
// Data flow:
//   loadAll() → GET /graph + GET /history → render cy + cache state
//   click edge → renderPanel(edge) + filter cached history client-side
//   modal submit → POST → 204 → toast → loadAll() → cy redraws
//
// Endpoint contract is documented in di-agent/semantic-map/go/cmd/agent/dto.go
// and the Phase 1 routes.go.
//
// TODO(multi-agent v2): add a Peers panel that lists GET /peers, supports
// add/remove/trust-override, and surfaces LastSeen so operators can see live
// coordination state. Deferred from the v1 multi-agent change to keep the
// scope tight — the mapctl CLI covers the same surface in the meantime.

(function () {
  'use strict';

  // ── Module state ────────────────────────────────────────────────────────

  let cy = null;                  // cytoscape instance
  let cachedGraph = null;         // last GET /graph payload
  let cachedHistory = [];         // last GET /history payload
  let selectedEdge = null;        // EdgeDTO currently shown in panel (edge mode)
  let selectedNodeID = null;      // ConstructID currently shown in panel (node mode)
  let autoRefreshHandle = null;   // setInterval id when auto-refresh is on
  let showingCandidates = false;  // whether the candidates panel is currently visible

  // ── DOM refs ────────────────────────────────────────────────────────────

  const $ = (id) => document.getElementById(id);
  const els = {
    healthDot:   $('health-dot'),
    healthLabel: $('health-label'),
    refreshBtn:  $('refresh-btn'),
    autoRefresh: $('auto-refresh'),

    panelEmpty: $('panel-empty'),
    panelNode:  $('panel-node'),
    panelEdge:  $('panel-edge'),

    // Node-panel fields
    ndID:       $('nd-id'),
    ndName:     $('nd-name'),
    ndDesc:     $('nd-desc'),
    ndDegree:   $('nd-degree'),
    ndOutCount: $('nd-out-count'),
    ndOutList:  $('nd-out-list'),
    ndInCount:  $('nd-in-count'),
    ndInList:   $('nd-in-list'),

    edDescription: $('ed-description'),
    edFrom:        $('ed-from'),
    edTo:          $('ed-to'),
    edPid:         $('ed-pid'),
    edDir:         $('ed-dir'),
    edPrior:       $('ed-prior'),
    edEma:         $('ed-ema'),
    edConf:        $('ed-conf'),
    edNobs:        $('ed-nobs'),
    edDeprecated:  $('ed-deprecated'),

    historyList: $('history-list'),

    btnStrength:  $('btn-strength'),
    btnDeprecate: $('btn-deprecate'),
    btnReset:     $('btn-reset'),

    modal:         $('modal'),
    modalForm:     $('modal-form'),
    modalTitle:    $('modal-title'),
    modalSubtitle: $('modal-subtitle'),
    modalStrength: $('modal-strength-input'),
    modalReason:   $('modal-reason-input'),
    modalResetFrom:$('modal-reset-from'),
    modalResetTo:  $('modal-reset-to'),
    modalCancel:   $('modal-cancel'),
    modalSubmit:   $('modal-submit'),

    toasts: $('toasts'),

    candidatesChip: $('candidates-chip'),
    panelCandidates: $('panel-candidates'),
    candidatesList: $('candidates-list'),
  };

  // ── HTTP helpers ────────────────────────────────────────────────────────

  async function getJSON(path) {
    const resp = await fetch(path, { headers: { 'Accept': 'application/json' } });
    if (!resp.ok) {
      let msg = `${path} → HTTP ${resp.status}`;
      try { const er = await resp.json(); if (er && er.error) msg = er.error; } catch (_) {}
      throw new Error(msg);
    }
    return resp.json();
  }

  async function postJSON(path, body) {
    // The server's requireJSON guard rejects any POST without this header,
    // so it must be set explicitly on every mutation call.
    const resp = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (resp.status === 204) return null;
    if (!resp.ok) {
      let msg = `${path} → HTTP ${resp.status}`;
      try { const er = await resp.json(); if (er && er.error) msg = er.error; } catch (_) {}
      throw new Error(msg);
    }
    // 2xx with body — uncommon for our mutation endpoints but handled
    // gracefully so future endpoints don't surprise us.
    try { return await resp.json(); } catch (_) { return null; }
  }

  // ── Toasts ──────────────────────────────────────────────────────────────

  function toast(message, kind) {
    const node = document.createElement('div');
    node.className = 'toast' + (kind === 'error' ? ' toast-error' : kind === 'success' ? ' toast-success' : '');
    node.textContent = message;
    els.toasts.appendChild(node);
    // Defer adding the visible class so the transition runs.
    requestAnimationFrame(() => node.classList.add('toast-visible'));
    setTimeout(() => {
      node.classList.remove('toast-visible');
      setTimeout(() => node.remove(), 250);
    }, 4000);
  }

  // ── Health polling (lightweight, used on Refresh) ───────────────────────

  async function refreshHealth() {
    try {
      const h = await getJSON('/healthz');
      if (h && h.ok) {
        els.healthDot.className = 'health-dot health-ok';
        els.healthLabel.textContent = 'healthy';
      } else {
        els.healthDot.className = 'health-dot health-bad';
        els.healthLabel.textContent = 'unhealthy';
      }
    } catch (e) {
      els.healthDot.className = 'health-dot health-bad';
      els.healthLabel.textContent = 'unreachable';
    }
  }

  // ── Cytoscape setup ─────────────────────────────────────────────────────

  function cytoscapeStyle() {
    return [
      {
        selector: 'node',
        style: {
          'background-color': '#3e4c59',
          'label': 'data(label)',
          'color': '#1f2933',
          'text-valign': 'center',
          'text-halign': 'center',
          'text-margin-y': -22,
          'font-size': 11,
          'font-weight': 600,
          'width': 38,
          'height': 38,
          'border-color': '#1f2933',
          'border-width': 1,
        },
      },
      {
        // Edge styling per plan §"Cytoscape edge style":
        //   - line-color green for "+", red for "-"
        //   - opacity 0.3 + 0.7 * confidence
        //   - line-style dashed if deprecated
        //   - target-arrow-shape triangle
        selector: 'edge',
        style: {
          'curve-style': 'bezier',
          'control-point-step-size': 60,
          'line-color': 'mapData(directionNum, 0, 1, #2eb872, #e63946)',
          'target-arrow-color': 'mapData(directionNum, 0, 1, #2eb872, #e63946)',
          'target-arrow-shape': 'triangle',
          'arrow-scale': 1.1,
          'width': 2.5,
          'opacity': 'data(opacity)',
          'line-style': 'data(lineStyle)',
          'label': 'data(label)',
          'font-size': 9,
          'color': '#52606d',
          'text-background-color': '#ffffff',
          'text-background-opacity': 0.85,
          'text-background-padding': 2,
          'text-rotation': 'autorotate',
        },
      },
      {
        selector: 'edge:selected',
        style: {
          'width': 4.5,
          'opacity': 1.0,
          'overlay-color': '#1f2933',
          'overlay-opacity': 0.08,
          'overlay-padding': 4,
        },
      },
    ];
  }

  function buildElements(graph) {
    const elements = [];
    for (const c of graph.constructs || []) {
      elements.push({
        group: 'nodes',
        data: { id: c.construct_id, label: c.construct_id, name: c.name, description: c.description },
      });
    }
    for (const e of graph.edges || []) {
      const opacity = Math.max(0.1, Math.min(1.0, 0.3 + 0.7 * (e.confidence ?? 0)));
      // Match the edge to its proposition to know if it is deprecated.
      const prop = (graph.propositions || []).find((p) => p.proposition_id === e.proposition_id);
      const deprecated = !!(prop && prop.deprecated);
      elements.push({
        group: 'edges',
        // Cytoscape requires unique edge ids. Propositions are unique per
        // edge in the v1 ontology, so we use the proposition_id directly.
        data: {
          id: e.proposition_id,
          source: e.from,
          target: e.to,
          label: e.proposition_id,
          edge: e,
          deprecated: deprecated,
          directionNum: e.direction === '-' ? 1 : 0,
          opacity: opacity,
          lineStyle: deprecated ? 'dashed' : 'solid',
        },
      });
    }
    return elements;
  }

  function renderGraph(graph) {
    if (cy) {
      cy.elements().remove();
      cy.add(buildElements(graph));
      cy.layout({ name: 'cose', animate: false, padding: 30 }).run();
    } else {
      cy = cytoscape({
        container: document.getElementById('cy'),
        elements: buildElements(graph),
        style: cytoscapeStyle(),
        layout: {
          name: 'cose',
          animate: false,
          padding: 30,
          nodeRepulsion: 8000,
          idealEdgeLength: 110,
          edgeElasticity: 100,
        },
        wheelSensitivity: 0.25,
        minZoom: 0.4,
        maxZoom: 2.5,
      });

      cy.on('tap', 'edge', (evt) => {
        const edge = evt.target.data('edge');
        if (!edge) return;
        selectEdge(edge);
      });

      cy.on('tap', 'node', (evt) => {
        const id = evt.target.data('id');
        if (!id) return;
        selectNode(id);
      });

      cy.on('tap', (evt) => {
        // Tap on background → clear selection.
        if (evt.target === cy) {
          showEmptyPanel();
        }
      });
    }
  }

  // ── Side panel rendering ────────────────────────────────────────────────

  function fmtFloat(v, digits) {
    if (v === null || v === undefined || Number.isNaN(v)) return '—';
    return Number(v).toFixed(digits ?? 3);
  }

  function showEmptyPanel() {
    selectedEdge = null;
    selectedNodeID = null;
    els.panelEdge.classList.add('hidden');
    els.panelNode.classList.add('hidden');
    els.panelEmpty.classList.remove('hidden');
  }

  function selectEdge(edge) {
    selectedEdge = edge;
    selectedNodeID = null;
    els.panelEmpty.classList.add('hidden');
    els.panelNode.classList.add('hidden');
    els.panelEdge.classList.remove('hidden');

    const prop = cachedGraph && cachedGraph.propositions
      ? cachedGraph.propositions.find((p) => p.proposition_id === edge.proposition_id)
      : null;

    // Causal-claim sentence from the ontology. Empty for auto-proposed
    // candidates that haven't been described yet; we hide the element in
    // that case rather than show a blank line.
    if (prop && prop.description) {
      els.edDescription.textContent = prop.description;
      els.edDescription.classList.remove('hidden');
    } else {
      els.edDescription.textContent = '';
      els.edDescription.classList.add('hidden');
    }

    els.edFrom.textContent = edge.from;
    els.edTo.textContent   = edge.to;
    els.edPid.textContent  = edge.proposition_id;
    els.edDir.textContent  = edge.direction;
    els.edDir.className    = edge.direction === '-' ? 'dir-neg' : 'dir-pos';
    els.edPrior.textContent = fmtFloat(edge.prior_weight);
    els.edEma.textContent   = fmtFloat(edge.ema_weight);
    els.edConf.textContent  = fmtFloat(edge.confidence);
    els.edNobs.textContent  = edge.n_observations ?? 0;

    if (prop && prop.deprecated) {
      els.edDeprecated.textContent = `yes — ${prop.deprecated_reason || '(no reason)'}`;
      els.edDeprecated.className = 'flag-deprecated';
    } else {
      els.edDeprecated.textContent = 'no';
      els.edDeprecated.className = 'flag-active';
    }

    renderHistory(edge.proposition_id);
  }

  function renderHistory(propositionID) {
    // Per plan §"Mutation flow" step 1: filter the cached /history payload
    // client-side for entries whose target_id matches this proposition.
    const matches = cachedHistory
      .filter((h) => h && h.target_id === propositionID)
      .sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp))
      .slice(0, 5);

    if (matches.length === 0) {
      els.historyList.innerHTML = '<li class="hint small">No history yet for this proposition.</li>';
      return;
    }

    els.historyList.innerHTML = '';
    for (const ev of matches) {
      const li = document.createElement('li');
      const ts = ev.timestamp ? new Date(ev.timestamp).toISOString().replace('T', ' ').slice(0, 19) : '—';
      const detail = ev.detail ? JSON.stringify(ev.detail) : '';
      li.innerHTML =
        `<span class="history-kind">${escapeHTML(ev.kind || '?')}</span>` +
        `<span class="history-ts">${escapeHTML(ts)}</span>` +
        (detail ? `<br/><span>${escapeHTML(detail)}</span>` : '');
      els.historyList.appendChild(li);
    }
  }

  function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, (c) => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    })[c]);
  }

  // ── Node panel ──────────────────────────────────────────────────────────

  // selectNode populates the side panel with a construct's descriptor and the
  // edges that touch it. Constructs are append-only (the ontology contract
  // forbids removal), so the panel offers no mutation actions — but every
  // listed edge is clickable to jump into the edge panel where mutations live.
  function selectNode(constructID) {
    if (!cachedGraph) return;
    const ctor = (cachedGraph.constructs || []).find((c) => c.construct_id === constructID);
    if (!ctor) {
      toast(`Unknown construct: ${constructID}`, 'error');
      return;
    }

    selectedNodeID = constructID;
    selectedEdge = null;
    els.panelEmpty.classList.add('hidden');
    els.panelEdge.classList.add('hidden');
    els.panelNode.classList.remove('hidden');

    els.ndID.textContent   = ctor.construct_id;
    els.ndName.textContent = ctor.name || '—';
    els.ndDesc.textContent = ctor.description || '—';

    const edges = cachedGraph.edges || [];
    const outgoing = edges.filter((e) => e.from === constructID);
    const incoming = edges.filter((e) => e.to === constructID);
    const total = outgoing.length + incoming.length;
    els.ndDegree.textContent =
      `${total} (${outgoing.length} out · ${incoming.length} in)`;

    renderEdgeLinkList(els.ndOutList, els.ndOutCount, outgoing,
      'No outgoing propositions.');
    renderEdgeLinkList(els.ndInList, els.ndInCount, incoming,
      'No incoming propositions.');
  }

  // renderEdgeLinkList fills a <ul> with clickable edge summaries. Clicking
  // an item swaps the panel to edge mode for that proposition.
  function renderEdgeLinkList(listEl, countEl, edges, emptyMsg) {
    countEl.textContent = edges.length ? `(${edges.length})` : '';
    if (!edges.length) {
      listEl.innerHTML = `<li class="hint small">${escapeHTML(emptyMsg)}</li>`;
      return;
    }
    listEl.innerHTML = '';
    for (const e of edges) {
      const prop = (cachedGraph.propositions || []).find(
        (p) => p.proposition_id === e.proposition_id
      );
      const deprecated = !!(prop && prop.deprecated);
      const li = document.createElement('li');
      li.className = 'edge-link' + (deprecated ? ' edge-link-deprecated' : '');
      const dirCls = e.direction === '-' ? 'dir-neg' : 'dir-pos';
      li.innerHTML =
        `<span class="edge-link-pid">${escapeHTML(e.proposition_id)}</span>` +
        ` <span class="${dirCls}">${escapeHTML(e.direction)}</span>` +
        ` <span class="edge-link-pair">${escapeHTML(e.from)} → ${escapeHTML(e.to)}</span>` +
        ` <span class="edge-link-weight hint small">w=${fmtFloat(e.ema_weight, 2)} · c=${fmtFloat(e.confidence, 2)}</span>` +
        (deprecated ? ' <span class="edge-link-flag hint small">deprecated</span>' : '');
      li.addEventListener('click', () => {
        selectEdge(e);
        if (cy) {
          cy.elements().unselect();
          const cyEdge = cy.getElementById(e.proposition_id);
          if (cyEdge && cyEdge.length) cyEdge.select();
        }
      });
      listEl.appendChild(li);
    }
  }

  // ── Modal ───────────────────────────────────────────────────────────────

  function openModal(kind, edge) {
    if (!edge) {
      toast('Select an edge first', 'error');
      return;
    }

    // Reset class + populate fields based on action kind.
    els.modal.className = '';
    els.modal.classList.add('modal-' + kind);

    if (kind === 'strength') {
      els.modalTitle.textContent = 'Set proposition strength';
      els.modalSubtitle.textContent = `${edge.proposition_id}: ${edge.from} → ${edge.to}`;
      els.modalStrength.value = (edge.prior_weight ?? 0.5).toFixed(2);
    } else if (kind === 'deprecate') {
      els.modalTitle.textContent = 'Deprecate proposition';
      els.modalSubtitle.textContent = `${edge.proposition_id}: ${edge.from} → ${edge.to}`;
      els.modalReason.value = '';
    } else if (kind === 'reset') {
      els.modalTitle.textContent = 'Reset edge EMA';
      els.modalSubtitle.textContent = `${edge.proposition_id}`;
      els.modalResetFrom.textContent = edge.from;
      els.modalResetTo.textContent = edge.to;
    }

    // Rebind submit handler for this specific (kind, edge) pair. We replace
    // the entire onsubmit so prior handlers can't fire against a stale edge.
    els.modalForm.onsubmit = (ev) => {
      ev.preventDefault();
      submitModal(kind, edge).catch((err) => toast(err.message || String(err), 'error'));
    };

    if (typeof els.modal.showModal === 'function') {
      els.modal.showModal();
    } else {
      // Extremely unlikely fallback for browsers without <dialog>.
      els.modal.setAttribute('open', '');
    }
  }

  function closeModal() {
    if (typeof els.modal.close === 'function') {
      els.modal.close();
    } else {
      els.modal.removeAttribute('open');
    }
  }

  async function submitModal(kind, edge) {
    if (kind === 'strength') {
      const v = parseFloat(els.modalStrength.value);
      if (Number.isNaN(v) || v < 0 || v > 1) {
        toast('Strength must be a number in [0.0, 1.0]', 'error');
        return;
      }
      await postJSON('/ontology/strength', {
        proposition_id: edge.proposition_id,
        strength: v,
      });
      closeModal();
      toast(`${edge.proposition_id} strength set to ${v.toFixed(2)}`, 'success');
    } else if (kind === 'deprecate') {
      const reason = (els.modalReason.value || '').trim();
      if (!reason) {
        toast('Please provide a reason', 'error');
        return;
      }
      await postJSON('/ontology/deprecate', {
        proposition_id: edge.proposition_id,
        reason: reason,
      });
      closeModal();
      toast(`${edge.proposition_id} deprecated`, 'success');
    } else if (kind === 'reset') {
      await postJSON('/agent/reset', {
        from: edge.from,
        to: edge.to,
      });
      closeModal();
      toast(`Edge ${edge.from} → ${edge.to} reset`, 'success');
    }

    await loadAll();
    // If the selected edge still exists, re-select it so the panel reflects
    // the new state (e.g. updated EMA weight, deprecated flag).
    if (cachedGraph && selectedEdge) {
      const fresh = (cachedGraph.edges || []).find(
        (e) => e.proposition_id === selectedEdge.proposition_id
      );
      if (fresh) selectEdge(fresh);
    }
  }

  // ── Candidates ───────────────────────────────────────────────────────────────

  async function loadCandidates() {
    try {
      const resp = await fetch('/candidates');
      if (!resp.ok) return;
      const candidates = await resp.json();
      updateCandidatesChip(candidates);
      renderCandidatesList(candidates);
    } catch (e) {
      console.warn('candidates fetch failed', e);
    }
  }

  function updateCandidatesChip(candidates) {
    const chip = els.candidatesChip;
    if (!chip) return;
    const n = candidates ? candidates.length : 0;
    chip.textContent = n + ' candidate' + (n !== 1 ? 's' : '');
    chip.classList.toggle('hidden', n === 0);
  }

  function renderCandidatesList(candidates) {
    const el = els.candidatesList;
    if (!el) return;
    if (!candidates || candidates.length === 0) {
      el.innerHTML = '<p class="empty">No pending candidates.</p>';
      return;
    }
    el.innerHTML = candidates.map(c => `
      <div class="candidate-card">
        <div class="candidate-header">
          <strong>${escapeHTML(c.FromID)} &rarr; ${escapeHTML(c.ToID)}</strong>
          <span class="direction ${c.Direction === 1 ? 'pos' : 'neg'}">${c.Direction === 1 ? '+' : '−'}</span>
        </div>
        <div class="candidate-meta">
          r=${(c.MIScore || 0).toFixed(3)} &nbsp; p=${(c.PValue || 0).toFixed(4)} &nbsp; n=${c.NObservations || 0}
        </div>
        <div class="candidate-actions">
          <button onclick="window.__reviewCandidate('${escapeHTML(c.CandidateID)}','confirm')" class="btn-confirm">Confirm</button>
          <button onclick="window.__reviewCandidate('${escapeHTML(c.CandidateID)}','reject')" class="btn-reject">Reject</button>
          <button onclick="window.__reviewCandidate('${escapeHTML(c.CandidateID)}','defer')" class="btn-defer">Defer</button>
        </div>
      </div>
    `).join('');
  }

  async function reviewCandidate(id, action) {
    try {
      const resp = await fetch(`/candidates/${encodeURIComponent(id)}/${action}`, { method: 'POST' });
      if (!resp.ok) {
        const err = await resp.json().catch(() => ({ error: resp.statusText }));
        toast('Error: ' + (err.error || resp.statusText), 'error');
        return;
      }
      toast(`Candidate ${action}ed`, 'success');
      await loadAll(); // refresh graph and candidates
    } catch (e) {
      toast('Request failed: ' + e.message, 'error');
    }
  }

  // Expose reviewCandidate globally so inline onclick handlers in the
  // candidates list can reach it from inside the IIFE.
  window.__reviewCandidate = reviewCandidate;

  // ── Data load ───────────────────────────────────────────────────────────

  async function loadAll() {
    try {
      const [graph, history] = await Promise.all([
        getJSON('/graph'),
        getJSON('/history'),
      ]);
      cachedGraph = graph;
      cachedHistory = Array.isArray(history) ? history : [];
      renderGraph(graph);
      if (selectedEdge) {
        const fresh = (graph.edges || []).find(
          (e) => e.proposition_id === selectedEdge.proposition_id
        );
        if (fresh) selectEdge(fresh);
        else showEmptyPanel();
      } else if (selectedNodeID) {
        const stillThere = (graph.constructs || []).some(
          (c) => c.construct_id === selectedNodeID
        );
        if (stillThere) selectNode(selectedNodeID);
        else showEmptyPanel();
      }
      await refreshHealth();
      await loadCandidates();
    } catch (err) {
      toast(err.message || String(err), 'error');
      els.healthDot.className = 'health-dot health-bad';
      els.healthLabel.textContent = 'error';
    }
  }

  // ── Auto-refresh toggle ─────────────────────────────────────────────────

  function setAutoRefresh(enabled) {
    if (autoRefreshHandle) {
      clearInterval(autoRefreshHandle);
      autoRefreshHandle = null;
    }
    if (enabled) {
      autoRefreshHandle = setInterval(loadAll, 5000);
    }
  }

  // ── Operator Tune ──────────────────────────────────────────────────────

  async function submitTune() {
    const input = document.getElementById('tune-input');
    const text = input ? input.value.trim() : '';
    if (!text) return;
    try {
      const resp = await fetch('/agent/tune', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ intent: text, operator: 'ui' })
      });
      const data = await resp.json();
      if (!resp.ok) {
        toast('Tune error: ' + (data.error || resp.statusText), 'error');
        return;
      }
      const resultEl = document.getElementById('tune-result');
      if (data.applied && data.applied.length > 0) {
        const summary = data.applied.map(a =>
          `${a.proposition_id}: ${a.old_strength.toFixed(3)} → ${a.new_strength.toFixed(3)}`
        ).join(', ');
        toast('Tuned: ' + summary, 'success');
        if (resultEl) {
          resultEl.innerHTML =
            '<ul>' + data.applied.map(a =>
              '<li><strong>' + escapeHTML(a.proposition_id) + '</strong> ' +
              a.old_strength.toFixed(3) + ' → ' + a.new_strength.toFixed(3) +
              '<br><small>' + escapeHTML(a.rationale) + '</small></li>'
            ).join('') + '</ul>';
        }
      } else {
        toast('Intent not recognized — no adjustments applied.', 'success');
        if (resultEl) {
          resultEl.innerHTML = '<p class="empty">No adjustments.</p>';
        }
      }
      await loadAll();
    } catch (e) {
      toast('Tune failed: ' + e.message, 'error');
    }
  }

  // Expose submitTune globally so the inline onclick on #tune-btn can reach it.
  window.submitTune = submitTune;

  // ── Wire up event listeners ─────────────────────────────────────────────

  function wire() {
    els.refreshBtn.addEventListener('click', () => loadAll());
    els.autoRefresh.addEventListener('change', (e) => setAutoRefresh(e.target.checked));

    els.btnStrength.addEventListener('click',  () => openModal('strength',  selectedEdge));
    els.btnDeprecate.addEventListener('click', () => openModal('deprecate', selectedEdge));
    els.btnReset.addEventListener('click',     () => openModal('reset',     selectedEdge));

    els.modalCancel.addEventListener('click', (ev) => {
      ev.preventDefault();
      closeModal();
    });

    // Candidates chip click: toggle the candidates panel.
    if (els.candidatesChip) {
      els.candidatesChip.addEventListener('click', () => {
        showingCandidates = !showingCandidates;
        if (showingCandidates) {
          // Hide other sections; show candidates.
          els.panelEmpty.classList.add('hidden');
          els.panelNode.classList.add('hidden');
          els.panelEdge.classList.add('hidden');
          els.panelCandidates.classList.remove('hidden');
          // Re-render with latest data.
          loadCandidates();
        } else {
          els.panelCandidates.classList.add('hidden');
          showEmptyPanel();
        }
      });
    }

    // ESC inside <dialog> fires a "cancel" event, then closes. Nothing to do.
  }

  // ── Bootstrap ───────────────────────────────────────────────────────────

  document.addEventListener('DOMContentLoaded', () => {
    if (typeof cytoscape !== 'function') {
      toast('Failed to load Cytoscape from CDN', 'error');
      return;
    }
    wire();
    loadAll();

    // Wire Enter key on tune input.
    const tuneInput = document.getElementById('tune-input');
    if (tuneInput) {
      tuneInput.addEventListener('keydown', function(e) {
        if (e.key === 'Enter') submitTune();
      });
    }
  });
})();
