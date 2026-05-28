"""
Shared data types for the Semantic Map contract layer.

All contracts communicate exclusively through these types — no
implementation-specific types may cross a contract boundary.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Optional


class Direction(Enum):
    POSITIVE = "positive"   # source construct increases target construct
    NEGATIVE = "negative"   # source construct decreases target construct


class CandidateStatus(Enum):
    PENDING   = "pending"
    CONFIRMED = "confirmed"
    REJECTED  = "rejected"   # permanently suppressed for this deployment session
    DEFERRED  = "deferred"   # re-evaluated after more observations accumulate


# ── Graph primitives ──────────────────────────────────────────────────────────

@dataclass
class NodeDescriptor:
    node_id: str
    construct_type: str
    prior_value: float
    ema_value: float
    confidence: float        # 0.0 = prior-dominated, 1.0 = evidence-dominated
    n_observations: int


@dataclass
class EdgeDescriptor:
    from_id: str
    to_id: str
    proposition_id: str
    direction: Direction
    prior_weight: float
    ema_weight: float
    confidence: float
    n_observations: int
    mu: Optional[float] = None      # Gaussian mean  (edge-standard profile+)
    sigma: Optional[float] = None   # Gaussian std   (edge-standard profile+)


# ── Ontology primitives ───────────────────────────────────────────────────────

@dataclass
class Construct:
    construct_id: str
    name: str
    description: str


@dataclass
class Proposition:
    proposition_id: str
    from_construct: str
    to_construct: str
    direction: Direction
    prior_strength: float           # magnitude of the relationship, from P1–P5 evidence
    evidence_sources: list[str] = field(default_factory=list)  # e.g. ["P1", "P4"]


@dataclass
class ValidationResult:
    valid: bool
    conflicts: list[str] = field(default_factory=list)   # proposition_ids that conflict
    warnings: list[str]  = field(default_factory=list)


# ── Agent query types ─────────────────────────────────────────────────────────

@dataclass
class OffloadContext:
    task_type: str
    source_node_id: str
    data_size_bytes: int
    latency_budget_ms: float
    energy_budget_joules: Optional[float] = None


@dataclass
class ActionCost:
    cpu_cost: float
    energy_cost: float
    latency_estimate: float
    confidence: float
    rationale: str              # must reference specific node/edge ids
    graph_path_used: list[str] = field(default_factory=list)


@dataclass
class PeerRecommendation:
    peer_id: str
    expected_savings: float
    rationale: str
    graph_path_used: list[str] = field(default_factory=list)


@dataclass
class OutcomeSimulation:
    expected_latency: float
    expected_energy: float
    confidence: float
    graph_path_used: list[str] = field(default_factory=list)
    p95_latency: Optional[float] = None     # None if Gaussian not available
    p95_energy: Optional[float]  = None
    risk_flags: list[str]        = field(default_factory=list)


# ── Collector types ───────────────────────────────────────────────────────────

class MetricType(Enum):
    """Semantic metric types that the Semantic Map graph understands.

    Collectors may emit additional implementation-specific types; the bridge
    silently ignores any MetricType it has no mapping for.

    Value units are fixed per type — implementations must normalize to these:
      cpu_utilization      fraction [0, 1]   CPU quota consumed
      memory_utilization   fraction [0, 1]   memory limit consumed
      cpu_throttle_ratio   fraction [0, 1]   scheduling periods throttled
      block_io_util        fraction [0, 1]   block I/O bandwidth consumed
      pod_startup_ms       milliseconds      pod creation → running
      scheduling_latency_ms milliseconds     pod pending → scheduled
      network_rx_bps       bytes/sec         receive throughput
      network_tx_bps       bytes/sec         transmit throughput
      network_loss_ratio   fraction [0, 1]   packet loss
      network_latency_ms   milliseconds      RTT to a peer node
      energy_joules        joules            energy in the sample interval
    """
    CPU_UTILIZATION       = "cpu_utilization"
    MEMORY_UTILIZATION    = "memory_utilization"
    CPU_THROTTLE_RATIO    = "cpu_throttle_ratio"
    BLOCK_IO_UTIL         = "block_io_util"
    POD_STARTUP_MS        = "pod_startup_ms"
    SCHEDULING_LATENCY_MS = "scheduling_latency_ms"
    NETWORK_RX_BPS        = "network_rx_bps"
    NETWORK_TX_BPS        = "network_tx_bps"
    NETWORK_LOSS_RATIO    = "network_loss_ratio"
    NETWORK_LATENCY_MS    = "network_latency_ms"
    ENERGY_JOULES         = "energy_joules"


@dataclass
class MetricSample:
    """One normalized observation emitted by a CollectorContract implementation.

    event_id must be deterministic: the same physical observation (same source,
    node, container, metric type, and timestamp) must always produce the same
    event_id. This allows the Updater's idempotency guarantee to hold end-to-end.

    container_id is empty for node-level aggregates.
    labels carries source-specific metadata (e.g. cgroup path, Netdata chart ID)
    and is informational only — the bridge must not branch on it.
    """
    node_id:       str
    metric_type:   MetricType
    value:         float
    timestamp_unix: int
    event_id:      str
    container_id:  str = ""
    labels:        dict = field(default_factory=dict)


# ── Proposer types ────────────────────────────────────────────────────────────

@dataclass
class CandidateEdge:
    candidate_id: str
    from_id: str
    to_id: str
    direction: Direction
    mi_score: float
    p_value: float
    n_observations: int
    deployments_seen: int
    status: CandidateStatus = CandidateStatus.PENDING
