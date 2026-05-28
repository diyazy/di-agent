# Semantic Map

An adaptive behavioral data structure for autonomous edge orchestration agents.

The Semantic Map is the "brain" of an edge agent: it starts with scientifically grounded priors derived from the [Di-Select framework](../di-select/) and five empirical publications, then continuously refines those priors with real deployment telemetry. As observations accumulate, the agent's decisions shift from generic literature knowledge toward deployment-specific behavioral patterns — without any labeling or manual tuning.

For design rationale, contract decisions, language strategy, and research connection see **[ARCHITECTURE.md](ARCHITECTURE.md)**.

> **Research context:** Core artifact for Publication P6 — *Semantic Map & Context-Aware Agent Architecture* — part of the dissertation *A Context-Aware Agentic Framework for Decentralized Edge Computing* (Tampere University, Diyaz Yakubov).

---

## Table of Contents

- [1. Project Structure](#1-project-structure)
- [2. The Agent API](#2-the-agent-api)
- [3. Running the Edge Daemon](#3-running-the-edge-daemon)
- [4. Compliance Tests](#4-compliance-tests)
- [5. Prior Initialization](#5-prior-initialization)

---

## 1. Project Structure

```
semantic-map/
│
│  ── Python layer (specification + cloud-full) ──────────────────
│
├── types.py                    Shared data types (MetricSample, MetricType, EdgeDescriptor, …)
├── map.py                      SemanticMap Python facade
├── profiles.py                 Deployment profile registry + build_map() factory
├── prior_weights.json          Calibrated prior strengths output by the init pipeline
├── requirements.txt            pytest only (no runtime deps for contracts)
│
├── contracts/                  Contract definitions (Python ABCs)
│   ├── collector.py            CollectorContract
│   ├── storage.py              StorageContract
│   ├── ontology.py             OntologyContract
│   ├── updater.py              UpdaterContract
│   ├── reasoner.py             ReasonerContract + InsufficientTrustError
│   └── proposer.py             ProposerContract
│
├── compliance/                 Shared compliance test suites (mix into pytest class)
│   ├── collector.py            CollectorComplianceTests
│   ├── storage.py              StorageComplianceTests
│   ├── ontology.py             OntologyComplianceTests
│   ├── updater.py              UpdaterComplianceTests
│   ├── reasoner.py             ReasonerComplianceTests
│   └── proposer.py             ProposerComplianceTests
│
├── prior_init/                 Prior initialization pipeline (Step 4)
│   ├── pipeline.py             Entry point — reads P1–P5 constants, writes prior_weights.json
│   ├── calibration.py          Construct scoring + proposition strength computation
│   ├── constants.py            Publication constants (J/pod, mJ/op, CIS scores, …)
│   └── loaders.py              CSV / result file loaders
│
│  ── Go layer (edge daemon) ──────────────────────────────────────
│
└── go/
    ├── go.mod                  Module: github.com/DiyazY/di-agent
    │
    ├── pkg/                    Public packages — importable by agent code
    │   ├── types/types.go      Go equivalents of all Python types
    │   ├── contracts/          Go interfaces (mirrors of Python ABCs)
    │   │   └── contracts.go    All 6 interfaces + sentinel errors
    │   ├── semmap/map.go       SemanticMap Go facade
    │   └── profiles/           Profile factory
    │       └── profiles.go     Build("edge-minimal", cfg) + ontology seeding
    │
    ├── internal/               Implementation packages — not importable externally
    │   └── minimal/            edge-minimal profile implementations
    │       ├── collector_cgroup.go  CgroupCollector  (cgroups v2, no daemon)
    │       ├── storage.go      InMemoryStorage        (sync.RWMutex maps)
    │       ├── ontology.go     StaticDiSelectOntology (7 constructs + P1–P15)
    │       ├── updater.go      EMAUpdater             (idempotency, reset)
    │       ├── reasoner.go     RuleEngineReasoner     (deterministic, blended)
    │       ├── proposer.go     DisabledProposer       (no-op)
    │       └── tests/
    │           └── compliance_test.go   Runs all Go compliance suites
    │
    ├── compliance/             Go compliance test suites
    │   ├── collector.go        RunCollectorCompliance(t, factory)
    │   ├── storage.go          RunStorageCompliance(t, factory)
    │   └── updater.go          RunUpdaterCompliance(t, factory)
    │
    └── cmd/agent/              Daemon entry point
        └── main.go             HTTP server: /ingest /cost /recommend /simulate /candidates
```

---

## 2. The Agent API

Three stable queries across all profiles:

### `GET /cost?task=<type>&node=<id>`

Estimates the cost of executing a task on a given node.

```json
{
  "cpu_cost": 0.12,
  "energy_cost": 0.034,
  "latency_estimate": 7.4,
  "confidence": 0.62,
  "rationale": "task=pod-scheduling node=node_1 path=[SC→RC(0.58), RC→PS(0.41)]",
  "graph_path_used": ["SC→RC(0.58)", "RC→PS(0.41)"]
}
```

### `POST /recommend`

Finds the best peer for task offloading. Returns `InsufficientTrustError` if no peer meets the minimum trust threshold.

```json
// request
{"task_type":"pod-scheduling","source_node_id":"node_1","data_size_bytes":2048,"latency_budget_ms":500}

// response
{"peer_id":"node_2","expected_savings":0.018,"rationale":"...","graph_path_used":["..."]}
```

### `POST /simulate`

Pre-flight simulation before committing an offload. Read-only — never modifies state.

```json
// request
{"context":{...},"target_node_id":"node_2"}

// response
{
  "expected_latency": 8.1,
  "expected_energy": 0.029,
  "confidence": 0.55,
  "p95_latency": 12.4,
  "p95_energy": null,
  "risk_flags": [],
  "graph_path_used": ["..."]
}
```

`p95_*` is `null` on `edge-minimal` — requires Gaussian descriptors (`edge-standard` upward).

### `POST /ingest`

Feed a telemetry observation directly into the Updater.

```json
{"from_id":"SC","to_id":"RC","observation":0.71,"event_id":"cgroup-1703208286-001"}
```

`event_id` is required. The Updater is idempotent: replaying the same `event_id` is a no-op. The Collector produces these automatically; use this endpoint for manual injection or testing.

### `GET /candidates`

Lists Proposer candidate edges pending review.

```json
[{"candidate_id":"cand-001","from_id":"CO","to_id":"PS","direction":1,
  "mi_score":0.73,"p_value":0.002,"n_observations":1240,"deployments_seen":2,"status":0}]
```

Review via `POST /candidates/{id}/confirm`, `/reject`, or `/defer` — Year 2 scope.

---

## 3. Running the Edge Daemon

**Prerequisites:** Go 1.22+, `linux/arm64` for RPi4.

```bash
# Build for RPi4
cd semantic-map/go
GOOS=linux GOARCH=arm64 go build -o agent-arm64 ./cmd/agent

# Deploy
scp agent-arm64 pi@192.168.1.x:/usr/local/bin/agent

# Run on RPi4
agent -profile edge-minimal -addr :8080 -alpha 0.2 -convergence 500

# Run locally (development)
go run ./cmd/agent -profile edge-minimal
```

| Flag           | Default        | Description                             |
| -------------- | -------------- | --------------------------------------- |
| `-profile`     | `edge-minimal` | Deployment profile name                 |
| `-addr`        | `:8080`        | HTTP listen address                     |
| `-alpha`       | `0.2`          | EMA decay factor                        |
| `-convergence` | `500`          | Observations until confidence = 1.0     |
| `-min-trust`   | `0.5`          | Minimum peer trust score for offloading |

---

## 4. Compliance Tests

Every contract has a compliance suite. A new implementation is valid if and only if it passes the suite for its contract.

### Python

Mix the compliance class into a pytest class and provide the named fixture:

```python
from semantic_map.compliance import StorageComplianceTests, CollectorComplianceTests
import pytest

class TestMyStorage(StorageComplianceTests):
    @pytest.fixture
    def storage(self):
        return MyStorage(":memory:")

class TestMyCgroupCollector(CollectorComplianceTests):
    @pytest.fixture
    def collector(self):
        return CgroupCollector(node_id="node_1", cgroup_root="/sys/fs/cgroup")
```

```bash
pip install -r requirements.txt
pytest compliance/
```

### Go

```go
func TestStorageCompliance(t *testing.T) {
    compliance.RunStorageCompliance(t, func(t *testing.T) contracts.StorageContract {
        return NewMyStorage()
    })
}

func TestCgroupCollectorCompliance(t *testing.T) {
    compliance.RunCollectorCompliance(t, func(t *testing.T) contracts.CollectorContract {
        return NewCgroupCollector("node_1", "/sys/fs/cgroup")
    })
}
```

```bash
cd go && go test ./...
```

---

## 5. Prior Initialization

Run once before deploying to a new cluster to replace bootstrap edge weights with values grounded in P1–P5 empirical data:

```bash
python -m semantic_map.prior_init.pipeline \
  --root ../ \
  --out prior_weights.json
```

The pipeline reads publication constants (J/pod, mJ/op, CIS scores, overhead fractions) and writes calibrated `prior_strength` values for all 15 propositions across all 5 KDs. The agent daemon loads `prior_weights.json` at startup via `-priors` flag (to be implemented).

The current `prior_weights.json` was generated on 2026-05-12. Re-run if new empirical papers are incorporated or the KD set changes.
