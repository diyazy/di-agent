"""
SemanticMap — the agent-facing facade.

Wires the five contracts together and exposes the three agent queries.
Agent code imports only this class; it never touches contract implementations.
"""

from __future__ import annotations

from .contracts import (
    OntologyContract,
    ProposerContract,
    ReasonerContract,
    StorageContract,
    UpdaterContract,
)
from .types import ActionCost, CandidateEdge, OffloadContext, OutcomeSimulation, PeerRecommendation


class SemanticMap:

    def __init__(
        self,
        storage: StorageContract,
        ontology: OntologyContract,
        updater: UpdaterContract,
        reasoner: ReasonerContract,
        proposer: ProposerContract,
    ) -> None:
        self._storage  = storage
        self._ontology = ontology
        self._updater  = updater
        self._reasoner = reasoner
        self._proposer = proposer

    # ── Agent queries ─────────────────────────────────────────────────────────

    def cost_of_action(self, task_type: str, node_id: str) -> ActionCost:
        """Estimate the cost of executing task_type on node_id."""
        return self._reasoner.cost_of_action(task_type, node_id)

    def recommend_peer(self, context: OffloadContext) -> PeerRecommendation:
        """Identify the best peer for offloading the given task context."""
        return self._reasoner.recommend_peer(context)

    def simulate_outcome(
        self, context: OffloadContext, target_node_id: str
    ) -> OutcomeSimulation:
        """Pre-flight simulation before committing an offload decision."""
        return self._reasoner.simulate_outcome(context, target_node_id)

    # ── Telemetry ingestion ───────────────────────────────────────────────────

    def ingest(self, from_id: str, to_id: str, observation: float, event_id: str) -> None:
        """
        Feed a telemetry observation into the evidence layer.
        Updates the edge descriptor and notifies the Proposer.
        """
        self._updater.update_edge(from_id, to_id, observation, event_id)
        self._proposer.observe(from_id, to_id, observation, observation)

    # ── Graph extension ───────────────────────────────────────────────────────

    def pending_candidates(self) -> list[CandidateEdge]:
        """Return candidate edges waiting for operator review."""
        return self._proposer.get_candidates()

    def confirm_candidate(self, candidate_id: str) -> None:
        self._proposer.confirm(candidate_id)

    def reject_candidate(self, candidate_id: str) -> None:
        self._proposer.reject(candidate_id)

    def defer_candidate(self, candidate_id: str) -> None:
        self._proposer.defer(candidate_id)
