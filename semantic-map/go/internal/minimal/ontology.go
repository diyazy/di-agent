package minimal

import (
	"fmt"
	"sync"
	"time"

	"github.com/DiyazY/di-agent/pkg/types"
)

// StaticDiSelectOntology is the edge-minimal OntologyContract implementation.
// The 7 constructs and 15 propositions from Di-Select are hardcoded as the
// bootstrap minimum. The ontology is live — constructs and propositions may
// be added, prior strengths recalibrated, and propositions deprecated at
// runtime. Mutations append to an in-memory audit log readable via
// GetHistory. The log is ephemeral on this profile (lost on restart); the
// cloud-full profile persists it.
type StaticDiSelectOntology struct {
	mu           sync.RWMutex
	constructs   []*types.Construct
	propositions []*types.Proposition
	events       []*types.OntologyEvent

	// now overrides time.Now for deterministic testing. Production callers
	// leave it nil and the implementation uses the wall clock.
	now func() time.Time
}

func NewStaticDiSelectOntology() *StaticDiSelectOntology {
	return &StaticDiSelectOntology{
		constructs:   diSelectConstructs(),
		propositions: diSelectPropositions(),
	}
}

// appendEvent records one mutation in the audit log. Callers hold o.mu.
// actor defaults to "system" when the parameter is empty.
func (o *StaticDiSelectOntology) appendEvent(actor string, kind types.OntologyEventKind, targetID string, detail map[string]any) {
	if actor == "" {
		actor = "system"
	}
	var ts time.Time
	if o.now != nil {
		ts = o.now()
	} else {
		ts = time.Now().UTC()
	}
	o.events = append(o.events, &types.OntologyEvent{
		Timestamp: ts,
		Actor:     actor,
		Kind:      kind,
		TargetID:  targetID,
		Detail:    detail,
	})
}

// Constructs returns a defensive copy of the construct list. Callers may mutate
// the returned slice or its elements without affecting the ontology's internal
// state; to register a new construct, use the ontology's setters.
func (o *StaticDiSelectOntology) Constructs() ([]*types.Construct, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]*types.Construct, len(o.constructs))
	for i, c := range o.constructs {
		cp := *c
		out[i] = &cp
	}
	return out, nil
}

// Propositions returns a defensive copy of the proposition list. Mutating
// returned entries does NOT update the ontology — use SetPropositionStrength
// (or AddValidatedProposition) to make changes.
func (o *StaticDiSelectOntology) Propositions() ([]*types.Proposition, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]*types.Proposition, len(o.propositions))
	for i, p := range o.propositions {
		cp := *p
		// EvidenceSources is a slice — copy it to avoid shared backing array.
		if len(p.EvidenceSources) > 0 {
			cp.EvidenceSources = append([]string(nil), p.EvidenceSources...)
		}
		out[i] = &cp
	}
	return out, nil
}

func (o *StaticDiSelectOntology) Relationships(constructID string) ([]*types.Proposition, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	var out []*types.Proposition
	for _, p := range o.propositions {
		if p.FromConstruct == constructID || p.ToConstruct == constructID {
			cp := *p
			if len(p.EvidenceSources) > 0 {
				cp.EvidenceSources = append([]string(nil), p.EvidenceSources...)
			}
			out = append(out, &cp)
		}
	}
	return out, nil
}

// SetPropositionStrength updates the PriorStrength of an existing proposition
// and appends an EventPropositionStrengthSet entry to the history. This is the
// safe write path used by the prior initialization pipeline and by operator
// tuning — pointer mutation through Propositions() is not supported because
// that method returns defensive copies.
//
// Returns an error if the proposition ID is not found.
func (o *StaticDiSelectOntology) SetPropositionStrength(propositionID string, strength float64) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, p := range o.propositions {
		if p.PropositionID == propositionID {
			old := p.PriorStrength
			p.PriorStrength = strength
			o.appendEvent("system", types.EventPropositionStrengthSet, propositionID, map[string]any{
				"strength_old": old,
				"strength_new": strength,
			})
			return nil
		}
	}
	return fmt.Errorf("proposition %q not found", propositionID)
}

// AddConstruct appends a new construct to the ontology. Constructs are
// append-only — there is no removal path because constructs are domain-stable
// per the architecture. Duplicate ConstructIDs are rejected.
func (o *StaticDiSelectOntology) AddConstruct(c *types.Construct) error {
	if c == nil || c.ConstructID == "" {
		return fmt.Errorf("AddConstruct: nil construct or empty ConstructID")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, existing := range o.constructs {
		if existing.ConstructID == c.ConstructID {
			return fmt.Errorf("AddConstruct: ConstructID %q already exists", c.ConstructID)
		}
	}
	cp := *c
	o.constructs = append(o.constructs, &cp)
	o.appendEvent("system", types.EventConstructAdded, cp.ConstructID, map[string]any{
		"name":        cp.Name,
		"description": cp.Description,
	})
	return nil
}

// Deprecate marks a proposition as no-longer-endorsed. The proposition stays
// in the ontology (visible to GetHistory replay and to clients that walk the
// full backbone) but Reasoners must skip it during cost computation.
// Idempotent: calling Deprecate twice on the same proposition is a no-op on
// the second call (no duplicate event, no error).
//
// Returns an error if the proposition ID is not found.
func (o *StaticDiSelectOntology) Deprecate(propositionID, reason string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, p := range o.propositions {
		if p.PropositionID == propositionID {
			if p.Deprecated {
				return nil // idempotent
			}
			p.Deprecated = true
			p.DeprecatedReason = reason
			o.appendEvent("system", types.EventPropositionDeprecated, propositionID, map[string]any{
				"reason": reason,
			})
			return nil
		}
	}
	return fmt.Errorf("proposition %q not found", propositionID)
}

// GetHistory returns ontology mutation events appended at or after `since`,
// in chronological insertion order. Pass a zero time.Time to retrieve the
// full log. The returned slice is a defensive copy; mutating it does not
// affect the ontology's internal log.
func (o *StaticDiSelectOntology) GetHistory(since time.Time) ([]*types.OntologyEvent, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]*types.OntologyEvent, 0, len(o.events))
	for _, e := range o.events {
		if !since.IsZero() && e.Timestamp.Before(since) {
			continue
		}
		cp := *e
		if len(e.Detail) > 0 {
			cp.Detail = make(map[string]any, len(e.Detail))
			for k, v := range e.Detail {
				cp.Detail[k] = v
			}
		}
		out = append(out, &cp)
	}
	return out, nil
}

func (o *StaticDiSelectOntology) ValidateProposition(fromID, toID string, dir types.Direction) (*types.ValidationResult, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	res := &types.ValidationResult{Valid: true}
	for _, p := range o.propositions {
		if p.FromConstruct == fromID && p.ToConstruct == toID && p.Direction != dir {
			res.Valid = false
			res.Conflicts = append(res.Conflicts, p.PropositionID)
		}
	}
	return res, nil
}

func (o *StaticDiSelectOntology) AddValidatedProposition(p *types.Proposition) error {
	if p == nil || p.PropositionID == "" {
		return fmt.Errorf("AddValidatedProposition: nil proposition or empty PropositionID")
	}
	res, err := o.ValidateProposition(p.FromConstruct, p.ToConstruct, p.Direction)
	if err != nil {
		return err
	}
	if !res.Valid {
		return fmt.Errorf("proposition contradicts existing backbone: conflicts=%v", res.Conflicts)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	// Defensive copy: the ontology owns its proposition entries; the caller
	// must not be able to mutate them via their original pointer.
	cp := *p
	if len(p.EvidenceSources) > 0 {
		cp.EvidenceSources = append([]string(nil), p.EvidenceSources...)
	}
	o.propositions = append(o.propositions, &cp)
	o.appendEvent("system", types.EventPropositionAdded, cp.PropositionID, map[string]any{
		"from":           cp.FromConstruct,
		"to":             cp.ToConstruct,
		"direction":      cp.Direction,
		"prior_strength": cp.PriorStrength,
	})
	return nil
}

// ── Di-Select bootstrap data ──────────────────────────────────────────────────
// Construct IDs match the short names used as edge FromID/ToID throughout the system.

func diSelectConstructs() []*types.Construct {
	return []*types.Construct{
		{ConstructID: "PS", Name: "Performance & Scalability", Description: "Latency, throughput, pod startup, cluster scaling, scheduling efficiency"},
		{ConstructID: "SC", Name: "Security & Compliance", Description: "CIS benchmarks, encryption, FIPS compliance, attack surface, CVE patching"},
		{ConstructID: "RR", Name: "Reliability & Resilience", Description: "Recovery time, fault tolerance, disaster recovery, self-healing, HA"},
		{ConstructID: "MU", Name: "Maintainability & Usability", Description: "Installation ease, configuration complexity, upgrade burden, automation"},
		{ConstructID: "RC", Name: "Resource Constraints & Cost", Description: "Memory/CPU footprint, energy efficiency, hardware and operational costs"},
		{ConstructID: "CO", Name: "Connectivity & Offline Resilience", Description: "Offline autonomy, bandwidth optimization, disconnected operation, sync"},
		{ConstructID: "CE", Name: "Community & Ecosystem", Description: "Vendor backing, plugin availability, documentation, long-term stability"},
	}
}

func diSelectPropositions() []*types.Proposition {
	// Prior strengths are initialised from P1–P5 empirical evidence.
	// They will be refined by the prior initialization pipeline.
	//
	// Descriptions are the one-sentence causal statements from Di-Select
	// (see CLAUDE.md §"The 15 Causal Propositions"). They surface in the
	// /graph response and the UI edge panel so operators can read the
	// claim, not just the ID.
	return []*types.Proposition{
		{PropositionID: "P1",  FromConstruct: "SC", ToConstruct: "RC", Direction: types.Positive, PriorStrength: 0.6,
			Description:     "Security hardening increases CPU consumption.",
			EvidenceSources: []string{"P1", "P2", "P4"}},
		{PropositionID: "P2",  FromConstruct: "RC", ToConstruct: "PS", Direction: types.Negative, PriorStrength: 0.4,
			Description:     "CPU overhead from security reduces scheduling throughput.",
			EvidenceSources: []string{"P1", "P4"}},
		{PropositionID: "P3",  FromConstruct: "RC", ToConstruct: "PS", Direction: types.Positive, PriorStrength: 0.5,
			Description:     "Lightweight distributions reduce pod-startup latency.",
			EvidenceSources: []string{"P1", "P4"}},
		{PropositionID: "P4",  FromConstruct: "SC", ToConstruct: "RR", Direction: types.Negative, PriorStrength: 0.4,
			Description:     "Security hardening slows recovery time after failures.",
			EvidenceSources: []string{"P2"}},
		{PropositionID: "P5",  FromConstruct: "CO", ToConstruct: "RR", Direction: types.Positive, PriorStrength: 0.7,
			Description:     "Offline autonomy improves continuity during network partitions.",
			EvidenceSources: []string{"P2"}},
		{PropositionID: "P6",  FromConstruct: "CO", ToConstruct: "RR", Direction: types.Negative, PriorStrength: 0.5,
			Description:     "Cloud dependency reduces stability in poor networks.",
			EvidenceSources: []string{"P2"}},
		{PropositionID: "P7",  FromConstruct: "CE", ToConstruct: "MU", Direction: types.Positive, PriorStrength: 0.6,
			Description:     "Rich ecosystem lowers operator effort.",
			EvidenceSources: []string{"P2"}},
		{PropositionID: "P8",  FromConstruct: "MU", ToConstruct: "RC", Direction: types.Negative, PriorStrength: 0.5,
			Description:     "Administrative simplicity reduces operational cost.",
			EvidenceSources: []string{"P2"}},
		{PropositionID: "P9",  FromConstruct: "CE", ToConstruct: "MU", Direction: types.Negative, PriorStrength: 0.4,
			Description:     "Excessive features increase maintenance complexity.",
			EvidenceSources: []string{"P2"}},
		{PropositionID: "P10", FromConstruct: "PS", ToConstruct: "RC", Direction: types.Negative, PriorStrength: 0.5,
			Description:     "Better efficiency enables equal workload at lower cost.",
			EvidenceSources: []string{"P1", "P4"}},
		{PropositionID: "P11", FromConstruct: "CE", ToConstruct: "SC", Direction: types.Positive, PriorStrength: 0.5,
			Description:     "Active communities accelerate security patch availability.",
			EvidenceSources: []string{"P2"}},
		{PropositionID: "P12", FromConstruct: "SC", ToConstruct: "MU", Direction: types.Negative, PriorStrength: 0.6,
			Description:     "Security controls add configuration and upgrade burden.",
			EvidenceSources: []string{"P2"}},
		{PropositionID: "P13", FromConstruct: "CO", ToConstruct: "PS", Direction: types.Negative, PriorStrength: 0.5,
			Description:     "Offline designs incur caching and sync overhead.",
			EvidenceSources: []string{"P1"}},
		{PropositionID: "P14", FromConstruct: "RC", ToConstruct: "SC", Direction: types.Negative, PriorStrength: 0.5,
			Description:     "Tight budgets lead to relaxed security hardening.",
			EvidenceSources: []string{"P2", "P5"}},
		{PropositionID: "P15", FromConstruct: "MU", ToConstruct: "RR", Direction: types.Positive, PriorStrength: 0.5,
			Description:     "Better automation shortens recovery time.",
			EvidenceSources: []string{"P2"}},
	}
}
