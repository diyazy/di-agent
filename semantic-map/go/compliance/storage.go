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
}
