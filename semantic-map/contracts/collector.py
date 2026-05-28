"""
Collector contract — read raw metrics from a source and emit normalized samples.

The collector sits between a metric source (cgroup filesystem, Netdata HTTP API,
kubelet /metrics, etc.) and the Semantic Map's Updater contract. It is responsible
for reading, normalizing, and tagging observations. It knows nothing about the
graph topology — that mapping is the bridge's responsibility.

Behavioral guarantees every implementation must uphold:
  - Pure read: collect() never modifies any system state. It may fail with an
    error if the source is unavailable, but it must not raise on empty data —
    an empty list is the correct return when no new samples are ready.
  - Deterministic event_id: for the same physical observation (same source_id,
    node_id, container_id, metric_type, and timestamp_unix) the event_id must
    be identical across calls and across restarts. This carries the Updater's
    idempotency guarantee end-to-end.
  - Metric type stability: available_metrics() returns the same set for the
    entire lifetime of the collector instance. Implementations must not emit
    samples with metric_type outside this set.
  - Node ID completeness: every emitted MetricSample has a non-empty node_id
    that identifies the cluster node (e.g. "master", "node_1").
  - source_id stability: source_id() returns the same string across restarts
    for the same physical collector on the same node.
"""

from __future__ import annotations

from abc import ABC, abstractmethod

from ..types import MetricSample, MetricType


class CollectorContract(ABC):

    @abstractmethod
    def collect(self) -> list[MetricSample]:
        """Read one batch of current metric samples from the source.

        Returns an empty list when no new data is available.
        Never raises; returns an error-free empty list on transient source
        unavailability — implementations should log internally.
        """

    @abstractmethod
    def source_id(self) -> str:
        """Stable identifier for this collector instance.

        Used as a component in event_id generation so that samples from
        different collector types on the same node remain distinguishable.
        Must be non-empty and stable across restarts.
        """

    @abstractmethod
    def available_metrics(self) -> list[MetricType]:
        """Metric types this implementation can produce.

        Static — the returned set must not change within a deployment session.
        The bridge uses this to determine which graph edges can be updated by
        this collector without calling collect() first.
        """
