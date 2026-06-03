// Scenario-style integration tests for the Semantic Map.
//
// These exercise the full pipeline — Ontology + Storage + Updater + Reasoner —
// across controlled, narrated timesteps. Compliance tests prove the parts
// satisfy their contracts in isolation; scenarios prove the parts compose
// into the behaviors the architecture promises.
//
// Run with:
//
//	go test -v -run TestScenario ./internal/minimal/tests/...
//
// Every scenario emits a compact narrative via t.Logf so the verbose output
// reads like a story. Hard assertions guard the mechanics that must not
// regress when we refactor.

package minimal_test

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/DiyazY/di-agent/internal/minimal"
	"github.com/DiyazY/di-agent/pkg/profiles"
	"github.com/DiyazY/di-agent/pkg/semmap"
	"github.com/DiyazY/di-agent/pkg/types"
)

// ── scenarioAgent: SemanticMap + handles for inspection ──────────────────────

type scenarioAgent struct {
	sm       *semmap.SemanticMap
	storage  *minimal.InMemoryStorage
	ontology *minimal.StaticDiSelectOntology
	updater  *minimal.EMAUpdater
}

// newScenarioAgent wires the same edge-minimal stack a production daemon
// would (InMemoryStorage + StaticDiSelectOntology + EMAUpdater +
// RuleEngineReasoner + DisabledProposer) and seeds storage from the
// ontology's default Di-Select bootstrap. It returns the SemanticMap plus
// raw handles so scenarios can call ontology mutations and inspect storage
// state directly — operations the facade intentionally hides from agent code.
func newScenarioAgent(t *testing.T) *scenarioAgent {
	t.Helper()
	storage := minimal.NewInMemoryStorage()
	ontology := minimal.NewStaticDiSelectOntology()
	updater := minimal.NewEMAUpdater(storage, 0.2, 500)
	reasoner := minimal.NewRuleEngineReasoner(storage, ontology, 0.5, nil, nil)
	proposer := minimal.NewDisabledProposer()

	seedReasonerState(t, storage, ontology)

	return &scenarioAgent{
		sm:       semmap.New(storage, ontology, updater, reasoner, proposer),
		storage:  storage,
		ontology: ontology,
		updater:  updater,
	}
}

// edgeSnapshot is a frozen view of one edge's state at a moment in time.
// Used to print "T=N: edge X is here" narrative rows.
type edgeSnapshot struct {
	FromID, ToID, PropID                                   string
	PriorWeight, EMAWeight, Effective, Confidence float64
	NObservations                                  int
}

func (s edgeSnapshot) String() string {
	return fmt.Sprintf("%s→%s[%s]  prior=%.3f  ema=%.3f  conf=%.3f  effective=%.3f  n=%d",
		s.FromID, s.ToID, s.PropID, s.PriorWeight, s.EMAWeight, s.Confidence, s.Effective, s.NObservations)
}

func snapEdgeByPropID(t *testing.T, s *minimal.InMemoryStorage, propID string) edgeSnapshot {
	t.Helper()
	edges, _ := s.AllEdges()
	for _, e := range edges {
		if e.PropositionID == propID {
			return edgeSnapshot{
				FromID:        e.FromID,
				ToID:          e.ToID,
				PropID:        e.PropositionID,
				PriorWeight:   e.PriorWeight,
				EMAWeight:     e.EMAWeight,
				Effective:     (1-e.Confidence)*e.PriorWeight + e.Confidence*e.EMAWeight,
				Confidence:    e.Confidence,
				NObservations: e.NObservations,
			}
		}
	}
	t.Fatalf("propID %q not in storage", propID)
	return edgeSnapshot{}
}

// ── Scenario 1: Cold start — every edge defers to its prior ──────────────────

func TestScenario_ColdStart(t *testing.T) {
	a := newScenarioAgent(t)

	t.Log("Scenario: cold start — ontology bootstrapped, no observations yet.")
	t.Log("Expected: confidence=0 on every edge, effective value equals the prior, EMA equals the prior.")

	edges, err := a.storage.AllEdges()
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 15 {
		t.Fatalf("expected 15 seeded edges (P1–P15); got %d", len(edges))
	}

	sort.Slice(edges, func(i, j int) bool { return edges[i].PropositionID < edges[j].PropositionID })
	for _, e := range edges {
		if e.Confidence != 0.0 {
			t.Errorf("edge %s starts with confidence=%.3f; expected 0.0", e.PropositionID, e.Confidence)
		}
		if e.EMAWeight != e.PriorWeight {
			t.Errorf("edge %s EMA=%.3f != prior=%.3f at cold start",
				e.PropositionID, e.EMAWeight, e.PriorWeight)
		}
		if e.NObservations != 0 {
			t.Errorf("edge %s NObservations=%d at cold start; expected 0", e.PropositionID, e.NObservations)
		}
	}

	cost, err := a.sm.CostOfAction("pod-scheduling", "node_1")
	if err != nil {
		t.Fatal(err)
	}
	if cost.Confidence != 0.0 {
		t.Errorf("aggregate confidence at cold start = %.3f; expected 0.0", cost.Confidence)
	}
	t.Logf("T=0 (cold start)")
	t.Logf("  edges seeded: %d", len(edges))
	t.Logf("  aggregate confidence: %.3f", cost.Confidence)
	t.Logf("  decision latency estimate: %.3f  energy: %.3f", cost.LatencyEstimate, cost.EnergyCost)
	t.Logf("  graph path length: %d (all edges contribute via priors)", len(cost.GraphPathUsed))
}

// ── Scenario 2: Convergence on a single edge ─────────────────────────────────

func TestScenario_ConvergenceOnOneEdge(t *testing.T) {
	a := newScenarioAgent(t)
	const target = "P10" // PS→RC negative — performance reducing resource overhead
	const observed = 0.85
	const totalObs = 500

	t.Logf("Scenario: stream %d constant observations at value %.2f on edge %s.", totalObs, observed, target)
	t.Log("Expected: confidence climbs 0 → 1, EMA drifts prior → observed, effective crosses over.")
	t.Log("")

	checkpoints := []int{0, 1, 5, 25, 100, 250, 500}
	cpIdx := 0

	for i := 0; i < totalObs; i++ {
		if cpIdx < len(checkpoints) && i == checkpoints[cpIdx] {
			snap := snapEdgeByPropID(t, a.storage, target)
			t.Logf("  T=%-4d  %s", i, snap)
			cpIdx++
		}
		// Find an edge with target propID to discover its (from, to).
		if i == 0 {
			snap := snapEdgeByPropID(t, a.storage, target)
			a.streamObservation(t, snap.FromID, snap.ToID, observed, i)
			continue
		}
		snap := snapEdgeByPropID(t, a.storage, target)
		a.streamObservation(t, snap.FromID, snap.ToID, observed, i)
	}

	final := snapEdgeByPropID(t, a.storage, target)
	t.Logf("  T=%-4d  %s", totalObs, final)

	// Invariants.
	if final.Confidence < 0.999 {
		t.Errorf("confidence should be ≈1.0 after %d obs (convergence=500); got %.3f", totalObs, final.Confidence)
	}
	if math.Abs(final.EMAWeight-observed) > 0.01 {
		t.Errorf("EMA should converge to %.3f after constant stream; got %.3f", observed, final.EMAWeight)
	}
	if math.Abs(final.Effective-observed) > 0.01 {
		t.Errorf("effective value should equal the EMA at confidence=1.0; got effective=%.3f ema=%.3f",
			final.Effective, final.EMAWeight)
	}
}

// streamObservation is a tiny convenience for the per-iteration ingest with a
// deterministic eventID. Errors fail the test fast.
func (a *scenarioAgent) streamObservation(t *testing.T, fromID, toID string, value float64, i int) {
	t.Helper()
	if err := a.sm.Ingest(fromID, toID, value, fmt.Sprintf("evt-%s→%s-%d", fromID, toID, i)); err != nil {
		t.Fatalf("ingest failed at i=%d: %v", i, err)
	}
}

// ── Scenario 3: Per-KD decisions differ ──────────────────────────────────────

func TestScenario_PerKDDecisionsDiffer(t *testing.T) {
	pwPath := findPriorWeightsFileForScenarios(t)

	smK3s, _, err := profiles.Build("edge-minimal", profiles.Config{
		EMAAlpha: 0.2, ConvergenceThreshold: 500, MinTrustScore: 0.5,
		PriorWeightsPath: pwPath, KD: "k3s",
	})
	if err != nil {
		t.Fatal(err)
	}
	smK0s, _, err := profiles.Build("edge-minimal", profiles.Config{
		EMAAlpha: 0.2, ConvergenceThreshold: 500, MinTrustScore: 0.5,
		PriorWeightsPath: pwPath, KD: "k0s",
	})
	if err != nil {
		t.Fatal(err)
	}

	costK3s, _ := smK3s.CostOfAction("pod-scheduling", "node_1")
	costK0s, _ := smK0s.CostOfAction("pod-scheduling", "node_1")

	t.Log("Scenario: two agents, same query, different -kd. Decisions should diverge because the per-KD")
	t.Log("priors land different initial weights on each edge.")
	t.Log("")
	t.Logf("  k3s  →  latency=%.3f  energy=%.3f  confidence=%.3f", costK3s.LatencyEstimate, costK3s.EnergyCost, costK3s.Confidence)
	t.Logf("  k0s  →  latency=%.3f  energy=%.3f  confidence=%.3f", costK0s.LatencyEstimate, costK0s.EnergyCost, costK0s.Confidence)

	if costK3s.LatencyEstimate == costK0s.LatencyEstimate &&
		costK3s.EnergyCost == costK0s.EnergyCost {
		t.Errorf("k3s and k0s agents produced identical cost estimates — per-KD seeding is not steering behavior")
	}
}

// ── Scenario 4: Deprecation shrinks the graph (but preserves the edge) ───────

func TestScenario_DeprecationShrinksGraph(t *testing.T) {
	a := newScenarioAgent(t)

	before, _ := a.sm.CostOfAction("pod-scheduling", "node_1")
	t.Log("Scenario: deprecate P1. Reasoner must skip it; storage must retain the EdgeDescriptor.")
	t.Log("")
	t.Logf("  before deprecation  graph path length = %d", len(before.GraphPathUsed))
	t.Logf("                       latency=%.3f  energy=%.3f  confidence=%.3f",
		before.LatencyEstimate, before.EnergyCost, before.Confidence)

	if err := a.ontology.Deprecate("P1", "scenario test"); err != nil {
		t.Fatal(err)
	}

	after, _ := a.sm.CostOfAction("pod-scheduling", "node_1")
	t.Logf("  after deprecation   graph path length = %d", len(after.GraphPathUsed))
	t.Logf("                       latency=%.3f  energy=%.3f  confidence=%.3f",
		after.LatencyEstimate, after.EnergyCost, after.Confidence)

	if len(after.GraphPathUsed) != len(before.GraphPathUsed)-1 {
		t.Errorf("graph path should shrink by exactly 1 after deprecating one proposition; got %d → %d",
			len(before.GraphPathUsed), len(after.GraphPathUsed))
	}

	// Storage must still hold the deprecated edge — soft delete only.
	all, _ := a.storage.AllEdges()
	stillPresent := false
	for _, e := range all {
		if e.PropositionID == "P1" {
			stillPresent = true
			break
		}
	}
	if !stillPresent {
		t.Error("deprecated edge P1 disappeared from storage — Deprecate must be soft-delete, not removal")
	}

	// Ontology surface still includes the deprecated proposition (flagged).
	props, _ := a.ontology.Propositions()
	foundDeprecated := false
	for _, p := range props {
		if p.PropositionID == "P1" {
			if !p.Deprecated {
				t.Error("P1 not flagged Deprecated in ontology after Deprecate()")
			}
			foundDeprecated = true
		}
	}
	if !foundDeprecated {
		t.Error("P1 vanished from Propositions() after Deprecate() — should remain visible with Deprecated=true")
	}
}

// ── Scenario 5: Idempotent replay ────────────────────────────────────────────

func TestScenario_IdempotentReplay(t *testing.T) {
	a := newScenarioAgent(t)

	t.Log("Scenario: stream 200 observations, capture state, replay them all with identical eventIDs.")
	t.Log("Expected: state byte-identical after replay. Then re-stream with NEW eventIDs — state changes.")
	t.Log("")

	const target = "P10" // PS→RC
	startSnap := snapEdgeByPropID(t, a.storage, target)

	// First pass.
	for i := 0; i < 200; i++ {
		a.streamObservation(t, startSnap.FromID, startSnap.ToID, 0.7, i)
	}
	afterFirst := snapEdgeByPropID(t, a.storage, target)
	t.Logf("  after first pass (200 obs):   %s", afterFirst)

	// Replay — same eventIDs.
	for i := 0; i < 200; i++ {
		a.streamObservation(t, startSnap.FromID, startSnap.ToID, 0.7, i)
	}
	afterReplay := snapEdgeByPropID(t, a.storage, target)
	t.Logf("  after replay (same evtIDs):   %s", afterReplay)

	if afterReplay.NObservations != afterFirst.NObservations ||
		afterReplay.EMAWeight != afterFirst.EMAWeight ||
		afterReplay.Confidence != afterFirst.Confidence {
		t.Error("replay with identical eventIDs changed state — idempotency violated")
	}

	// Now stream with new eventIDs starting from offset 1000 — state should advance.
	for i := 1000; i < 1200; i++ {
		a.streamObservation(t, startSnap.FromID, startSnap.ToID, 0.7, i)
	}
	afterNew := snapEdgeByPropID(t, a.storage, target)
	t.Logf("  after 200 new evtIDs:         %s", afterNew)

	if afterNew.NObservations == afterFirst.NObservations {
		t.Error("new eventIDs did not accumulate — idempotency is over-broad (suppressing legitimate updates)")
	}
}

// ── Scenario 6: Audit trail records every mutation ───────────────────────────

func TestScenario_AuditTrailRecordsEverything(t *testing.T) {
	a := newScenarioAgent(t)

	t.Log("Scenario: trigger one of each ontology mutation. GetHistory must contain exactly four events.")
	t.Log("")

	if err := a.ontology.SetPropositionStrength("P3", 0.77); err != nil {
		t.Fatal(err)
	}
	if err := a.ontology.AddConstruct(&types.Construct{
		ConstructID: "PR",
		Name:        "Privacy & Data Sovereignty",
		Description: "Sample new construct added by scenario test",
	}); err != nil {
		t.Fatal(err)
	}
	// Pick a fresh proposition pair that doesn't conflict with anything
	// existing — PS↔SC is not in the Di-Select bootstrap propositions.
	if err := a.ontology.AddValidatedProposition(&types.Proposition{
		PropositionID: "P-scenario",
		FromConstruct: "PS",
		ToConstruct:   "SC",
		Direction:     types.Positive,
		PriorStrength: 0.42,
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.ontology.Deprecate("P15", "scenario test deprecation"); err != nil {
		t.Fatal(err)
	}

	events, err := a.ontology.GetHistory(time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("  recorded %d events in audit log:", len(events))
	for i, e := range events {
		t.Logf("    [%d] %s  kind=%s  target=%s  actor=%s",
			i, e.Timestamp.Format(time.RFC3339Nano), e.Kind, e.TargetID, e.Actor)
	}

	if len(events) != 4 {
		t.Errorf("expected exactly 4 events after 4 mutations; got %d", len(events))
	}

	want := []types.OntologyEventKind{
		types.EventPropositionStrengthSet,
		types.EventConstructAdded,
		types.EventPropositionAdded,
		types.EventPropositionDeprecated,
	}
	for i, w := range want {
		if i >= len(events) {
			break
		}
		if events[i].Kind != w {
			t.Errorf("event[%d] kind=%s; expected %s", i, events[i].Kind, w)
		}
	}

	// Chronological insertion order: each timestamp must be ≥ the previous.
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp.Before(events[i-1].Timestamp) {
			t.Errorf("events not in chronological order: events[%d].Timestamp=%v < events[%d].Timestamp=%v",
				i, events[i].Timestamp, i-1, events[i-1].Timestamp)
		}
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// findPriorWeightsFileForScenarios walks up to locate prior_weights.json so
// scenarios are runnable from any working directory the test runner picks.
func findPriorWeightsFileForScenarios(t *testing.T) string {
	t.Helper()
	dir, _ := filepath.Abs(".")
	for i := 0; i < 6; i++ {
		candidates := []string{
			filepath.Join(dir, "prior_weights.json"),
			filepath.Join(dir, "semantic-map", "prior_weights.json"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Skip("prior_weights.json not found — per-KD scenario skipped")
	return ""
}
