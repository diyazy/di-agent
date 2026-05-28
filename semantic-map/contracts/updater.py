"""
Updater contract — incorporate new telemetry into edge and node descriptors.

Behavioral guarantees every implementation must uphold:
  - Idempotency: calling update_edge or update_node twice with the same
    event_id must leave the stored state identical to what it was after the
    first call. Implementations must track processed event_ids within a session.
  - No-raise on valid input: update_edge and update_node never raise for
    in-range observations. Out-of-range values are clipped silently;
    implementations must document their accepted ranges.
  - Reset semantics: reset restores ema_value to prior_value, sets
    n_observations to 0 and confidence to 0.0. It never deletes the
    edge/node from storage.
"""

from __future__ import annotations

from abc import ABC, abstractmethod

from ..types import EdgeDescriptor, NodeDescriptor


class UpdaterContract(ABC):

    @abstractmethod
    def update_edge(
        self,
        from_id: str,
        to_id: str,
        observation: float,
        event_id: str,
    ) -> EdgeDescriptor:
        """
        Incorporate a new observation for the edge (from_id → to_id) and
        return the updated descriptor. No-op if event_id was already processed.
        """

    @abstractmethod
    def update_node(
        self,
        node_id: str,
        observation: float,
        event_id: str,
    ) -> NodeDescriptor:
        """
        Incorporate a new observation for node_id and return the updated
        descriptor. No-op if event_id was already processed.
        """

    @abstractmethod
    def reset(self, from_id: str, to_id: str) -> None:
        """
        Restore the edge to its prior state: ema_value = prior_weight,
        n_observations = 0, confidence = 0.0. Does not delete the edge.
        """
