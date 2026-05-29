package compliance

import (
	"testing"

	"github.com/DiyazY/di-agent/pkg/contracts"
	"github.com/DiyazY/di-agent/pkg/types"
)

// ProposerFactory builds a fresh ProposerContract for one compliance subtest.
type ProposerFactory func(t *testing.T) contracts.ProposerContract

// RunProposerCompliance verifies the behavioral guarantees of ProposerContract:
//
//   - Observe is non-fatal — it never returns errors on well-formed input.
//   - GetCandidates returns only Pending entries (Confirmed/Rejected/Deferred
//     candidates live in history but are not "pending").
//   - Reject permanently suppresses a candidate from future GetCandidates calls.
//   - Rejected candidates appear in GetHistory with status Rejected.
//   - Defer moves a candidate out of pending.
//   - GetHistory is a superset of GetCandidates (every pending entry has a
//     history record).
//
// Implementations that do not generate candidates (e.g. DisabledProposer) pass
// every assertion vacuously — this is the correct behavior for a "no proposals"
// implementation.
func RunProposerCompliance(t *testing.T, factory ProposerFactory) {
	t.Helper()

	// feed pushes n strongly-correlated observations across one construct pair.
	// Implementations that mine correlations should generate at least one
	// candidate after this; implementations that don't (DisabledProposer) will
	// return an empty candidate list, which is also valid.
	feed := func(p contracts.ProposerContract) {
		for i := 0; i < 100; i++ {
			x := float64(i)
			y := x * 0.9
			_ = p.Observe("PS", "RC", x, y)
		}
	}

	// ── Observe ───────────────────────────────────────────────────────────────

	t.Run("ObserveDoesNotError", func(t *testing.T) {
		p := factory(t)
		if err := p.Observe("a", "b", 0.5, 0.4); err != nil {
			t.Errorf("Observe must not error on well-formed input; got %v", err)
		}
	})

	t.Run("ObserveDoesNotPanicOnRepeat", func(t *testing.T) {
		p := factory(t)
		for i := 0; i < 50; i++ {
			if err := p.Observe("a", "b", float64(i), float64(i)*0.5); err != nil {
				t.Fatalf("Observe error at iteration %d: %v", i, err)
			}
		}
	})

	// ── GetCandidates ─────────────────────────────────────────────────────────

	t.Run("GetCandidatesReturnsOnlyPending", func(t *testing.T) {
		p := factory(t)
		feed(p)
		cs, err := p.GetCandidates()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			if c.Status != types.Pending {
				t.Errorf("GetCandidates returned non-pending candidate %q with status %v",
					c.CandidateID, c.Status)
			}
		}
	})

	t.Run("GetCandidatesInitiallyEmpty", func(t *testing.T) {
		p := factory(t)
		cs, err := p.GetCandidates()
		if err != nil {
			t.Fatal(err)
		}
		if len(cs) != 0 {
			t.Errorf("expected empty candidates before any Observe; got %d", len(cs))
		}
	})

	// ── Reject ───────────────────────────────────────────────────────────────

	t.Run("RejectSuppressesCandidate", func(t *testing.T) {
		p := factory(t)
		feed(p)
		cs, _ := p.GetCandidates()
		if len(cs) == 0 {
			t.Skip("implementation generated no candidates — vacuously satisfies Reject contract")
		}
		cid := cs[0].CandidateID
		if err := p.Reject(cid); err != nil {
			t.Fatal(err)
		}
		after, _ := p.GetCandidates()
		for _, c := range after {
			if c.CandidateID == cid {
				t.Errorf("rejected candidate %q must not appear in GetCandidates", cid)
			}
		}
	})

	t.Run("RejectedAppearsInHistoryWithCorrectStatus", func(t *testing.T) {
		p := factory(t)
		feed(p)
		cs, _ := p.GetCandidates()
		if len(cs) == 0 {
			t.Skip("implementation generated no candidates — history check skipped")
		}
		cid := cs[0].CandidateID
		if err := p.Reject(cid); err != nil {
			t.Fatal(err)
		}
		history, err := p.GetHistory()
		if err != nil {
			t.Fatal(err)
		}
		var found *types.CandidateEdge
		for _, h := range history {
			if h.CandidateID == cid {
				found = h
				break
			}
		}
		if found == nil {
			t.Errorf("rejected candidate %q must appear in GetHistory", cid)
			return
		}
		if found.Status != types.Rejected {
			t.Errorf("history entry for %q has status %v; expected Rejected", cid, found.Status)
		}
	})

	// ── Defer ────────────────────────────────────────────────────────────────

	t.Run("DeferMovesCandidateOutOfPending", func(t *testing.T) {
		p := factory(t)
		feed(p)
		cs, _ := p.GetCandidates()
		if len(cs) == 0 {
			t.Skip("implementation generated no candidates — defer check skipped")
		}
		cid := cs[0].CandidateID
		if err := p.Defer(cid); err != nil {
			t.Fatal(err)
		}
		after, _ := p.GetCandidates()
		for _, c := range after {
			if c.CandidateID == cid {
				t.Errorf("deferred candidate %q must not appear in GetCandidates", cid)
			}
		}
	})

	// ── GetHistory ───────────────────────────────────────────────────────────

	t.Run("HistoryIsSupersetOfPending", func(t *testing.T) {
		p := factory(t)
		feed(p)
		pending, _ := p.GetCandidates()
		history, err := p.GetHistory()
		if err != nil {
			t.Fatal(err)
		}
		historyIDs := make(map[string]bool, len(history))
		for _, h := range history {
			historyIDs[h.CandidateID] = true
		}
		for _, c := range pending {
			if !historyIDs[c.CandidateID] {
				t.Errorf("pending candidate %q missing from GetHistory — history must be a superset",
					c.CandidateID)
			}
		}
	})
}
