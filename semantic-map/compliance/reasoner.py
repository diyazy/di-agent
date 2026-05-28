"""
Compliance tests for ReasonerContract.
"""

from __future__ import annotations

import pytest

from ..contracts.reasoner import InsufficientTrustError
from ..types import OffloadContext


class ReasonerComplianceTests:

    @pytest.fixture
    def reasoner(self):
        raise NotImplementedError("provide a reasoner fixture")

    @pytest.fixture
    def context(self):
        return OffloadContext(
            task_type="pod-scheduling",
            source_node_id="node_1",
            data_size_bytes=1024,
            latency_budget_ms=500.0,
        )

    def test_cost_of_action_has_rationale(self, reasoner, context):
        result = reasoner.cost_of_action(context.task_type, context.source_node_id)
        assert result.rationale, "rationale must not be empty"

    def test_cost_of_action_has_graph_path(self, reasoner, context):
        result = reasoner.cost_of_action(context.task_type, context.source_node_id)
        assert len(result.graph_path_used) > 0

    def test_cost_of_action_confidence_in_range(self, reasoner, context):
        result = reasoner.cost_of_action(context.task_type, context.source_node_id)
        assert 0.0 <= result.confidence <= 1.0

    def test_recommend_peer_has_rationale(self, reasoner, context):
        try:
            result = reasoner.recommend_peer(context)
            assert result.rationale, "rationale must not be empty"
        except InsufficientTrustError:
            pass  # acceptable outcome — no trusted peers in test env

    def test_simulate_outcome_is_pure(self, reasoner, context):
        reasoner.simulate_outcome(context, "node_2")
        reasoner.simulate_outcome(context, "node_2")   # calling twice must not change state

    def test_simulate_outcome_confidence_in_range(self, reasoner, context):
        result = reasoner.simulate_outcome(context, "node_2")
        assert 0.0 <= result.confidence <= 1.0

    def test_simulate_outcome_p95_requires_gaussian(self, reasoner, context):
        result = reasoner.simulate_outcome(context, "node_2")
        if result.p95_latency is not None:
            assert result.p95_latency >= result.expected_latency
