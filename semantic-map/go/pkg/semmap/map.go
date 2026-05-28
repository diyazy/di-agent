// Package semmap provides the SemanticMap facade.
// Agent code imports only this package — never contract implementations directly.
package semmap

import (
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
