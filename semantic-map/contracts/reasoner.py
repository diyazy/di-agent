"""
Reasoner contract — produce agent decisions with traceable rationales.

Behavioral guarantees every implementation must uphold:
  - Traceable rationale: every returned object must include a non-empty
    rationale string that references specific node/edge ids from the graph
    traversal path used to produce the decision. Implementations that cannot
    produce a rationale must raise NotImplementedError rather than return
    an empty string.
  - Read-only simulation: simulate_outcome never writes to Storage or modifies
    any contract's state. It is a pure query.
  - Trust filtering: recommend_peer must never return a peer whose trust score
    falls below the minimum threshold supplied at construction time. If no peer
    meets the threshold, it must raise InsufficientTrustError rather than
    return a low-trust recommendation.
"""

from __future__ import annotations

from abc import ABC, abstractmethod

from ..types import ActionCost, OffloadContext, OutcomeSimulation, PeerRecommendation


class InsufficientTrustError(Exception):
    """Raised when no peer meets the minimum trust threshold for offloading."""


class ReasonerContract(ABC):

    @abstractmethod
    def cost_of_action(self, task_type: str, node_id: str) -> ActionCost:
        """
        Estimate the cost of executing task_type on node_id using the current
        state of the Semantic Map (blended prior + evidence).
        """

    @abstractmethod
    def recommend_peer(self, context: OffloadContext) -> PeerRecommendation:
        """
        Given the current node's state and the offload context, identify the
        best peer for task offloading. Raises InsufficientTrustError if no
        peer meets the minimum trust threshold.
        """

    @abstractmethod
    def simulate_outcome(
        self, context: OffloadContext, target_node_id: str
    ) -> OutcomeSimulation:
        """
        Pre-flight simulation of offloading context.task_type to target_node_id.
        Returns expected costs, P95 estimates (if Gaussian descriptors are
        available), and any risk flags. Does not modify state.
        """
