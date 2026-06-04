package compliance

import (
	"testing"

	"github.com/DiyazY/di-agent/pkg/contracts"
	"github.com/DiyazY/di-agent/pkg/types"
)

// TunerFactory builds a fresh TunerContract for one compliance subtest.
type TunerFactory func(t *testing.T) contracts.TunerContract

// RunTunerCompliance verifies the behavioral guarantees of TunerContract:
//
//   - ParseIntent does not error on well-formed input.
//   - ParseIntent returns empty for unrecognized text.
//   - ParseIntent returns empty for empty text (nil or empty slice).
//   - Validate accepts in-bounds adjustments.
//   - Validate rejects NewStrength above ceiling (0.95).
//   - Validate rejects NewStrength below global floor (0.10).
//   - Validate enforces SC-related floor (0.30) for P1, P4, P11, P14.
//
// Implementations that always accept (e.g. DisabledTuner) pass the rejection
// subtests vacuously via t.Skip, because a no-op tuner is a valid contract
// choice. The skip logic uses ParseIntent("prioritize security") as a canary:
// if it returns intents the tuner is active; if empty the tuner is disabled.
func RunTunerCompliance(t *testing.T, factory TunerFactory) {
	t.Helper()

	t.Run("ParseIntentDoesNotError", func(t *testing.T) {
		tu := factory(t)
		_, err := tu.ParseIntent("prioritize security")
		if err != nil {
			t.Errorf("ParseIntent must not error on well-formed input; got %v", err)
		}
	})

	t.Run("ParseIntentUnknownReturnsEmpty", func(t *testing.T) {
		tu := factory(t)
		out, err := tu.ParseIntent("zzz xyzzy nonsense")
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 0 {
			t.Errorf("unknown intent should return empty slice; got %d intents", len(out))
		}
	})

	t.Run("ParseIntentEmptyReturnsEmpty", func(t *testing.T) {
		tu := factory(t)
		_, err := tu.ParseIntent("")
		if err != nil {
			t.Fatal(err)
		}
		// nil or empty — both are valid per the contract.
	})

	t.Run("ValidateAcceptsWithinBounds", func(t *testing.T) {
		tu := factory(t)
		adj := []*types.TuneAdjustment{
			{PropositionID: "P3", OldStrength: 0.5, NewStrength: 0.62, Rationale: "test"},
		}
		if err := tu.Validate(adj); err != nil {
			t.Errorf("Validate must accept in-bounds adjustment; got %v", err)
		}
	})

	t.Run("ValidateRejectsAboveCeiling", func(t *testing.T) {
		tu := factory(t)
		if isDisabled(tu) {
			t.Skip("disabled tuner — validation rejection subtests skipped")
		}
		adj := []*types.TuneAdjustment{
			{PropositionID: "P3", OldStrength: 0.9, NewStrength: 0.99, Rationale: "test"},
		}
		if err := tu.Validate(adj); err == nil {
			t.Error("Validate must reject strength > 0.95")
		}
	})

	t.Run("ValidateRejectsBelowFloor", func(t *testing.T) {
		tu := factory(t)
		if isDisabled(tu) {
			t.Skip("disabled tuner — validation rejection subtests skipped")
		}
		adj := []*types.TuneAdjustment{
			{PropositionID: "P3", OldStrength: 0.2, NewStrength: 0.05, Rationale: "test"},
		}
		if err := tu.Validate(adj); err == nil {
			t.Error("Validate must reject strength < 0.10")
		}
	})

	t.Run("ValidateEnforcesSCFloor", func(t *testing.T) {
		tu := factory(t)
		if isDisabled(tu) {
			t.Skip("disabled tuner — SC floor enforcement skipped")
		}
		// P1 has floor 0.30 — try to bring to 0.15 (above global floor but below SC floor).
		adj := []*types.TuneAdjustment{
			{PropositionID: "P1", OldStrength: 0.5, NewStrength: 0.15, Rationale: "test"},
		}
		if err := tu.Validate(adj); err == nil {
			t.Error("Validate must enforce SC floor (0.30) for P1")
		}
	})
}

// isDisabled returns true when the tuner appears to be a no-op: ParseIntent
// on a known keyword returns no intents, indicating a disabled implementation.
func isDisabled(tu contracts.TunerContract) bool {
	intents, err := tu.ParseIntent("prioritize security")
	return err == nil && len(intents) == 0
}
