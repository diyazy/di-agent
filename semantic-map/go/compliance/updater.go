package compliance

import (
	"testing"

	"github.com/DiyazY/di-agent/pkg/contracts"
	"github.com/DiyazY/di-agent/pkg/types"
)

type UpdaterFactory func(t *testing.T) (contracts.UpdaterContract, contracts.StorageContract)

func RunUpdaterCompliance(t *testing.T, factory UpdaterFactory) {
	t.Helper()

	seed := func(t *testing.T) (contracts.UpdaterContract, contracts.StorageContract) {
		t.Helper()
		u, s := factory(t)
		_ = s.PutEdge(&types.EdgeDescriptor{
			FromID: "a", ToID: "b", PropositionID: "P1",
			Direction: types.Positive, PriorWeight: 0.5, EMAWeight: 0.5,
		})
		return u, s
	}

	t.Run("UpdateIncrementsObservationCount", func(t *testing.T) {
		u, s := seed(t)
		if _, err := u.UpdateEdge("a", "b", 0.6, "evt-1"); err != nil {
			t.Fatal(err)
		}
		edge, _ := s.GetEdge("a", "b")
		if edge.NObservations != 1 {
			t.Errorf("expected 1 observation, got %d", edge.NObservations)
		}
	})

	t.Run("UpdateShiftsEMATowardObservation", func(t *testing.T) {
		u, s := seed(t)
		_, _ = u.UpdateEdge("a", "b", 1.0, "evt-1")
		edge, _ := s.GetEdge("a", "b")
		if edge.EMAWeight <= 0.5 {
			t.Errorf("EMA should have increased toward 1.0, got %.4f", edge.EMAWeight)
		}
	})

	t.Run("IdempotentOnSameEventID", func(t *testing.T) {
		u, s := seed(t)
		_, _ = u.UpdateEdge("a", "b", 0.9, "evt-1")
		after1, _ := s.GetEdge("a", "b")
		_, _ = u.UpdateEdge("a", "b", 0.9, "evt-1")
		after2, _ := s.GetEdge("a", "b")
		if after1.EMAWeight != after2.EMAWeight || after1.NObservations != after2.NObservations {
			t.Error("second call with same event_id must not change state")
		}
	})

	t.Run("DifferentEventIDsAccumulate", func(t *testing.T) {
		u, s := seed(t)
		_, _ = u.UpdateEdge("a", "b", 0.6, "evt-1")
		_, _ = u.UpdateEdge("a", "b", 0.7, "evt-2")
		edge, _ := s.GetEdge("a", "b")
		if edge.NObservations != 2 {
			t.Errorf("expected 2 observations, got %d", edge.NObservations)
		}
	})

	t.Run("ResetRestoresPrior", func(t *testing.T) {
		u, s := seed(t)
		_, _ = u.UpdateEdge("a", "b", 0.9, "evt-1")
		if err := u.Reset("a", "b"); err != nil {
			t.Fatal(err)
		}
		edge, _ := s.GetEdge("a", "b")
		if edge.EMAWeight != edge.PriorWeight {
			t.Errorf("EMA %.4f should equal prior %.4f after reset", edge.EMAWeight, edge.PriorWeight)
		}
		if edge.NObservations != 0 {
			t.Errorf("NObservations should be 0 after reset, got %d", edge.NObservations)
		}
		if edge.Confidence != 0.0 {
			t.Errorf("Confidence should be 0.0 after reset, got %.4f", edge.Confidence)
		}
	})

	t.Run("ResetDoesNotDeleteEdge", func(t *testing.T) {
		u, s := seed(t)
		_ = u.Reset("a", "b")
		edge, _ := s.GetEdge("a", "b")
		if edge == nil {
			t.Error("edge must still exist after reset")
		}
	})

	t.Run("ConfidenceIncreasesWithObservations", func(t *testing.T) {
		u, s := seed(t)
		c0, _ := s.GetEdge("a", "b")
		_, _ = u.UpdateEdge("a", "b", 0.6, "evt-1")
		c1, _ := s.GetEdge("a", "b")
		if c1.Confidence <= c0.Confidence {
			t.Errorf("confidence should increase: %.4f -> %.4f", c0.Confidence, c1.Confidence)
		}
	})
}
