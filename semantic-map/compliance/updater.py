"""
Compliance tests for UpdaterContract.
"""

from __future__ import annotations

import pytest

from ..types import Direction, EdgeDescriptor


class UpdaterComplianceTests:

    @pytest.fixture
    def updater(self):
        raise NotImplementedError("provide an updater fixture")

    @pytest.fixture
    def seeded_storage(self, storage):
        """Storage pre-populated with one edge for updater tests."""
        storage.put_edge(
            EdgeDescriptor("a", "b", "P1", Direction.POSITIVE, 0.5, 0.5, 0.0, 0)
        )
        return storage

    def test_update_increments_observation_count(self, updater, seeded_storage):
        result = updater.update_edge("a", "b", 0.6, event_id="evt-1")
        assert result.n_observations == 1

    def test_update_shifts_ema_toward_observation(self, updater, seeded_storage):
        result = updater.update_edge("a", "b", 1.0, event_id="evt-1")
        assert result.ema_weight > 0.5

    def test_update_is_idempotent_on_same_event_id(self, updater, seeded_storage):
        first  = updater.update_edge("a", "b", 0.9, event_id="evt-1")
        second = updater.update_edge("a", "b", 0.9, event_id="evt-1")
        assert first == second

    def test_different_event_ids_accumulate(self, updater, seeded_storage):
        updater.update_edge("a", "b", 0.6, event_id="evt-1")
        result = updater.update_edge("a", "b", 0.7, event_id="evt-2")
        assert result.n_observations == 2

    def test_reset_restores_prior(self, updater, seeded_storage):
        updater.update_edge("a", "b", 0.9, event_id="evt-1")
        updater.reset("a", "b")
        edge = seeded_storage.get_edge("a", "b")
        assert edge.ema_weight == edge.prior_weight
        assert edge.n_observations == 0
        assert edge.confidence == pytest.approx(0.0)

    def test_reset_does_not_delete_edge(self, updater, seeded_storage):
        updater.reset("a", "b")
        assert seeded_storage.get_edge("a", "b") is not None

    def test_confidence_increases_with_observations(self, updater, seeded_storage):
        c0 = seeded_storage.get_edge("a", "b").confidence
        updater.update_edge("a", "b", 0.6, event_id="evt-1")
        c1 = seeded_storage.get_edge("a", "b").confidence
        assert c1 > c0
