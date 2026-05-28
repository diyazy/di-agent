"""
Compliance tests for StorageContract.

Mix into a pytest class and provide a `storage` fixture returning your
implementation. Every test here must pass for an implementation to be valid.

Usage:
    from semantic_map.compliance import StorageComplianceTests

    class TestMyStorage(StorageComplianceTests):
        @pytest.fixture
        def storage(self):
            return MyStorage(":memory:")
"""

from __future__ import annotations

import pytest

from ..types import Direction, EdgeDescriptor, NodeDescriptor


class StorageComplianceTests:

    @pytest.fixture
    def storage(self):
        raise NotImplementedError("provide a storage fixture")

    # ── NodeDescriptor ────────────────────────────────────────────────────────

    def test_get_missing_node_returns_none(self, storage):
        assert storage.get_node("does-not-exist") is None

    def test_put_get_node_roundtrip(self, storage):
        desc = NodeDescriptor("n1", "Performance", 0.5, 0.5, 0.0, 0)
        storage.put_node(desc)
        assert storage.get_node("n1") == desc

    def test_put_node_overwrites(self, storage):
        storage.put_node(NodeDescriptor("n1", "Performance", 0.5, 0.5, 0.0, 0))
        updated = NodeDescriptor("n1", "Security", 0.8, 0.8, 0.1, 5)
        storage.put_node(updated)
        assert storage.get_node("n1") == updated

    # ── EdgeDescriptor ────────────────────────────────────────────────────────

    def test_get_missing_edge_returns_none(self, storage):
        assert storage.get_edge("a", "b") is None

    def test_put_get_edge_roundtrip(self, storage):
        desc = EdgeDescriptor("a", "b", "P1", Direction.POSITIVE, 0.6, 0.6, 0.0, 0)
        storage.put_edge(desc)
        assert storage.get_edge("a", "b") == desc

    def test_edge_direction_is_preserved(self, storage):
        desc = EdgeDescriptor("a", "b", "P1", Direction.NEGATIVE, 0.4, 0.4, 0.0, 0)
        storage.put_edge(desc)
        assert storage.get_edge("a", "b").direction == Direction.NEGATIVE

    def test_edge_is_directed(self, storage):
        storage.put_edge(EdgeDescriptor("a", "b", "P1", Direction.POSITIVE, 0.5, 0.5, 0.0, 0))
        assert storage.get_edge("b", "a") is None

    # ── Neighbors / all_edges ─────────────────────────────────────────────────

    def test_get_neighbors_unknown_node_returns_empty(self, storage):
        assert storage.get_neighbors("ghost") == []

    def test_get_neighbors_reflects_edges(self, storage):
        storage.put_edge(EdgeDescriptor("x", "y", "P2", Direction.POSITIVE, 0.5, 0.5, 0.0, 0))
        storage.put_edge(EdgeDescriptor("x", "z", "P3", Direction.NEGATIVE, 0.3, 0.3, 0.0, 0))
        neighbors = storage.get_neighbors("x")
        assert set(neighbors) == {"y", "z"}

    def test_all_edges_returns_every_edge(self, storage):
        e1 = EdgeDescriptor("a", "b", "P1", Direction.POSITIVE, 0.5, 0.5, 0.0, 0)
        e2 = EdgeDescriptor("b", "c", "P2", Direction.NEGATIVE, 0.3, 0.3, 0.0, 0)
        storage.put_edge(e1)
        storage.put_edge(e2)
        edges = storage.all_edges()
        assert len(edges) >= 2
