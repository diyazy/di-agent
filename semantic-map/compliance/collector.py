"""
Compliance test suite for CollectorContract.

Mix CollectorComplianceTests into a pytest class and provide a `collector`
fixture that returns a concrete implementation under test.

Example:
    from semantic_map.compliance import CollectorComplianceTests
    import pytest

    class TestMyCgroupCollector(CollectorComplianceTests):
        @pytest.fixture
        def collector(self):
            return CgroupCollector(node_id="node_1", cgroup_root="/sys/fs/cgroup")

The fixture collector should be configured to point at a real or fake source
that can produce at least one sample. If the source is unavailable in CI,
return a stub that emits a fixed set of samples — the compliance suite itself
is source-agnostic.
"""

from __future__ import annotations

import pytest

from ..types import MetricSample, MetricType


class CollectorComplianceTests:
    """Shared compliance assertions for any CollectorContract implementation."""

    # subclasses must provide this fixture
    @pytest.fixture
    def collector(self):
        raise NotImplementedError("provide a collector fixture")

    # ── source_id ─────────────────────────────────────────────────────────────

    def test_source_id_non_empty(self, collector):
        assert collector.source_id(), "source_id() must return a non-empty string"

    def test_source_id_stable(self, collector):
        assert collector.source_id() == collector.source_id(), \
            "source_id() must return the same value on every call"

    # ── available_metrics ─────────────────────────────────────────────────────

    def test_available_metrics_non_empty(self, collector):
        metrics = collector.available_metrics()
        assert len(metrics) > 0, "available_metrics() must list at least one MetricType"

    def test_available_metrics_valid_types(self, collector):
        for mt in collector.available_metrics():
            assert isinstance(mt, MetricType), \
                f"available_metrics() must return MetricType members, got {type(mt)}"

    def test_available_metrics_stable(self, collector):
        first  = collector.available_metrics()
        second = collector.available_metrics()
        assert set(first) == set(second), \
            "available_metrics() must return the same set on every call"

    # ── collect ───────────────────────────────────────────────────────────────

    def test_collect_does_not_raise(self, collector):
        samples = collector.collect()
        assert samples is not None

    def test_collect_returns_list(self, collector):
        assert isinstance(collector.collect(), list)

    def test_collect_twice_no_raise(self, collector):
        collector.collect()
        collector.collect()

    def test_samples_have_non_empty_node_id(self, collector):
        for s in collector.collect():
            assert s.node_id, f"MetricSample.node_id must be non-empty; got {s!r}"

    def test_samples_have_non_empty_event_id(self, collector):
        for s in collector.collect():
            assert s.event_id, f"MetricSample.event_id must be non-empty; got {s!r}"

    def test_samples_metric_type_in_available(self, collector):
        available = set(collector.available_metrics())
        for s in collector.collect():
            assert s.metric_type in available, (
                f"sample metric_type {s.metric_type!r} not declared in "
                f"available_metrics(); declared: {available}"
            )

    def test_samples_are_metric_sample_instances(self, collector):
        for s in collector.collect():
            assert isinstance(s, MetricSample), \
                f"collect() must yield MetricSample instances, got {type(s)}"

    def test_event_id_deterministic(self, collector):
        """Same sample data must produce the same event_id across calls.

        This test collects twice and checks that event_ids from the first batch
        that also appear in the second batch are identical strings. Implementations
        using a monotonically advancing window will produce disjoint batches — in
        that case the test trivially passes (no overlapping event_ids to compare).
        """
        first  = {s.event_id: s for s in collector.collect()}
        second = {s.event_id: s for s in collector.collect()}
        overlap = set(first) & set(second)
        for eid in overlap:
            s1, s2 = first[eid], second[eid]
            assert s1.node_id      == s2.node_id
            assert s1.metric_type  == s2.metric_type
            assert s1.value        == s2.value
            assert s1.timestamp_unix == s2.timestamp_unix
