// Package types defines the shared data structures that cross contract boundaries.
// No contract implementation may define its own wire types — all must use these.
package types

// Direction encodes the sign of a proposition edge.
type Direction int

const (
	Positive Direction = iota // source construct increases target construct
	Negative                  // source construct decreases target construct
)

// CandidateStatus tracks the review state of a proposed backbone edge.
type CandidateStatus int

const (
	Pending   CandidateStatus = iota
	Confirmed                 // added to ontology backbone
	Rejected                  // suppressed for this deployment session
	Deferred                  // re-evaluate after more observations
)

// ── Graph primitives ──────────────────────────────────────────────────────────

type NodeDescriptor struct {
	NodeID        string
	ConstructType string
	PriorValue    float64
	EMAValue      float64
	Confidence    float64 // 0.0 = prior-dominated, 1.0 = evidence-dominated
	NObservations int
}

type EdgeDescriptor struct {
	FromID        string
	ToID          string
	PropositionID string
	Direction     Direction
	PriorWeight   float64
	EMAWeight     float64
	Confidence    float64
	NObservations int
	Mu            *float64 // Gaussian mean  (edge-standard profile+); nil if unavailable
	Sigma         *float64 // Gaussian std   (edge-standard profile+); nil if unavailable
}

// ── Ontology primitives ───────────────────────────────────────────────────────

type Construct struct {
	ConstructID string
	Name        string
	Description string
}

type Proposition struct {
	PropositionID  string
	FromConstruct  string
	ToConstruct    string
	Direction      Direction
	PriorStrength  float64
	EvidenceSources []string // e.g. ["P1", "P4"]
}

type ValidationResult struct {
	Valid     bool
	Conflicts []string // proposition IDs that contradict the proposed edge
	Warnings  []string
}

// ── Agent query types ─────────────────────────────────────────────────────────

type OffloadContext struct {
	TaskType            string
	SourceNodeID        string
	DataSizeBytes       int64
	LatencyBudgetMs     float64
	EnergyBudgetJoules  *float64 // nil = unconstrained
}

type ActionCost struct {
	CPUCost         float64
	EnergyCost      float64
	LatencyEstimate float64
	Confidence      float64
	Rationale       string   // must reference specific node/edge IDs
	GraphPathUsed   []string
}

type PeerRecommendation struct {
	PeerID          string
	ExpectedSavings float64
	Rationale       string
	GraphPathUsed   []string
}

type OutcomeSimulation struct {
	ExpectedLatency float64
	ExpectedEnergy  float64
	Confidence      float64
	GraphPathUsed   []string
	P95Latency      *float64 // nil if Gaussian descriptors unavailable
	P95Energy       *float64
	RiskFlags       []string
}

// ── Collector types ───────────────────────────────────────────────────────────

// MetricType is the semantic kind of an observation emitted by a collector.
// Values are fixed — collectors must normalize raw source data to these units:
//
//	CPUUtilization       fraction [0,1]  CPU quota consumed
//	MemoryUtilization    fraction [0,1]  memory limit consumed
//	CPUThrottleRatio     fraction [0,1]  scheduling periods throttled
//	BlockIOUtil          fraction [0,1]  block I/O bandwidth consumed
//	PodStartupMs         milliseconds    pod creation → running
//	SchedulingLatencyMs  milliseconds    pod pending → scheduled
//	NetworkRxBps         bytes/sec       receive throughput
//	NetworkTxBps         bytes/sec       transmit throughput
//	NetworkLossRatio     fraction [0,1]  packet loss
//	NetworkLatencyMs     milliseconds    RTT to a peer node
//	EnergyJoules         joules          energy in the sample interval
type MetricType string

const (
	CPUUtilization      MetricType = "cpu_utilization"
	MemoryUtilization   MetricType = "memory_utilization"
	CPUThrottleRatio    MetricType = "cpu_throttle_ratio"
	BlockIOUtil         MetricType = "block_io_util"
	PodStartupMs        MetricType = "pod_startup_ms"
	SchedulingLatencyMs MetricType = "scheduling_latency_ms"
	NetworkRxBps        MetricType = "network_rx_bps"
	NetworkTxBps        MetricType = "network_tx_bps"
	NetworkLossRatio    MetricType = "network_loss_ratio"
	NetworkLatencyMs    MetricType = "network_latency_ms"
	EnergyJoules        MetricType = "energy_joules"
)

// MetricSample is one normalized observation emitted by a CollectorContract.
//
// EventID must be deterministic: the same physical observation (same SourceID,
// NodeID, ContainerID, MetricType, and TimestampUnix) must produce the same
// EventID across calls and restarts, so that the Updater's idempotency
// guarantee holds end-to-end.
//
// ContainerID is empty for node-level aggregates.
// Labels carries source-specific metadata and is informational only.
type MetricSample struct {
	NodeID        string
	MetricType    MetricType
	Value         float64
	TimestampUnix int64
	EventID       string
	ContainerID   string            // empty = node-level aggregate
	Labels        map[string]string // informational; bridge must not branch on these
}

// ── Proposer types ────────────────────────────────────────────────────────────

type CandidateEdge struct {
	CandidateID     string
	FromID          string
	ToID            string
	Direction       Direction
	MIScore         float64
	PValue          float64
	NObservations   int
	DeploymentsSeen int
	Status          CandidateStatus
}
