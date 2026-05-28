"""
Compliance tests for OntologyContract.
"""

from __future__ import annotations

import pytest

from ..types import Direction, Proposition


class OntologyComplianceTests:

    @pytest.fixture
    def ontology(self):
        raise NotImplementedError("provide an ontology fixture")

    def test_has_seven_constructs(self, ontology):
        assert len(ontology.get_constructs()) >= 7

    def test_has_fifteen_propositions(self, ontology):
        assert len(ontology.get_propositions()) >= 15

    def test_all_proposition_ids_are_unique(self, ontology):
        ids = [p.proposition_id for p in ontology.get_propositions()]
        assert len(ids) == len(set(ids))

    def test_get_relationships_returns_subset(self, ontology):
        constructs = ontology.get_constructs()
        cid = constructs[0].construct_id
        rels = ontology.get_relationships(cid)
        all_props = ontology.get_propositions()
        for r in rels:
            assert r in all_props

    def test_get_relationships_unknown_construct_returns_empty(self, ontology):
        assert ontology.get_relationships("nonexistent") == []

    def test_validate_proposition_is_pure(self, ontology):
        before = len(ontology.get_propositions())
        ontology.validate_proposition("Performance", "Security", Direction.POSITIVE)
        after = len(ontology.get_propositions())
        assert before == after

    def test_add_validated_proposition_persists(self, ontology):
        result = ontology.validate_proposition("Performance", "Reliability", Direction.POSITIVE)
        if not result.valid:
            pytest.skip("proposition conflicts with backbone — skip add test")
        p = Proposition("P-test", "Performance", "Reliability", Direction.POSITIVE, 0.5, ["test"])
        ontology.add_validated_proposition(p)
        assert any(x.proposition_id == "P-test" for x in ontology.get_propositions())

    def test_add_contradicting_proposition_raises(self, ontology):
        props = ontology.get_propositions()
        existing = props[0]
        contradicting = Proposition(
            "P-bad",
            existing.from_construct,
            existing.to_construct,
            Direction.POSITIVE if existing.direction == Direction.NEGATIVE else Direction.NEGATIVE,
            0.5,
        )
        with pytest.raises(Exception):
            ontology.add_validated_proposition(contradicting)
