"""
Proposer contract — detect statistical patterns that suggest new backbone edges.

Behavioral guarantees every implementation must uphold:
  - Read-only observation: observe never modifies Storage or Ontology. It only
    accumulates internal statistics. The Proposer is strictly
    read-from-evidence, write-to-candidates.
  - Confirm delegates to Ontology: confirm calls OntologyContract.add_validated_
    proposition on the underlying ontology and does not write to Storage directly.
    The Ontology contract is responsible for structural validation before the
    edge is accepted.
  - Permanent suppression: after reject(candidate_id), the same
    (from_id, to_id, direction) triple will not be re-proposed within the same
    deployment session. Suppression is lifted only on explicit session reset.
  - get_candidates returns only PENDING candidates. CONFIRMED, REJECTED, and
    DEFERRED candidates are accessible only via get_history.
"""

from __future__ import annotations

from abc import ABC, abstractmethod

from ..types import CandidateEdge


class ProposerContract(ABC):

    @abstractmethod
    def observe(
        self, from_id: str, to_id: str, value_a: float, value_b: float
    ) -> None:
        """
        Feed a co-occurring pair of metric values for (from_id, to_id).
        Accumulates statistics internally; never modifies Storage or Ontology.
        """

    @abstractmethod
    def get_candidates(self) -> list[CandidateEdge]:
        """Return all PENDING candidate edges that meet the proposal threshold."""

    @abstractmethod
    def confirm(self, candidate_id: str) -> None:
        """
        Mark the candidate as CONFIRMED and add it to the Ontology via
        add_validated_proposition. The Ontology contract performs the
        structural validation; this method raises if validation fails.
        """

    @abstractmethod
    def reject(self, candidate_id: str) -> None:
        """
        Mark the candidate as REJECTED. The (from_id, to_id, direction)
        triple is suppressed for the rest of this deployment session.
        """

    @abstractmethod
    def defer(self, candidate_id: str) -> None:
        """Mark the candidate as DEFERRED; re-evaluate after more observations."""

    @abstractmethod
    def get_history(self) -> list[CandidateEdge]:
        """Return all candidates regardless of status, including CONFIRMED/REJECTED."""
