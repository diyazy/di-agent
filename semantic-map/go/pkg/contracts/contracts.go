// Package contracts defines the five interface contracts of the Semantic Map.
//
// Behavioral guarantees are documented on each interface. All implementations
// must satisfy these guarantees and pass the shared compliance suite in
// github.com/DiyazY/di-agent/compliance.
package contracts

import "github.com/DiyazY/di-agent/pkg/types"

// ── Storage ───────────────────────────────────────────────────────────────────

// StorageContract persists node and edge descriptors.
//
// Guarantees:
//   - Atomicity: Put operations either fully succeed or leave prior state intact.
//   - Nil safety: Get operations return (nil, nil) for unknown IDs; they never
//     return a non-nil error for a simple miss.
//   - Empty-safe: Neighbors and AllEdges return empty slices, never nil.
type StorageContract interface {
	GetNode(nodeID string) (*types.NodeDescriptor, error)
	PutNode(d *types.NodeDescriptor) error
	GetEdge(fromID, toID string) (*types.EdgeDescriptor, error)
	PutEdge(d *types.EdgeDescriptor) error
	Neighbors(nodeID string) ([]string, error)
	AllEdges() ([]*types.EdgeDescriptor, error)
}

// ── Ontology ──────────────────────────────────────────────────────────────────

// OntologyContract answers structural questions about constructs and propositions.
//
// Guarantees:
//   - Bootstrap minimum: always returns at least the 7 Di-Select constructs
//     and 15 propositions, regardless of runtime extensions.
//   - Immutability: existing validated propositions are never removed or reversed.
//   - Pure query: ValidateProposition never modifies state.
type OntologyContract interface {
	Constructs() ([]*types.Construct, error)
	Propositions() ([]*types.Proposition, error)
	Relationships(constructID string) ([]*types.Proposition, error)
	ValidateProposition(fromID, toID string, dir types.Direction) (*types.ValidationResult, error)
	AddValidatedProposition(p *types.Proposition) error
}

// ── Updater ───────────────────────────────────────────────────────────────────

// UpdaterContract incorporates telemetry into edge and node descriptors.
//
// Guarantees:
//   - Idempotency: calling UpdateEdge or UpdateNode twice with the same eventID
//     leaves stored state identical to what it was after the first call.
//   - No-panic on valid input: out-of-range observations are clipped silently.
//   - Reset semantics: Reset restores EMAWeight = PriorWeight, NObservations = 0,
//     Confidence = 0.0 without deleting the edge.
type UpdaterContract interface {
	UpdateEdge(fromID, toID string, observation float64, eventID string) (*types.EdgeDescriptor, error)
	UpdateNode(nodeID string, observation float64, eventID string) (*types.NodeDescriptor, error)
	Reset(fromID, toID string) error
}

// ── Reasoner ──────────────────────────────────────────────────────────────────

// ReasonerContract produces agent decisions with traceable rationales.
//
// Guarantees:
//   - Traceable rationale: every returned value includes a non-empty Rationale
//     string referencing specific node/edge IDs. Implementations that cannot
//     produce a rationale must return ErrNoRationale.
//   - Pure simulation: SimulateOutcome never writes to Storage or any contract.
//   - Trust filtering: RecommendPeer never returns a peer below the minimum
//     trust threshold; returns ErrInsufficientTrust if no peer qualifies.
type ReasonerContract interface {
	CostOfAction(taskType, nodeID string) (*types.ActionCost, error)
	RecommendPeer(ctx *types.OffloadContext) (*types.PeerRecommendation, error)
	SimulateOutcome(ctx *types.OffloadContext, targetNodeID string) (*types.OutcomeSimulation, error)
}

// Sentinel errors returned by ReasonerContract implementations.
var (
	ErrNoRationale      = contractError("reasoner must provide a non-empty rationale")
	ErrInsufficientTrust = contractError("no peer meets the minimum trust threshold")
)

// ── Proposer ──────────────────────────────────────────────────────────────────

// ProposerContract detects statistical patterns suggesting new backbone edges.
//
// Guarantees:
//   - Read-only observation: Observe never modifies Storage or Ontology.
//   - Confirm delegates: Confirm calls OntologyContract.AddValidatedProposition;
//     it never writes to Storage directly.
//   - Permanent suppression: after Reject, the same (fromID, toID, direction)
//     triple is not re-proposed within the current deployment session.
//   - Candidates: GetCandidates returns only Pending entries.
type ProposerContract interface {
	Observe(fromID, toID string, valueA, valueB float64) error
	GetCandidates() ([]*types.CandidateEdge, error)
	Confirm(candidateID string) error
	Reject(candidateID string) error
	Defer(candidateID string) error
	GetHistory() ([]*types.CandidateEdge, error)
}

// ── Collector ─────────────────────────────────────────────────────────────────

// CollectorContract reads raw metrics from a source and emits normalized samples.
//
// The collector sits between a metric source (cgroup filesystem, Netdata HTTP API,
// kubelet /metrics, etc.) and the Updater. It normalizes observations into
// MetricSamples. It knows nothing about the graph topology — that mapping is
// the bridge's responsibility.
//
// Guarantees:
//   - Pure read: Collect never modifies any system state.
//   - Empty on no data: Collect returns ([], nil) when no new samples are ready;
//     it never returns a non-nil error for a temporarily unavailable source.
//   - Deterministic EventID: the same physical observation always produces the
//     same EventID, enabling end-to-end idempotency with the Updater.
//   - Metric type stability: AvailableMetrics returns the same set for the
//     entire lifetime of the instance; Collect never emits a MetricType outside it.
//   - Node ID completeness: every emitted MetricSample has a non-empty NodeID.
//   - SourceID stability: SourceID returns the same string across restarts.
type CollectorContract interface {
	// Collect reads one batch of current metric samples from the source.
	// Returns an empty slice (not an error) when no new data is available.
	Collect() ([]*types.MetricSample, error)

	// SourceID returns a stable identifier for this collector instance.
	// Used as a component in EventID generation.
	SourceID() string

	// AvailableMetrics returns the metric types this implementation can produce.
	// The returned slice is static for the lifetime of the instance.
	AvailableMetrics() []types.MetricType
}

// ── helpers ───────────────────────────────────────────────────────────────────

type contractError string

func (e contractError) Error() string { return string(e) }
