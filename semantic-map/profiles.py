"""
Deployment profiles — wiring configurations for the five contracts.

A profile is a plain dataclass that names which implementation class to use
for each contract. The factory function build_map() resolves the names and
returns a fully wired SemanticMap instance.

Adding a new profile:
  1. Define a DeploymentProfile with the desired implementation class names.
  2. Register it in PROFILES.
  3. No other file needs to change.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class DeploymentProfile:
    name: str
    storage:  str   # fully qualified class name
    ontology: str
    updater:  str
    reasoner: str
    proposer: str


PROFILES: dict[str, DeploymentProfile] = {
    "edge-minimal": DeploymentProfile(
        name     = "edge-minimal",
        storage  = "semantic_map.implementations.storage.InMemoryStorage",
        ontology = "semantic_map.implementations.ontology.StaticDiSelectOntology",
        updater  = "semantic_map.implementations.updater.EMAUpdater",
        reasoner = "semantic_map.implementations.reasoner.RuleEngineReasoner",
        proposer = "semantic_map.implementations.proposer.DisabledProposer",
    ),
    "edge-standard": DeploymentProfile(
        name     = "edge-standard",
        storage  = "semantic_map.implementations.storage.SQLiteStorage",
        ontology = "semantic_map.implementations.ontology.StaticDiSelectOntology",
        updater  = "semantic_map.implementations.updater.EMAGaussianUpdater",
        reasoner = "semantic_map.implementations.reasoner.RuleEngineReasoner",
        proposer = "semantic_map.implementations.proposer.ThresholdProposer",
    ),
    "cloud-full": DeploymentProfile(
        name     = "cloud-full",
        storage  = "semantic_map.implementations.storage.Neo4jStorage",
        ontology = "semantic_map.implementations.ontology.RDFOntology",
        updater  = "semantic_map.implementations.updater.BayesianUpdater",
        reasoner = "semantic_map.implementations.reasoner.SLMReasoner",
        proposer = "semantic_map.implementations.proposer.CorrelationMinerProposer",
    ),
}


def build_map(profile_name: str, **kwargs):
    """
    Instantiate and wire a SemanticMap for the named profile.

    kwargs are forwarded to each implementation constructor, so implementations
    can accept profile-specific configuration (e.g. db_path for SQLiteStorage).

    Usage:
        sm = build_map("edge-minimal")
        sm = build_map("edge-standard", db_path="/data/semantic.db")
    """
    import importlib
    from .map import SemanticMap

    profile = PROFILES[profile_name]

    def load(dotted_path: str):
        module_path, class_name = dotted_path.rsplit(".", 1)
        module = importlib.import_module(module_path)
        cls = getattr(module, class_name)
        return cls(**kwargs)

    return SemanticMap(
        storage  = load(profile.storage),
        ontology = load(profile.ontology),
        updater  = load(profile.updater),
        reasoner = load(profile.reasoner),
        proposer = load(profile.proposer),
    )
