// Package semmap provides the SemanticMap facade.
// Agent code imports only this package — never contract implementations directly.
package semmap

import (
	"time"

	"github.com/DiyazY/di-agent/pkg/contracts"
	"github.com/DiyazY/di-agent/pkg/types"
)

// SemanticMap wires the five contracts and exposes the agent API.
type SemanticMap struct {
	storage  contracts.StorageContract
	ontology contracts.OntologyContract
	updater  contracts.UpdaterContract
	reasoner contracts.ReasonerContract
	proposer contracts.ProposerContract
}

func New(
	storage contracts.StorageContract,
	ontology contracts.OntologyContract,
	updater contracts.UpdaterContract,
	reasoner contracts.ReasonerContract,
	proposer contracts.ProposerContract,
) *SemanticMap {
	return &SemanticMap{storage, ontology, updater, reasoner, proposer}
}

// ── Agent queries ─────────────────────────────────────────────────────────────

func (m *SemanticMap) CostOfAction(taskType, nodeID string) (*types.ActionCost, error) {
	return m.reasoner.CostOfAction(taskType, nodeID)
}

func (m *SemanticMap) RecommendPeer(ctx *types.OffloadContext) (*types.PeerRecommendation, error) {
	return m.reasoner.RecommendPeer(ctx)
}

func (m *SemanticMap) SimulateOutcome(ctx *types.OffloadContext, targetNodeID string) (*types.OutcomeSimulation, error) {
	return m.reasoner.SimulateOutcome(ctx, targetNodeID)
}

// ── Telemetry ingestion ───────────────────────────────────────────────────────

// Ingest feeds one telemetry observation into the evidence layer.
// It updates the edge descriptor and notifies the proposer.
func (m *SemanticMap) Ingest(fromID, toID string, observation float64, eventID string) error {
	if _, err := m.updater.UpdateEdge(fromID, toID, observation, eventID); err != nil {
		return err
	}
	return m.proposer.Observe(fromID, toID, observation, observation)
}

// IngestSample feeds one MetricSample through the Bridge. The Bridge maps the
// metric type to its primary construct, looks up every relationship that
// touches that construct, and calls UpdateEdge on each unique (from, to)
// pair. Idempotency is per-edge — replaying the same sample is a no-op.
//
// Returns nil even when the metric type has no mapping (forward-compat with
// future MetricTypes). Per-edge errors are returned (first one wins) so the
// caller can decide whether to keep looping; the Bridge itself processes
// every reachable edge regardless of individual failures.
func (m *SemanticMap) IngestSample(sample *types.MetricSample) error {
	return Bridge(sample, m.ontology, m.updater)
}

// ── Graph extension ───────────────────────────────────────────────────────────

func (m *SemanticMap) PendingCandidates() ([]*types.CandidateEdge, error) {
	return m.proposer.GetCandidates()
}

func (m *SemanticMap) ConfirmCandidate(candidateID string) error {
	return m.proposer.Confirm(candidateID)
}

func (m *SemanticMap) RejectCandidate(candidateID string) error {
	return m.proposer.Reject(candidateID)
}

func (m *SemanticMap) DeferCandidate(candidateID string) error {
	return m.proposer.Defer(candidateID)
}

// ── Read-side facade (introspection) ──────────────────────────────────────────
//
// These pass-throughs expose graph state to transports (HTTP, CLI, UI). They
// intentionally live on the facade — many small methods over one mega-Snapshot
// — so the package stays transport-agnostic and easy to consume.

// Constructs returns every construct currently registered in the ontology.
func (m *SemanticMap) Constructs() ([]*types.Construct, error) {
	return m.ontology.Constructs()
}

// Propositions returns every proposition (including deprecated ones, flagged
// via Proposition.Deprecated).
func (m *SemanticMap) Propositions() ([]*types.Proposition, error) {
	return m.ontology.Propositions()
}

// AllEdges returns every edge descriptor currently held in storage.
func (m *SemanticMap) AllEdges() ([]*types.EdgeDescriptor, error) {
	return m.storage.AllEdges()
}

// EdgesByPair returns every edge between (from, to). Conflict-pair endpoints
// (e.g. RC→PS for P2/P3) yield more than one descriptor.
func (m *SemanticMap) EdgesByPair(from, to string) ([]*types.EdgeDescriptor, error) {
	return m.storage.GetEdgesByPair(from, to)
}

// Neighbors returns the set of construct IDs reachable from nodeID via one
// outgoing edge.
func (m *SemanticMap) Neighbors(nodeID string) ([]string, error) {
	return m.storage.Neighbors(nodeID)
}

// History returns ontology mutation events appended at or after `since`.
// Pass the zero time.Time to retrieve the full audit log.
func (m *SemanticMap) History(since time.Time) ([]*types.OntologyEvent, error) {
	return m.ontology.GetHistory(since)
}

// ── Write-side facade (ontology mutations) ────────────────────────────────────

// SetPropositionStrength recalibrates the prior strength of an existing
// proposition and appends an event to the audit log.
func (m *SemanticMap) SetPropositionStrength(id string, strength float64) error {
	return m.ontology.SetPropositionStrength(id, strength)
}

// Deprecate marks a proposition as no-longer-endorsed (soft delete).
// Reasoners must skip deprecated propositions during cost computation.
func (m *SemanticMap) Deprecate(id, reason string) error {
	return m.ontology.Deprecate(id, reason)
}

// AddConstruct appends a new construct to the ontology.
func (m *SemanticMap) AddConstruct(c *types.Construct) error {
	return m.ontology.AddConstruct(c)
}

// AddValidatedProposition appends a new proposition after the ontology has
// validated it against the existing backbone.
func (m *SemanticMap) AddValidatedProposition(p *types.Proposition) error {
	return m.ontology.AddValidatedProposition(p)
}

// ResetEdge restores every edge between (from, to) to its prior state.
func (m *SemanticMap) ResetEdge(from, to string) error {
	return m.updater.Reset(from, to)
}
