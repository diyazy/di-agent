package compare

import (
	"math"
	"testing"

	"github.com/DiyazY/di-agent/pkg/types"
)

// helper: build a PerKDResult with two edges (P1, P2) of given effective
// values. Effective is computed by Compute() from prior/EMA/confidence, so
// for unit tests we pick a configuration where the formula produces clean
// numbers: confidence=1.0 → effective == ema. That isolates Compute()'s
// stats math from the seeding details.
func mkResult(kd string, p1EMA, p2EMA float64) *PerKDResult {
	return &PerKDResult{
		KD: kd,
		Edges: []*types.EdgeDescriptor{
			{
				PropositionID: "P1",
				FromID:        "SC",
				ToID:          "RC",
				Direction:     types.Positive,
				PriorWeight:   0.5,
				EMAWeight:     p1EMA,
				Confidence:    1.0,
				NObservations: 100,
			},
			{
				PropositionID: "P2",
				FromID:        "RC",
				ToID:          "PS",
				Direction:     types.Negative,
				PriorWeight:   0.4,
				EMAWeight:     p2EMA,
				Confidence:    1.0,
				NObservations: 100,
			},
		},
	}
}

func TestCompute_RangeAndStdDev(t *testing.T) {
	perKD := []*PerKDResult{
		mkResult("a", 0.10, 0.20),
		mkResult("b", 0.20, 0.30),
		mkResult("c", 0.30, 0.40),
	}
	div := Compute(perKD)
	if len(div) != 2 {
		t.Fatalf("Compute: got %d entries, want 2", len(div))
	}

	// Confidence=1.0 → effective == ema. So:
	//   P1: 0.10, 0.20, 0.30 → range = 0.20, stddev = sqrt(((-0.1)^2 + 0 + (0.1)^2)/2) = sqrt(0.01) = 0.1
	//   P2: same shifted by 0.1 → same range and stddev
	for _, d := range div {
		if !approxEq(d.Range, 0.20, 1e-9) {
			t.Errorf("%s Range: got %.6f, want 0.20", d.PropositionID, d.Range)
		}
		if !approxEq(d.StdDev, 0.10, 1e-9) {
			t.Errorf("%s StdDev: got %.6f, want 0.10", d.PropositionID, d.StdDev)
		}
		if len(d.Effective) != 3 {
			t.Errorf("%s Effective slice len: got %d, want 3", d.PropositionID, len(d.Effective))
		}
	}
}

func TestCompute_SortsByRangeDescending(t *testing.T) {
	// P1 range = 0.10, P2 range = 0.30. Compute should return [P2, P1].
	perKD := []*PerKDResult{
		mkResult("a", 0.10, 0.20),
		mkResult("b", 0.20, 0.50),
	}
	div := Compute(perKD)
	if len(div) != 2 {
		t.Fatalf("len: %d", len(div))
	}
	if div[0].PropositionID != "P2" {
		t.Errorf("most-divergent should be first: got %s, want P2", div[0].PropositionID)
	}
	if div[1].PropositionID != "P1" {
		t.Errorf("least-divergent should be last: got %s, want P1", div[1].PropositionID)
	}
	if div[0].Range <= div[1].Range {
		t.Errorf("Range desc broken: %.3f then %.3f", div[0].Range, div[1].Range)
	}
}

func TestCompute_EffectiveFormula(t *testing.T) {
	// Effective = (1 - confidence) * prior + confidence * ema.
	// Half-blended example: confidence=0.5, prior=0.4, ema=0.8 → effective=0.6.
	perKD := []*PerKDResult{
		{
			KD: "a",
			Edges: []*types.EdgeDescriptor{
				{
					PropositionID: "P1",
					FromID:        "SC", ToID: "RC",
					Direction:     types.Positive,
					PriorWeight:   0.4,
					EMAWeight:     0.8,
					Confidence:    0.5,
					NObservations: 1,
				},
			},
		},
	}
	div := Compute(perKD)
	if len(div) != 1 {
		t.Fatalf("len: %d", len(div))
	}
	if !approxEq(div[0].Effective[0], 0.6, 1e-9) {
		t.Errorf("Effective: got %.6f, want 0.6 (= 0.5*0.4 + 0.5*0.8)", div[0].Effective[0])
	}
}

func TestCompute_DirectionMapping(t *testing.T) {
	perKD := []*PerKDResult{
		{
			KD: "a",
			Edges: []*types.EdgeDescriptor{
				{PropositionID: "P1", FromID: "SC", ToID: "RC", Direction: types.Positive, PriorWeight: 0.5},
				{PropositionID: "P2", FromID: "RC", ToID: "PS", Direction: types.Negative, PriorWeight: 0.4},
			},
		},
	}
	div := Compute(perKD)
	dirs := map[string]string{}
	for _, d := range div {
		dirs[d.PropositionID] = d.Direction
	}
	if dirs["P1"] != "+" {
		t.Errorf("P1 direction: got %q, want +", dirs["P1"])
	}
	if dirs["P2"] != "-" {
		t.Errorf("P2 direction: got %q, want -", dirs["P2"])
	}
}

func TestCompute_SingleKDHasZeroRange(t *testing.T) {
	// One KD → range and stddev must both be 0 (no spread to measure).
	perKD := []*PerKDResult{mkResult("a", 0.1, 0.2)}
	div := Compute(perKD)
	for _, d := range div {
		if d.Range != 0 {
			t.Errorf("%s Range with 1 KD: got %.6f, want 0", d.PropositionID, d.Range)
		}
		if d.StdDev != 0 {
			t.Errorf("%s StdDev with 1 KD: got %.6f, want 0", d.PropositionID, d.StdDev)
		}
	}
}

func TestCompute_EmptyInput(t *testing.T) {
	if got := Compute(nil); got != nil {
		t.Errorf("Compute(nil): got %v, want nil", got)
	}
	if got := Compute([]*PerKDResult{}); got != nil {
		t.Errorf("Compute(empty): got %v, want nil", got)
	}
}

func TestTopN(t *testing.T) {
	div := []*EdgeDivergence{
		{PropositionID: "P3", Range: 0.30},
		{PropositionID: "P1", Range: 0.20},
		{PropositionID: "P2", Range: 0.10},
	}
	top := TopN(div, 2)
	if len(top) != 2 {
		t.Errorf("TopN(2) len: %d", len(top))
	}
	if top[0].PropositionID != "P3" {
		t.Errorf("TopN[0]: %s, want P3", top[0].PropositionID)
	}
	if got := TopN(div, 0); len(got) != len(div) {
		t.Errorf("TopN(0): got len=%d, want len=%d (full slice)", len(got), len(div))
	}
	if got := TopN(div, 100); len(got) != len(div) {
		t.Errorf("TopN(huge): got len=%d, want len=%d (full slice)", len(got), len(div))
	}
}

func approxEq(a, b, eps float64) bool {
	return math.Abs(a-b) <= eps
}
