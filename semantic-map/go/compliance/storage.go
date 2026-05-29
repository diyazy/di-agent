// Package compliance provides shared contract compliance test suites.
// Call RunXxxCompliance(t, factory) from any implementation's _test.go file.
package compliance

import (
	"testing"

	"github.com/DiyazY/di-agent/pkg/contracts"
	"github.com/DiyazY/di-agent/pkg/types"
)

type StorageFactory func(t *testing.T) contracts.StorageContract

func RunStorageCompliance(t *testing.T, factory StorageFactory) {
	t.Helper()

	t.Run("GetMissingNodeReturnsNil", func(t *testing.T) {
		s := factory(t)
		node, err := s.GetNode("nonexistent")
		if err != nil {
			t.Fatal(err)
		}
		if node != nil {
			t.Error("expected nil for unknown node")
		}
	})

	t.Run("PutGetNodeRoundtrip", func(t *testing.T) {
		s := factory(t)
		d := &types.NodeDescriptor{NodeID: "n1", ConstructType: "PS", PriorValue: 0.5, EMAValue: 0.5}
		if err := s.PutNode(d); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetNode("n1")
		if err != nil {
			t.Fatal(err)
		}
		if got.NodeID != d.NodeID || got.ConstructType != d.ConstructType {
			t.Errorf("roundtrip mismatch: got %+v", got)
		}
	})

	t.Run("PutNodeOverwrites", func(t *testing.T) {
		s := factory(t)
		_ = s.PutNode(&types.NodeDescriptor{NodeID: "n1", ConstructType: "PS"})
		updated := &types.NodeDescriptor{NodeID: "n1", ConstructType: "SC", PriorValue: 0.8, EMAValue: 0.8}
		_ = s.PutNode(updated)
		got, _ := s.GetNode("n1")
		if got.ConstructType != "SC" {
			t.Errorf("expected overwrite; got %+v", got)
		}
	})

	t.Run("GetMissingEdgeReturnsNil", func(t *testing.T) {
		s := factory(t)
		edge, err := s.GetEdge("a", "b")
		if err != nil {
			t.Fatal(err)
		}
		if edge != nil {
			t.Error("expected nil for unknown edge")
		}
	})

	t.Run("PutGetEdgeRoundtrip", func(t *testing.T) {
		s := factory(t)
		d := &types.EdgeDescriptor{FromID: "a", ToID: "b", PropositionID: "P1", Direction: types.Positive, PriorWeight: 0.6, EMAWeight: 0.6}
		_ = s.PutEdge(d)
		got, err := s.GetEdge("a", "b")
		if err != nil {
			t.Fatal(err)
		}
		if got.PropositionID != "P1" || got.Direction != types.Positive {
			t.Errorf("roundtrip mismatch: got %+v", got)
		}
	})

	t.Run("EdgeIsDirected", func(t *testing.T) {
		s := factory(t)
		_ = s.PutEdge(&types.EdgeDescriptor{FromID: "a", ToID: "b", Direction: types.Positive, PriorWeight: 0.5, EMAWeight: 0.5})
		got, _ := s.GetEdge("b", "a")
		if got != nil {
			t.Error("reverse edge must not exist")
		}
	})

	t.Run("NeighborsUnknownNodeReturnsEmpty", func(t *testing.T) {
		s := factory(t)
		ns, err := s.Neighbors("ghost")
		if err != nil {
			t.Fatal(err)
		}
		if len(ns) != 0 {
			t.Errorf("expected empty, got %v", ns)
		}
	})

	t.Run("NeighborsReflectsEdges", func(t *testing.T) {
		s := factory(t)
		_ = s.PutEdge(&types.EdgeDescriptor{FromID: "x", ToID: "y", PriorWeight: 0.5, EMAWeight: 0.5})
		_ = s.PutEdge(&types.EdgeDescriptor{FromID: "x", ToID: "z", PriorWeight: 0.5, EMAWeight: 0.5})
		ns, _ := s.Neighbors("x")
		if len(ns) < 2 {
			t.Errorf("expected at least 2 neighbors, got %v", ns)
		}
	})

	t.Run("AllEdgesReturnsEveryEdge", func(t *testing.T) {
		s := factory(t)
		_ = s.PutEdge(&types.EdgeDescriptor{FromID: "a", ToID: "b", PriorWeight: 0.5, EMAWeight: 0.5})
		_ = s.PutEdge(&types.EdgeDescriptor{FromID: "b", ToID: "c", PriorWeight: 0.3, EMAWeight: 0.3})
		edges, err := s.AllEdges()
		if err != nil {
			t.Fatal(err)
		}
		if len(edges) < 2 {
			t.Errorf("expected >= 2 edges, got %d", len(edges))
		}
	})

	// ── Multigraph behavior ──────────────────────────────────────────────────
	//
	// Di-Select has three conflict-pair propositions (P2/P3, P5/P6, P7/P9)
	// where two propositions share the same (from, to) but differ in direction.
	// Storage MUST hold both as independent descriptors keyed by the full
	// (from, to, propositionID) triple.

	t.Run("ConflictPairCoexistsAsTwoEdges", func(t *testing.T) {
		s := factory(t)
		// P2: RC→PS negative (overhead reduces throughput)
		_ = s.PutEdge(&types.EdgeDescriptor{
			FromID: "RC", ToID: "PS", PropositionID: "P2",
			Direction: types.Negative, PriorWeight: 0.4, EMAWeight: 0.4,
		})
		// P3: RC→PS positive (lightweight reduces startup latency)
		_ = s.PutEdge(&types.EdgeDescriptor{
			FromID: "RC", ToID: "PS", PropositionID: "P3",
			Direction: types.Positive, PriorWeight: 0.5, EMAWeight: 0.5,
		})

		all, _ := s.AllEdges()
		count := 0
		for _, e := range all {
			if e.FromID == "RC" && e.ToID == "PS" {
				count++
			}
		}
		if count != 2 {
			t.Errorf("expected 2 edges on RC→PS (P2 + P3); got %d — storage collapsed the conflict pair", count)
		}

		pair, err := s.GetEdgesByPair("RC", "PS")
		if err != nil {
			t.Fatal(err)
		}
		if len(pair) != 2 {
			t.Errorf("GetEdgesByPair(RC,PS) should return 2 edges; got %d", len(pair))
		}
		// Each must retain its own PropositionID and Direction.
		seen := make(map[string]types.Direction, 2)
		for _, e := range pair {
			seen[e.PropositionID] = e.Direction
		}
		if seen["P2"] != types.Negative {
			t.Errorf("P2 should retain Negative direction; got %v", seen["P2"])
		}
		if seen["P3"] != types.Positive {
			t.Errorf("P3 should retain Positive direction; got %v", seen["P3"])
		}
	})

	t.Run("GetEdgeReturnsDeterministicPickFromPair", func(t *testing.T) {
		s := factory(t)
		_ = s.PutEdge(&types.EdgeDescriptor{
			FromID: "X", ToID: "Y", PropositionID: "P9",
			Direction: types.Negative, PriorWeight: 0.4, EMAWeight: 0.4,
		})
		_ = s.PutEdge(&types.EdgeDescriptor{
			FromID: "X", ToID: "Y", PropositionID: "P7",
			Direction: types.Positive, PriorWeight: 0.6, EMAWeight: 0.6,
		})
		first, err := s.GetEdge("X", "Y")
		if err != nil || first == nil {
			t.Fatalf("expected an edge for (X,Y); got err=%v edge=%v", err, first)
		}
		// Must always return the same one (lex-smallest PropositionID = P7 here).
		second, _ := s.GetEdge("X", "Y")
		if first.PropositionID != second.PropositionID {
			t.Errorf("GetEdge must be deterministic across calls; got %q then %q",
				first.PropositionID, second.PropositionID)
		}
	})

	t.Run("GetEdgesByPairUnknownReturnsEmpty", func(t *testing.T) {
		s := factory(t)
		out, err := s.GetEdgesByPair("ghost", "nobody")
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 0 {
			t.Errorf("expected empty result; got %d edges", len(out))
		}
	})

	t.Run("PutEdgeWithDifferentPropIDsAreIndependent", func(t *testing.T) {
		// Putting two edges with same (from, to) but different PropositionIDs
		// must NOT overwrite each other.
		s := factory(t)
		_ = s.PutEdge(&types.EdgeDescriptor{
			FromID: "P", ToID: "Q", PropositionID: "Pa",
			Direction: types.Positive, PriorWeight: 0.1, EMAWeight: 0.1,
		})
		_ = s.PutEdge(&types.EdgeDescriptor{
			FromID: "P", ToID: "Q", PropositionID: "Pb",
			Direction: types.Negative, PriorWeight: 0.9, EMAWeight: 0.9,
		})
		pair, _ := s.GetEdgesByPair("P", "Q")
		if len(pair) != 2 {
			t.Fatalf("expected 2 independent edges; got %d", len(pair))
		}
		weights := make(map[string]float64, 2)
		for _, e := range pair {
			weights[e.PropositionID] = e.PriorWeight
		}
		if weights["Pa"] != 0.1 || weights["Pb"] != 0.9 {
			t.Errorf("edges merged or overwritten; weights = %v", weights)
		}
	})

	t.Run("NeighborsDeduplicatesConflictPair", func(t *testing.T) {
		s := factory(t)
		_ = s.PutEdge(&types.EdgeDescriptor{
			FromID: "S", ToID: "T", PropositionID: "Pα",
			Direction: types.Positive, PriorWeight: 0.5, EMAWeight: 0.5,
		})
		_ = s.PutEdge(&types.EdgeDescriptor{
			FromID: "S", ToID: "T", PropositionID: "Pβ",
			Direction: types.Negative, PriorWeight: 0.5, EMAWeight: 0.5,
		})
		ns, _ := s.Neighbors("S")
		count := 0
		for _, n := range ns {
			if n == "T" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("Neighbors(S) should list T exactly once even with multiple edges; got %d", count)
		}
	})
}
