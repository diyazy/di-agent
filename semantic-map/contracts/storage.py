"""
Storage contract — read/write of node and edge descriptors.

Behavioral guarantees every implementation must uphold:
  - Atomicity: put_node/put_edge either fully succeed or leave the previous
    state intact. Partial writes are never visible to concurrent readers.
  - Null safety: get_node and get_edge return None for unknown ids; they
    never raise KeyError or similar.
  - Empty-safe: get_neighbors and all_edges return empty collections for
    unknown ids; they never raise.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import Optional

from ..types import EdgeDescriptor, NodeDescriptor


class StorageContract(ABC):

    @abstractmethod
    def get_node(self, node_id: str) -> Optional[NodeDescriptor]:
        """Return the descriptor for node_id, or None if it does not exist."""

    @abstractmethod
    def put_node(self, descriptor: NodeDescriptor) -> None:
        """Atomically write (or overwrite) the descriptor for descriptor.node_id."""

    @abstractmethod
    def get_edge(self, from_id: str, to_id: str) -> Optional[EdgeDescriptor]:
        """Return the descriptor for the directed edge (from_id → to_id), or None."""

    @abstractmethod
    def put_edge(self, descriptor: EdgeDescriptor) -> None:
        """Atomically write (or overwrite) the descriptor for the directed edge."""

    @abstractmethod
    def get_neighbors(self, node_id: str) -> list[str]:
        """Return ids of all nodes reachable by a single outgoing edge from node_id."""

    @abstractmethod
    def all_edges(self) -> list[EdgeDescriptor]:
        """Return every edge currently in storage."""
