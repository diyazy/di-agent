package minimal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/DiyazY/di-agent/pkg/contracts"
	"github.com/DiyazY/di-agent/pkg/types"
)

// MICorrelationProposer is a demo-grade ProposerContract implementation.
//
// It maintains a fixed-size ring buffer of (valueA, valueB) observations per
// construct pair. Once `minPairs` samples accumulate, it computes the Pearson
// correlation coefficient over the window. If |r| > threshold AND no existing
// proposition between the pair exists AND the candidate was not previously
// rejected, it emits a CandidateEdge.
//
// Pearson correlation stands in for mutual information here — it captures
// linear dependence cheaply and deterministically. The name "MI" is kept for
// continuity with the literature on automatic graph extension; a richer
// edge-standard or cloud-full profile may swap in true mutual-information
// estimation without changing the interface.
//
// p-value computation is intentionally stubbed (PValue=0) — this is a demo
// proposer, not a statistical test harness. Operators should treat the score
// alone as the relevance signal.
type MICorrelationProposer struct {
	ontology contracts.OntologyContract

	mu         sync.Mutex
	buffers    map[string]*pairBuffer         // key: fromID + "→" + toID
	candidates map[string]*types.CandidateEdge // key: CandidateID — holds the LATEST status
	order      []string                        // insertion order of CandidateIDs, for stable history iteration

	threshold float64 // |Pearson r| trigger to emit a candidate
	minPairs  int     // minimum buffered samples before evaluating
	bufSize   int     // ring buffer capacity
}

type pairBuffer struct {
	a, b  []float64
	pos   int
	count int // total samples ever buffered (capped at bufSize)
}

// NewMICorrelationProposer builds an MICorrelationProposer.
//
//	threshold: |Pearson r| above which a candidate is emitted (e.g. 0.8)
//	minPairs:  observations required before correlation is computed
//	bufSize:   ring buffer capacity per pair; larger windows are more stable
//	            but slower to react
func NewMICorrelationProposer(
	ontology contracts.OntologyContract,
	threshold float64,
	minPairs, bufSize int,
) *MICorrelationProposer {
	if minPairs < 3 {
		minPairs = 3 // Pearson is undefined for n < 2; require ≥3 for stability
	}
	if bufSize < minPairs {
		bufSize = minPairs
	}
	return &MICorrelationProposer{
		ontology:   ontology,
		buffers:    make(map[string]*pairBuffer),
		candidates: make(map[string]*types.CandidateEdge),
		threshold:  threshold,
		minPairs:   minPairs,
		bufSize:    bufSize,
	}
}

// Observe appends a (valueA, valueB) pair to the ring buffer for
// (fromID, toID), then re-evaluates correlation if the buffer has enough
// samples. Emission rules:
//
//   - If a non-deprecated proposition already exists between (fromID, toID)
//     in either direction (regardless of sign), no candidate is emitted —
//     the backbone already covers this pair.
//   - If a previously rejected candidate exists for this pair, it is not
//     re-emitted within the session (permanent suppression per the contract).
//   - Otherwise, if |r| > threshold, a CandidateEdge is created or updated
//     in-place (idempotent on the deterministic CandidateID).
func (p *MICorrelationProposer) Observe(fromID, toID string, valueA, valueB float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := fromID + "→" + toID
	buf, ok := p.buffers[key]
	if !ok {
		buf = &pairBuffer{a: make([]float64, p.bufSize), b: make([]float64, p.bufSize)}
		p.buffers[key] = buf
	}
	buf.a[buf.pos] = valueA
	buf.b[buf.pos] = valueB
	buf.pos = (buf.pos + 1) % p.bufSize
	if buf.count < p.bufSize {
		buf.count++
	}

	if buf.count < p.minPairs {
		return nil
	}

	r := pearson(buf.a[:buf.count], buf.b[:buf.count])
	if math.IsNaN(r) || math.Abs(r) < p.threshold {
		return nil
	}

	direction := types.Positive
	if r < 0 {
		direction = types.Negative
	}

	// Backbone coverage check: skip if a non-deprecated proposition with the
	// same (from, to, direction) already exists. The multigraph permits a
	// conflict-pair sibling (opposite direction) to be proposed separately —
	// this is how the Di-Select P2/P3, P5/P6, P7/P9 conflict pairs would be
	// discovered if they were not part of the bootstrap.
	covered, err := p.pairAlreadyCovered(fromID, toID, direction)
	if err != nil {
		return err
	}
	if covered {
		return nil
	}

	candID := "P-prop-" + fromID + "-" + toID

	// Suppression check: never re-emit a rejected candidate.
	if existing, ok := p.candidates[candID]; ok && existing.Status == types.Rejected {
		return nil
	}

	if existing, ok := p.candidates[candID]; ok && existing.Status == types.Pending {
		// Idempotent refresh of an already-pending candidate — update score
		// and observation count without flipping direction (a previously
		// emitted positive candidate that now sees negative correlation
		// keeps its original direction; operators see the up-to-date score).
		existing.MIScore = math.Abs(r)
		existing.NObservations = buf.count
		return nil
	}

	cand := &types.CandidateEdge{
		CandidateID:   candID,
		FromID:        fromID,
		ToID:          toID,
		Direction:     direction,
		MIScore:       math.Abs(r),
		PValue:        0, // stub — see package doc
		NObservations: buf.count,
		Status:        types.Pending,
	}
	p.candidates[candID] = cand
	p.order = append(p.order, candID)
	return nil
}

// GetCandidates returns Pending candidates sorted by CandidateID for stable
// output.
func (p *MICorrelationProposer) GetCandidates() ([]*types.CandidateEdge, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*types.CandidateEdge, 0, len(p.candidates))
	for _, c := range p.candidates {
		if c.Status == types.Pending {
			out = append(out, copyCandidate(c))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CandidateID < out[j].CandidateID })
	return out, nil
}

// Confirm promotes a Pending candidate into the ontology backbone via
// AddValidatedProposition. The synthesized PropositionID is
//
//	"P-" + hex(sha256(CandidateID))[:8]
//
// so the same candidate always lands the same proposition ID across replays.
// On success the candidate's Status is flipped to Confirmed and a history
// entry is appended.
func (p *MICorrelationProposer) Confirm(candidateID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.candidates[candidateID]
	if !ok {
		return fmt.Errorf("candidate %q not found", candidateID)
	}
	if c.Status != types.Pending {
		return fmt.Errorf("candidate %q is not Pending (status=%v)", candidateID, c.Status)
	}

	propID := "P-" + synthesizePropSuffix(candidateID)
	prop := &types.Proposition{
		PropositionID:   propID,
		FromConstruct:   c.FromID,
		ToConstruct:     c.ToID,
		Direction:       c.Direction,
		PriorStrength:   c.MIScore,
		Description:     fmt.Sprintf("Auto-proposed by MICorrelationProposer (|r|=%.3f, n=%d)", c.MIScore, c.NObservations),
		EvidenceSources: []string{"proposer-mi"},
	}
	if err := p.ontology.AddValidatedProposition(prop); err != nil {
		return err
	}
	c.Status = types.Confirmed
	return nil
}

// Reject marks a candidate as permanently suppressed for this session.
// The candidate stays in the map so subsequent Observe calls on the same
// pair are short-circuited; a history entry is appended.
func (p *MICorrelationProposer) Reject(candidateID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.candidates[candidateID]
	if !ok {
		return fmt.Errorf("candidate %q not found", candidateID)
	}
	c.Status = types.Rejected
	return nil
}

// Defer marks a candidate as Deferred — it moves out of GetCandidates but
// remains in the candidates map. In this v1 implementation Defer behaves
// like a weaker form of Reject: re-Observe on the same pair will not
// re-emit while the deferred entry is present. A richer profile may
// re-promote deferred candidates after a fresh evidence cycle; not yet.
func (p *MICorrelationProposer) Defer(candidateID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.candidates[candidateID]
	if !ok {
		return fmt.Errorf("candidate %q not found", candidateID)
	}
	c.Status = types.Deferred
	return nil
}

// GetHistory returns one entry per candidate the proposer has ever emitted,
// reflecting the candidate's current status (Pending / Confirmed / Rejected /
// Deferred). Order matches insertion (first-seen first). This is the audit
// surface — every candidate that has existed appears here exactly once with
// its lifecycle endpoint.
func (p *MICorrelationProposer) GetHistory() ([]*types.CandidateEdge, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*types.CandidateEdge, 0, len(p.order))
	for _, id := range p.order {
		if c, ok := p.candidates[id]; ok {
			out = append(out, copyCandidate(c))
		}
	}
	return out, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// pairAlreadyCovered reports whether the ontology backbone already has a
// non-deprecated proposition connecting (fromID, toID) with the same
// direction. Conflict-pair siblings (opposite direction on the same pair)
// are intentionally NOT considered covered — the multigraph allows them.
func (p *MICorrelationProposer) pairAlreadyCovered(fromID, toID string, dir types.Direction) (bool, error) {
	props, err := p.ontology.Propositions()
	if err != nil {
		return false, err
	}
	for _, prop := range props {
		if prop.Deprecated {
			continue
		}
		if prop.FromConstruct == fromID && prop.ToConstruct == toID && prop.Direction == dir {
			return true, nil
		}
	}
	return false, nil
}

// pearson computes the Pearson correlation coefficient over equal-length
// slices. Returns NaN if either input has zero variance.
func pearson(xs, ys []float64) float64 {
	n := len(xs)
	if n < 2 || n != len(ys) {
		return math.NaN()
	}
	var sumX, sumY float64
	for i := 0; i < n; i++ {
		sumX += xs[i]
		sumY += ys[i]
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)

	var num, denX, denY float64
	for i := 0; i < n; i++ {
		dx := xs[i] - meanX
		dy := ys[i] - meanY
		num += dx * dy
		denX += dx * dx
		denY += dy * dy
	}
	if denX == 0 || denY == 0 {
		return math.NaN()
	}
	return num / math.Sqrt(denX*denY)
}

func synthesizePropSuffix(candidateID string) string {
	h := sha256.Sum256([]byte(candidateID))
	return hex.EncodeToString(h[:4]) // 8 hex chars
}

func copyCandidate(c *types.CandidateEdge) *types.CandidateEdge {
	cp := *c
	return &cp
}
