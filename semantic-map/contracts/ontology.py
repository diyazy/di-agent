"""
Ontology contract — structural knowledge about constructs and their relationships.

Behavioral guarantees every implementation must uphold:
  - Bootstrap minimum: get_constructs and get_propositions always return at
    least the 7 Di-Select constructs and 15 propositions, regardless of any
    runtime extensions that have been added.
  - Immutability of validated knowledge: implementations may add new propositions
    via add_validated_proposition, but must never remove or reverse the direction
    of an existing validated proposition.
  - Pure queries: validate_proposition is read-only and never modifies state.
"""

from __future__ import annotations

from abc import ABC, abstractmethod

from ..types import Construct, Direction, Proposition, ValidationResult


class OntologyContract(ABC):

    @abstractmethod
    def get_constructs(self) -> list[Construct]:
        """Return all known constructs (minimum: the 7 Di-Select constructs)."""

    @abstractmethod
    def get_propositions(self) -> list[Proposition]:
        """Return all validated propositions (minimum: P1–P15 from Di-Select)."""

    @abstractmethod
    def get_relationships(self, construct_id: str) -> list[Proposition]:
        """Return all propositions where construct_id is the source or target."""

    @abstractmethod
    def validate_proposition(
        self, from_id: str, to_id: str, direction: Direction
    ) -> ValidationResult:
        """
        Check whether a proposed edge is structurally consistent with the
        existing backbone. Returns conflicts (contradicting propositions) and
        warnings (weaker inconsistencies). Never modifies state.
        """

    @abstractmethod
    def add_validated_proposition(self, proposition: Proposition) -> None:
        """
        Permanently add a new proposition to the backbone. Callers must call
        validate_proposition first and must not proceed if valid is False.
        Implementations must reject propositions that contradict existing ones.
        """
