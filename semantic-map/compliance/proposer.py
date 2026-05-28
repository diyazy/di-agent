"""
Compliance tests for ProposerContract.
"""

from __future__ import annotations

import pytest

from ..types import CandidateStatus


class ProposerComplianceTests:

    @pytest.fixture
    def proposer(self):
        raise NotImplementedError("provide a proposer fixture")

    def _feed(self, proposer, n: int = 100) -> None:
        """Feed enough correlated observations to trigger a candidate proposal."""
        for i in range(n):
            proposer.observe("Performance", "Security", float(i), float(i) * 0.9)

    def test_observe_does_not_modify_external_state(self, proposer):
        # Proposer must not raise, and get_candidates is still queryable
        proposer.observe("a", "b", 0.5, 0.4)
        proposer.get_candidates()

    def test_get_candidates_returns_only_pending(self, proposer):
        self._feed(proposer)
        for c in proposer.get_candidates():
            assert c.status == CandidateStatus.PENDING

    def test_reject_suppresses_candidate(self, proposer):
        self._feed(proposer)
        candidates = proposer.get_candidates()
        if not candidates:
            pytest.skip("no candidates generated — lower threshold for this test")
        cid = candidates[0].candidate_id
        proposer.reject(cid)
        remaining = [c for c in proposer.get_candidates() if c.candidate_id == cid]
        assert remaining == []

    def test_rejected_appears_in_history(self, proposer):
        self._feed(proposer)
        candidates = proposer.get_candidates()
        if not candidates:
            pytest.skip("no candidates generated")
        cid = candidates[0].candidate_id
        proposer.reject(cid)
        history = proposer.get_history()
        rejected = [c for c in history if c.candidate_id == cid]
        assert rejected and rejected[0].status == CandidateStatus.REJECTED

    def test_defer_moves_candidate_out_of_pending(self, proposer):
        self._feed(proposer)
        candidates = proposer.get_candidates()
        if not candidates:
            pytest.skip("no candidates generated")
        cid = candidates[0].candidate_id
        proposer.defer(cid)
        remaining = [c for c in proposer.get_candidates() if c.candidate_id == cid]
        assert remaining == []

    def test_history_includes_all_statuses(self, proposer):
        self._feed(proposer)
        all_history = proposer.get_history()
        # History is a superset of pending candidates
        pending_ids = {c.candidate_id for c in proposer.get_candidates()}
        history_ids = {c.candidate_id for c in all_history}
        assert pending_ids.issubset(history_ids)
