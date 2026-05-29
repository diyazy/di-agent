# Semantic Map — Architecture

Design rationale and decision record. Update this file when a contract, profile, MetricType, or structural decision changes. For usage (running, API, compliance), see [README.md](README.md).

---

## Table of Contents

- [1. Core Concept](#1-core-concept)
- [2. Contract Architecture](#2-contract-architecture)
  - [The six contracts](#the-six-contracts)
  - [Behavioral guarantees](#behavioral-guarantees)
- [3. Deployment Profiles](#3-deployment-profiles)
- [4. Language Strategy](#4-language-strategy)
- [5. Telemetry Pipeline](#5-telemetry-pipeline)
  - [CollectorContract](#collectorcont ract)
  - [MetricType catalogue](#metrictype-catalogue)
  - [The Bridge](#the-bridge)
  - [Planned collector implementations](#planned-collector-implementations)
- [6. Automatic Graph Extension](#6-automatic-graph-extension)
- [7. Adding a New Profile](#7-adding-a-new-profile)
- [8. Connection to Research](#8-connection-to-research)
- [9. Control Surface](#9-control-surface)

---

## 1. Core Concept

The Semantic Map has two layers that are always present simultaneously:

```
┌────────────────────────────────────────────────────────────────┐
│  Layer 2 — Evidence (dynamic)                                  │
│  Statistical descriptors updated by live telemetry             │
│  "In THIS cluster, under THESE workloads, here is reality"     │
├────────────────────────────────────────────────────────────────┤
│  Layer 1 — Backbone (stable prior)                             │
│  7 Di-Select constructs + 15 causal propositions (P1–P15)      │
│  "What matters and how things relate"                          │
└────────────────────────────────────────────────────────────────┘
```

**The cold-start arc:** on day one the agent relies entirely on Di-Select priors. As deployment telemetry flows in, each edge's EMA drifts toward observed reality. A `confidence` score on every edge tracks the transition:

```
effective_value = (1 - confidence) × prior  +  confidence × ema
```

At `confidence = 0.0` the agent uses the literature. At `confidence = 1.0` it uses its own deployment history. The transition is smooth and automatic.

**What is stable and what is not:**

| Element                               | Stable?                                        |
| ------------------------------------- | ---------------------------------------------- |
| Graph topology — the 7 constructs     | Yes — domain-invariant                         |
| Proposition directions (P1–P15)       | Yes — causal directions do not change          |
| Proposition magnitudes (edge weights) | No — learned from evidence                     |
| New edges (P16+)                      | Possible — discovered by the Proposer contract |

---

## 2. Contract Architecture

The Semantic Map is not a monolith. It is a **set of responsibilities, each behind a contract (interface)**. Concrete implementations are fully swappable — agent code never imports an implementation directly.

```
  Metric source          Semantic Map
  (cgroup / Netdata)
        │
   [Collector] ──samples──▶ [Bridge] ──update_edge()──▶ [Updater]
                                                              │
                    ┌─────────────────────────────────────────┘
                    ▼
        ┌───────────────────────────────────────────┐
        │              SemanticMap facade            │
        │  cost_of_action()  recommend_peer()        │
        │  simulate_outcome()                        │
        └───┬───────┬──────────┬────────┬────────────┘
            │       │          │        │
        Storage  Ontology  Reasoner  Proposer
```

The Collector and Bridge live outside the SemanticMap facade — they feed it. The Bridge is not a contract; it is a thin, stateless mapper (see §5).

### The six contracts

| Contract      | Responsibility                                              | Key guarantees                                                                                            |
| ------------- | ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| **Collector** | Read raw metrics from a source; emit normalized samples     | Pure read; deterministic `event_id`; `available_metrics()` is static; never raises on empty data         |
| **Storage**   | Read/write node and edge descriptors                        | Atomic writes; `nil` on miss, never raises. **Multigraph:** edges keyed by `(from, to, proposition_id)` — `GetEdgesByPair` returns all edges between two constructs; `GetEdge` returns one deterministic pick |
| **Ontology**  | Live structural knowledge — constructs, propositions, validation, audit | Always returns ≥7 constructs + P1–P15; constructs are append-only; propositions are soft-deleted via `Deprecate` (never removed or direction-reversed); every mutation appends to an audit log readable via `GetHistory` |
| **Updater**   | Incorporate telemetry into edge/node descriptors            | Idempotent per `(edge, event_id)` — one observation updates every edge in a `(from, to)` pair, each tracking its own EMA. `Reset` restores prior without deleting |
| **Reasoner**  | Produce agent decisions with traceable rationales           | Every result includes a non-empty rationale referencing graph path; `SimulateOutcome` is pure (read-only) |
| **Proposer**  | Detect statistical patterns suggesting new backbone edges   | Never modifies Storage or Ontology directly; `Reject` permanently suppresses within session               |

### Behavioral guarantees

Guarantees are not just signatures — they are documented pre/post-conditions on each method in the contract source files. The compliance test suites in `compliance/` verify them mechanically. **A new implementation is valid if and only if it passes the compliance suite for its contract.** This is the definition, not a check.

### The ontology is alive

The ontology is not a static reference. Empirical priors get recalibrated as new papers land, operators deprecate claims that the deployment's evidence contradicts, and new domains may introduce new constructs. The contract therefore admits four kinds of mutation, each emitting one `OntologyEvent` to an append-only audit log:

| Mutation                          | Method                                       | Typical caller                          |
| --------------------------------- | -------------------------------------------- | --------------------------------------- |
| Edge magnitude recalibrated       | `SetPropositionStrength(propID, strength)`   | `prior_init` pipeline; operator tuning  |
| New edge added (validated)        | `AddValidatedProposition(p)`                 | `Proposer.Confirm` (post-review)        |
| New construct added               | `AddConstruct(c)`                            | Operator (new domain extension)         |
| Existing edge retired (soft)      | `Deprecate(propID, reason)`                  | Operator (evidence-against accumulated) |

What is stable, what is not:

- **Construct removal** is impossible. Constructs are domain-stable per the architecture; once added they stay forever.
- **Proposition removal** is impossible. `Deprecate` is the only retirement path. Deprecated propositions remain in `Propositions()` so the audit trail / replay are intact, but the Reasoner skips them during cost computation. The `EdgeDescriptor` in storage stays in place too — soft-delete preserves both the structural and the evidence record.
- **Proposition direction reversal** is impossible. `ValidateProposition` rejects a new edge that contradicts an existing direction. The three Di-Select conflict pairs (P2/P3, P5/P6, P7/P9) are exempt because both halves are present from the bootstrap; the Proposer cannot introduce *new* conflict pairs without explicit operator action (a future extension).

The audit log (`GetHistory(since)`) lets the agent answer "why is this edge weight what it is?" at any point in time. On the edge-minimal profile the log is in-memory and ephemeral across restarts. The `cloud-full` profile persists it.

Implementations that intentionally do not support a mutation (e.g. a read-only ontology cache layered in front of the canonical store) return `contracts.ErrNotImplemented` rather than silently succeeding. The compliance suite tolerates this — every live-ontology subtest skips on `ErrNotImplemented`.

### Why the backbone is a multigraph

Di-Select's 15 propositions span only 12 distinct construct pairs because three are **conflict pairs** — two propositions on the same `(from, to)` capturing distinct mechanisms with opposite directions:

| Pair        | Mechanism captured by each proposition                                                       |
| ----------- | -------------------------------------------------------------------------------------------- |
| **P2 / P3** on RC→PS | P2 (−): security/resource overhead reduces throughput. P3 (+): lightweight distributions reduce pod-startup latency. |
| **P5 / P6** on CO→RR | P5 (+): offline autonomy improves continuity during partition. P6 (−): cloud dependency reduces stability in poor networks. |
| **P7 / P9** on CE→MU | P7 (+): rich ecosystem lowers operator effort. P9 (−): excessive features increase maintenance complexity. |

These are not contradictions — they are **co-existing, evidence-distinguishable** mechanisms. In a real deployment, observed telemetry will support one mechanism more than the other, and each proposition's EMA drifts independently. The agent therefore needs to store both edges, update both from a single observation, and let the relative confidence-weighted magnitudes encode which mechanism dominates in this deployment.

Implications for the contracts:

- **Storage** keys edges by the full triple `(from, to, proposition_id)`. `GetEdgesByPair(from, to)` returns every edge — critical for the Updater. `GetEdge(from, to)` returns a deterministic pick (lex-smallest `proposition_id`) so single-edge callers keep working.
- **Updater** applies one observation to every edge between `(from, to)`. Idempotency is keyed on `(from, to, proposition_id, event_id)` so a replay is a no-op per-edge, not just per-pair.
- **Reasoner** iterates `AllEdges()` directly and uses each edge's own `Direction`. There is no proposition-to-edge join, and so no risk of conflating P2 with P3.
- **Ontology** `ValidateProposition` rejects a new proposition that contradicts an existing one. The three bootstrap conflict pairs are exempt because both are present from the start with domain validation. New auto-proposed conflicts from the Proposer go through the normal rejection path — backbone extension does not introduce conflict pairs without explicit operator action.

---

## 3. Deployment Profiles

A profile is a named configuration that wires specific implementations to each contract. The agent binary is identical across profiles — only the profile name changes at startup.

| Profile         | Collector                   | Storage   | Ontology                 | Updater        | Reasoner         | Proposer          | Target                |
| --------------- | --------------------------- | --------- | ------------------------ | -------------- | ---------------- | ----------------- | --------------------- |
| `edge-minimal`  | cgroup (direct `/sys/fs`)   | In-memory | Static Di-Select         | EMA            | Rule engine      | Disabled          | RPi4, IoT nodes       |
| `edge-standard` | cgroup + kubelet `/metrics` | SQLite    | Static + extension table | EMA + Gaussian | Rule engine      | Threshold-based   | Standard edge servers |
| `cloud-full`    | Netdata HTTP API            | Neo4j     | RDF/OWL + SPARQL         | Bayesian       | SLM (Phi-3 Mini) | Correlation miner | Cloud VMs             |

**EMA** — Exponential Moving Average: `new = α × observation + (1-α) × old`. Controls how fast the agent adapts. `α = 0.2` is the default.

**Gaussian (μ, σ)** — adds variance tracking alongside the mean. Required for `simulate_outcome()` to return P95 risk estimates. Available from `edge-standard` upward.

**Bayesian updater** — full posterior distribution update. Richer uncertainty quantification but heavier. Cloud-only.

**Why not Python on the edge?** Baseline interpreter footprint (~50–80 MB), unpredictable GC pauses under latency budgets, and the operational cost of managing a Python runtime on every constrained node.

---

## 4. Language Strategy

| Layer                                       | Language   | Why                                                                                                                           |
| ------------------------------------------- | ---------- | ----------------------------------------------------------------------------------------------------------------------------- |
| Contract interfaces + compliance tests      | **Python** | Specification role — readable, fast to iterate, serves as the authoritative definition of correct behavior                    |
| Prior initialization pipeline               | **Python** | One-time data wrangling from P1–P5; pandas/numpy/scipy ecosystem                                                              |
| `cloud-full` profile service                | **Python** | scipy for Bayesian updater; correlation miner; SLM integration                                                                |
| `edge-minimal` and `edge-standard` daemons  | **Go**     | Single ARM binary, <10 MB footprint, no runtime to manage on edge nodes, goroutines for concurrent telemetry, predictable GC  |

**The contract boundary enables this split.** The Python ABCs are the specification. The Go interfaces mirror them exactly. Both language implementations run against their respective compliance suites — passing both suites proves behavioral equivalence across languages.

---

## 5. Telemetry Pipeline

Live observations flow into the Semantic Map through a three-stage pipeline:

```
┌──────────────┐   MetricSample[]   ┌────────┐  update_edge()  ┌─────────┐
│  Collector   │ ─────────────────▶ │ Bridge │ ──────────────▶ │ Updater │
│  (contract)  │                    │ (thin) │                  │(contract│
└──────────────┘                    └────────┘                  └─────────┘
  cgroup plugin                     maps metric type      EMA / Gaussian /
  Netdata plugin                    → (from_id, to_id)    Bayesian update
  kubelet plugin                    via Ontology
```

### CollectorContract

A collector reads from a raw source, normalizes to `MetricSample`s, and returns them. It knows nothing about the graph.

```python
samples: list[MetricSample] = collector.collect()
```

Each `MetricSample` carries:

| Field            | Type         | Description                                              |
| ---------------- | ------------ | -------------------------------------------------------- |
| `node_id`        | `str`        | Cluster node (`"master"`, `"node_1"`, …)                |
| `metric_type`    | `MetricType` | Semantic type — see catalogue below                      |
| `value`          | `float`      | Normalized value in the fixed unit for the metric type   |
| `timestamp_unix` | `int`        | Unix timestamp of the observation                        |
| `event_id`       | `str`        | Deterministic ID — same observation always → same ID     |
| `container_id`   | `str`        | Empty for node-level aggregates; set for per-container   |
| `labels`         | `dict`       | Source metadata (cgroup path, Netdata chart, …); opaque  |

**`event_id` determinism** is the collector's responsibility. A stable recipe: `sha256(source_id + node_id + container_id + metric_type + str(timestamp_unix))[:16]`. This carries the Updater's idempotency guarantee end-to-end: replaying the same telemetry batch has no effect on the graph.

**`available_metrics()` is static** — declared once at construction, never changes within a deployment session. The Bridge uses this to know which graph edges can be updated without calling `collect()` first.

### MetricType catalogue

Units are fixed per type. Collectors must normalize raw source values to these units before emitting.

| `MetricType`            | Unit           | Maps to construct(s)            | Note                          |
| ----------------------- | -------------- | ------------------------------- | ----------------------------- |
| `cpu_utilization`       | fraction [0,1] | RC                              |                               |
| `memory_utilization`    | fraction [0,1] | RC                              |                               |
| `cpu_throttle_ratio`    | fraction [0,1] | RC → PS edge (P2 proxy)         | cgroup `cpu.stat` throttled_periods / total_periods |
| `block_io_util`         | fraction [0,1] | RC                              |                               |
| `pod_startup_ms`        | milliseconds   | PS                              | creation timestamp → Running  |
| `scheduling_latency_ms` | milliseconds   | PS                              | Pending → Scheduled           |
| `network_rx_bps`        | bytes/sec      | CO                              |                               |
| `network_tx_bps`        | bytes/sec      | CO                              |                               |
| `network_loss_ratio`    | fraction [0,1] | CO → PS edge (P13 proxy)        |                               |
| `network_latency_ms`    | milliseconds   | CO, PS                          | RTT to a peer node            |
| `energy_joules`         | joules         | RC (energy cost per interval)   | from RAPL or P4 model         |

**Constructs with no runtime telemetry** (SC, MU, CE, RR) are updated exclusively from the prior. This is intentional — those constructs reflect structural properties of the distribution (security posture, setup complexity, community health) that do not change during a running deployment. Their priors are set by the initialization pipeline.

### The Bridge

The bridge is a stateless function wired inside the agent. It is not a contract because its logic is fully determined by the Ontology — there is nothing to swap. For each `MetricSample` it:

1. Looks up which proposition edges involve the metric's target construct via `OntologyContract.Relationships(construct_id)`
2. Calls `UpdaterContract.update_edge(from_id, to_id, sample.value, sample.event_id)` for each edge
3. Calls `UpdaterContract.update_node(construct_id, sample.value, sample.event_id)` for the node descriptor

Because `event_id` flows unchanged from Collector → Bridge → Updater, idempotency is end-to-end.

### Planned collector implementations

| Plugin              | Source                           | Profile         | Status  | Available metrics                                                    |
| ------------------- | -------------------------------- | --------------- | ------- | -------------------------------------------------------------------- |
| `CgroupCollector`   | `/sys/fs/cgroup/`                | `edge-minimal`  | ✅ done | cpu\_utilization, memory\_utilization, cpu\_throttle\_ratio          |
| `KubeletCollector`  | kubelet `/metrics/resource`      | `edge-standard` | planned | pod\_startup\_ms, scheduling\_latency\_ms                            |
| `NetdataCollector`  | Netdata HTTP streaming API       | `cloud-full`    | planned | All MetricTypes + custom chart contexts                              |

Multiple collectors can run concurrently in the same agent (e.g., `edge-standard` runs both Cgroup and Kubelet). The Bridge processes all their outputs — idempotency ensures overlapping `event_id`s from the same physical observation are harmless.

---

## 6. Automatic Graph Extension

The Proposer contract supports discovering relationships beyond P1–P15. The flow is **propose-then-confirm** — patterns are detected automatically, but a human confirms before the backbone is modified.

```
Telemetry accumulates in the evidence layer
        ↓
Proposer computes mutual information between construct time series
        ↓
If MI > threshold AND p < 0.05 AND n_observations > min_support:
    → Emit CandidateEdge (visible via GET /candidates)
        ↓
Operator reviews: confirm / reject / defer
        ↓
Confirm → OntologyContract.AddValidatedProposition()
          (structural validation runs first — contradictions are rejected)
Reject  → Suppressed for this deployment session
```

The Proposer **never modifies the backbone directly**. `Confirm` delegates to `OntologyContract.AddValidatedProposition`, which validates the new edge against existing propositions before accepting. A proposed edge that contradicts a validated proposition (e.g., a positive direction where a negative is already established) is rejected.

The `edge-minimal` profile ships with `DisabledProposer` (no-op). Automatic extension is available from `edge-standard` upward.

---

## 7. Adding a New Profile

1. Create `go/internal/<profile-name>/` and implement all six contracts, or reuse existing implementations.
2. Every implementation must pass its contract's compliance suite before being wired into a profile.
3. Add a case to `go/pkg/profiles/profiles.go`:

```go
case "my-profile":
    collector := myprofile.NewMyCollector(...)
    storage   := myprofile.NewMyStorage(...)
    ontology  := minimal.NewStaticDiSelectOntology() // reuse if sufficient
    updater   := myprofile.NewMyUpdater(storage, ...)
    reasoner  := myprofile.NewMyReasoner(storage, ontology, ...)
    proposer  := myprofile.NewMyProposer(...)
    seedFromOntology(storage, ontology)
    return semmap.New(storage, ontology, updater, reasoner, proposer), collector, nil
```

4. Add the profile to `profiles.py` (Python registry) if a Python equivalent is needed.
5. Update the profiles table in this file (§3) and the project structure in README.md.

No other file needs to change.

---

## 8. Connection to Research

| Publication                                 | Role in Semantic Map                                                          |
| ------------------------------------------- | ----------------------------------------------------------------------------- |
| P1 (Performance & Resource Efficiency)      | Initial priors: pod-startup latency, throughput constants per KD              |
| P2 (Security, Resilience & Maintainability) | Initial priors: security compliance scores, recovery time constants           |
| P3 (Di-Select Framework)                    | Backbone topology: 7 constructs, 15 propositions, prior directions            |
| P4 (Energy Analysis / DVFS)                 | Initial priors: J/pod, mJ/op, interrupt amplification ratios per KD           |
| P5 (Overhead Decomposition)                 | Initial priors: per-component CPU overhead (kube-apiserver = 66.7% idle)      |
| **P6 (this work)**                          | The Semantic Map itself — schema, prior initialization, convergence study     |
| P7 (Decentralized Framework)                | Extends the Semantic Map with P2P trust edges and gossip-based peer discovery |

**P6 scientific contributions:**
1. Contract-based architecture enabling RPi4-to-cloud profile switching without changing agent logic
2. Prior initialization protocol connecting Di-Select to agent runtime (grounded in P1–P5 empirical constants)
3. Convergence study: how quickly does deployment evidence override generic priors?
4. Propose-then-confirm loop: controlled automatic backbone extension with structural validation

---

## 9. Control Surface

The Semantic Map facade is a Go API. The agent daemon wraps it in three layers so that operators, scripts, and demos can drive the same surface without sharing process memory:

```
┌─────────────────────────────────────────────────────────────┐
│  Layer C — Web UI         cmd/agent/static/{index,app,style} │
│  Vanilla JS + Cytoscape.js, embedded via go:embed all:static │
│  Click-to-mutate viewer at /ui/                              │
├─────────────────────────────────────────────────────────────┤
│  Layer B — CLI            cmd/mapctl/                        │
│  cobra + tablewriter; one binary, sixteen subcommands        │
│  Default --addr http://localhost:8080; --json for scripting  │
├─────────────────────────────────────────────────────────────┤
│  Layer A — HTTP API       cmd/agent/{routes,dto,static}.go   │
│  net/http only; JSON in/out; CSRF via Content-Type guard     │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
                  SemanticMap facade (pkg/semmap)
```

Every layer talks to the layer above only via HTTP — the CLI does not import `cmd/agent`, and the UI is served as static assets. This is deliberate: the daemon is the single integration point, and any third tool (e.g. a future TUI, a fleet controller) only needs to speak JSON to participate.

### HTTP API

Two endpoint families coexist on the same mux. The five pre-existing endpoints (`/ingest`, `/cost`, `/recommend`, `/simulate`, `/candidates`) keep their original `http.Error` plain-text error format to minimize diff against the v0 daemon. Every endpoint added in the Phase 1 expansion emits structured JSON errors and gates mutations on `Content-Type: application/json`.

| Verb | Path                              | Request body / params                                                  | Response (2xx)                | Semantics                                                                                              |
| ---- | --------------------------------- | ---------------------------------------------------------------------- | ----------------------------- | ------------------------------------------------------------------------------------------------------ |
| GET  | `/healthz`                        | —                                                                      | `{"ok":true}`                 | Liveness probe; never touches the facade                                                                |
| GET  | `/version`                        | —                                                                      | `VersionResponse`             | Agent version, Go runtime, build commit, construct/proposition counts                                  |
| GET  | `/graph`                          | —                                                                      | `GraphSnapshot`               | Full snapshot: every construct, every proposition (incl. deprecated), every edge                       |
| GET  | `/edges`                          | `?from=&to=` (either or both, optional)                                | `[]EdgeDTO`                   | Filtered edge list; when both `from` and `to` are set, returns the conflict-pair multigraph fan-out    |
| GET  | `/constructs`                     | —                                                                      | `[]ConstructDTO`              | Backbone nodes                                                                                         |
| GET  | `/propositions`                   | —                                                                      | `[]PropositionDTO`            | All propositions including those soft-deleted by `Deprecate` (the DTO carries a `deprecated` flag)     |
| GET  | `/history`                        | `?since=` (RFC3339 timestamp or Go duration like `1h`; omitted → zero) | `[]OntologyEventDTO`          | Append-only audit log of mutations                                                                     |
| GET  | `/neighbors`                      | `?node=ID` (required)                                                  | `[]string`                    | IDs of constructs reachable in one hop                                                                 |
| POST | `/ontology/strength`              | `SetStrengthRequest`                                                   | `204 No Content`              | Recalibrate `prior_strength` for one proposition; audit-logged                                          |
| POST | `/ontology/deprecate`             | `DeprecateRequest`                                                     | `204 No Content`              | Soft-delete a proposition (Reasoner skips deprecated edges; descriptor stays in storage for audit)     |
| POST | `/ontology/construct`             | `AddConstructRequest`                                                  | `204 No Content`              | Append a new construct (append-only; constructs are domain-stable)                                     |
| POST | `/ontology/proposition`           | `AddPropositionRequest` (`direction` is `"+"` or `"-"`)                | `204 No Content`              | Add a validated proposition; `ValidateProposition` rejects direction contradictions                    |
| POST | `/agent/reset`                    | `ResetRequest`                                                         | `204 No Content`              | Reset the EMA for a `(from, to)` pair back to its prior — does not delete the edge                     |
| POST | `/candidates/{id}/confirm`        | path only                                                              | `204 No Content`              | Promote a proposer candidate to a validated proposition                                                |
| POST | `/candidates/{id}/reject`         | path only                                                              | `204 No Content`              | Permanently suppress a candidate within the session                                                    |
| POST | `/candidates/{id}/defer`          | path only                                                              | `204 No Content`              | Keep the candidate pending; re-surface on next review                                                  |
| GET  | `/ui/...`                         | —                                                                      | static assets                 | Embedded HTML/JS/CSS for the viewer; served by `http.FileServer` over an `embed.FS` sub-tree            |

Errors on the new endpoints follow a single shape:

```json
{"error": "Content-Type must be application/json"}
```

`writeError` (in `cmd/agent/routes.go`) is the only path to a non-2xx response. The five pre-existing endpoints retain `http.Error`'s plain-text body for backward compatibility.

### CSRF mitigation: `requireJSON`

There is no auth in v1 — the daemon is intended for lab-network localhost. To stop a malicious page in a browser from issuing cross-origin mutations against a daemon on `localhost:8080`, every body-bearing POST handler calls `requireJSON(r)` and rejects requests whose `Content-Type` is not `application/json`. Browsers do not send that header on simple cross-origin form posts, so a CSRF attempt fails the Content-Type check before reaching the facade. The path-only candidate-review endpoints (`/candidates/{id}/{confirm,reject,defer}`) skip the check because they take no body; the candidate ID being unguessable in practice (UUID-shaped) is the mitigation.

This is sufficient for the v1 threat model. When the agent grows beyond localhost, a token-based auth layer is the next step (tracked in the plan's "Out of scope for v1" section).

### Direction on the wire: `"+"` vs `"-"`

`types.Direction` is a Go `int` internally (0 / 1). The DTO layer in `cmd/agent/dto.go` converts it to `"+"` (positive) and `"-"` (negative) before JSON serialization. Raw integers would render unreadably in CLI tables and UI legends; the string form preserves the publication notation. Mappers — `edgeToDTO`, `propositionToDTO`, `constructToDTO`, `eventToDTO` — are the only places conversion happens.

### Static UI: `embed.FS` with no explicit redirect

`cmd/agent/static.go` declares `//go:embed all:static` and exposes the sub-tree under `/ui/` via `http.FileServer(http.FS(sub))`. The `all:` prefix is required so dot-prefixed files (e.g. `.gitkeep`) are bundled into the binary.

There is no explicit `/ui/{$}` → `/ui/index.html` redirect. `http.FileServer` already serves `index.html` for directory roots that end in `/`, and it independently canonicalizes any URL ending in `/index.html` back to `./`. The two behaviors compose into an infinite redirect loop if you also add a manual `/ui/{$}` → `/ui/index.html` handler — which is what the v0 expansion did, and what hotfix `edffaa3` removed. The rule is: trust the file server, do not redirect.

### The `mapctl` CLI

`cmd/mapctl/` is a separate Go binary that speaks the same HTTP API. It exists for three reasons:

1. **Scripting.** `--json` makes every subcommand emit a parseable payload, suitable for Bash pipelines and CI checks.
2. **Demo control.** Subcommands map one-to-one to mutations the UI offers, so a recorded terminal session is a deterministic alternative to a click-through.
3. **Headless ops.** RPi4 nodes often lack a browser; the CLI is the only operator surface there.

| Subcommand                          | Wraps                                       | Notes                                                       |
| ----------------------------------- | ------------------------------------------- | ----------------------------------------------------------- |
| `graph`                             | `GET /graph`                                | Default table; `--json` for raw                             |
| `edges --from --to`                 | `GET /edges`                                | Multigraph: returns both edges for RC→PS, CO→RR, CE→MU      |
| `history --since`                   | `GET /history`                              | RFC3339 or duration                                         |
| `strength <id> <value>`             | `POST /ontology/strength`                   | Recalibrate one proposition                                 |
| `deprecate <id> <reason>`           | `POST /ontology/deprecate`                  | Soft-delete                                                 |
| `construct add <id> <name> <desc>`  | `POST /ontology/construct`                  |                                                             |
| `proposition add <id> <f> <t> ±<s>` | `POST /ontology/proposition`                |                                                             |
| `reset <from> <to>`                 | `POST /agent/reset`                         | EMA → prior                                                 |
| `candidates [list|confirm|reject|defer]` | `GET/POST /candidates*`                 |                                                             |
| `recommend` / `simulate`            | the corresponding POST                      | Existing endpoints                                          |
| `watch graph|edges`                 | polled GET                                  | 2s ticker; clear-screen unless `--no-color`                 |
| `dot`                               | `GET /graph` → Graphviz                     | Direct paste into `dot -Tpdf`                               |
| `health` / `version`                | `GET /healthz` / `GET /version`             | `version` also prints client build                          |

DTOs are duplicated in `cmd/mapctl/client/types.go` rather than imported from `cmd/agent` — the duplication is the contract. Treating the daemon as a remote service from day one means a third party can write a Python or Rust client without reverse-engineering Go types.

Dependencies: `github.com/spf13/cobra v1.8.1`, `github.com/olekukonko/tablewriter v0.0.5`. `tablewriter` is pinned below 1.x because the 1.x API revamp is breaking.

### The web viewer

`/ui/` serves a single-page application:

- `index.html` — markup: header (title + healthz dot + refresh), Cytoscape mount, side panel, one `<dialog>` modal, toast container
- `app.js` — controller: fetches `/graph`, builds the Cytoscape model, renders the side panel from selection events, POSTs mutations back through the same API
- `style.css` — visual rules: edge color by direction (green `+`, red `−`), opacity proportional to confidence, dashed when deprecated

Cytoscape.js 3.28.1 is loaded from `unpkg.com` (single CDN pin, no build chain). The built-in `cose` layout is sufficient for seven nodes — no extension packages needed.

Mutation flow (single edge):

1. User clicks edge → side panel populates from cached graph state and a filtered `/history` fetch
2. User clicks Deprecate / Set strength / Reset → the same `<dialog>` opens with class swaps providing the appropriate input
3. Submit → `fetch(..., {method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify(req)})`
4. `204` → success toast → re-fetch `/graph` → Cytoscape redraws (dashed edge for Deprecate, opacity/weight change for Strength, EMA fields reset for Reset)
5. non-2xx → toast with the server's `{"error":"..."}` message

There is no auto-poll by default; an opt-in checkbox enables a 5-second refresh tick. Same-origin only — no CORS configuration.

### Why three layers, not one

Each layer answers a different question:

- **HTTP API** answers: "what can the agent do, expressed in JSON?"
- **CLI** answers: "what can an operator do from a script, expressed in subcommands?"
- **Web UI** answers: "what can a reviewer see and click, expressed visually?"

Collapsing them — e.g. embedding HTML rendering inside Go handlers, or building a TUI that calls `pkg/semmap` directly — would couple the daemon to its consumers. Keeping the HTTP boundary firm means the Netdata adapter (step 5 in `research-docs/SEMANTIC-MAP-STATUS.md`) can land without touching the control surface, and any future client (mobile, TUI, fleet view) can be added without changing the daemon.
