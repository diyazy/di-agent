package minimal

import (
	"fmt"
	"sync"

	"github.com/DiyazY/di-agent/pkg/types"
)

// StaticDiSelectOntology is the edge-minimal OntologyContract implementation.
// The 7 constructs and 15 propositions from Di-Select are hardcoded as the
// bootstrap minimum. Additional validated propositions can be added at runtime
// via AddValidatedProposition; they do not persist across restarts.
type StaticDiSelectOntology struct {
	mu          sync.RWMutex
	constructs  []*types.Construct
	propositions []*types.Proposition
}

func NewStaticDiSelectOntology() *StaticDiSelectOntology {
	return &StaticDiSelectOntology{
		constructs:   diSelectConstructs(),
		propositions: diSelectPropositions(),
	}
}

func (o *StaticDiSelectOntology) Constructs() ([]*types.Construct, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.constructs, nil
}

func (o *StaticDiSelectOntology) Propositions() ([]*types.Proposition, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.propositions, nil
}

func (o *StaticDiSelectOntology) Relationships(constructID string) ([]*types.Proposition, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	var out []*types.Proposition
	for _, p := range o.propositions {
		if p.FromConstruct == constructID || p.ToConstruct == constructID {
			out = append(out, p)
		}
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
	res, err := o.ValidateProposition(p.FromConstruct, p.ToConstruct, p.Direction)
	if err != nil {
		return err
	}
	if !res.Valid {
		return fmt.Errorf("proposition contradicts existing backbone: conflicts=%v", res.Conflicts)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.propositions = append(o.propositions, p)
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
	return []*types.Proposition{
		{PropositionID: "P1",  FromConstruct: "SC", ToConstruct: "RC", Direction: types.Positive, PriorStrength: 0.6, EvidenceSources: []string{"P1", "P2", "P4"}},
		{PropositionID: "P2",  FromConstruct: "RC", ToConstruct: "PS", Direction: types.Negative, PriorStrength: 0.4, EvidenceSources: []string{"P1", "P4"}},
		{PropositionID: "P3",  FromConstruct: "RC", ToConstruct: "PS", Direction: types.Positive, PriorStrength: 0.5, EvidenceSources: []string{"P1", "P4"}},
		{PropositionID: "P4",  FromConstruct: "SC", ToConstruct: "RR", Direction: types.Negative, PriorStrength: 0.4, EvidenceSources: []string{"P2"}},
		{PropositionID: "P5",  FromConstruct: "CO", ToConstruct: "RR", Direction: types.Positive, PriorStrength: 0.7, EvidenceSources: []string{"P2"}},
		{PropositionID: "P6",  FromConstruct: "CO", ToConstruct: "RR", Direction: types.Negative, PriorStrength: 0.5, EvidenceSources: []string{"P2"}},
		{PropositionID: "P7",  FromConstruct: "CE", ToConstruct: "MU", Direction: types.Positive, PriorStrength: 0.6, EvidenceSources: []string{"P2"}},
		{PropositionID: "P8",  FromConstruct: "MU", ToConstruct: "RC", Direction: types.Negative, PriorStrength: 0.5, EvidenceSources: []string{"P2"}},
		{PropositionID: "P9",  FromConstruct: "CE", ToConstruct: "MU", Direction: types.Negative, PriorStrength: 0.4, EvidenceSources: []string{"P2"}},
		{PropositionID: "P10", FromConstruct: "PS", ToConstruct: "RC", Direction: types.Negative, PriorStrength: 0.5, EvidenceSources: []string{"P1", "P4"}},
		{PropositionID: "P11", FromConstruct: "CE", ToConstruct: "SC", Direction: types.Positive, PriorStrength: 0.5, EvidenceSources: []string{"P2"}},
		{PropositionID: "P12", FromConstruct: "SC", ToConstruct: "MU", Direction: types.Negative, PriorStrength: 0.6, EvidenceSources: []string{"P2"}},
		{PropositionID: "P13", FromConstruct: "CO", ToConstruct: "PS", Direction: types.Negative, PriorStrength: 0.5, EvidenceSources: []string{"P1"}},
		{PropositionID: "P14", FromConstruct: "RC", ToConstruct: "SC", Direction: types.Negative, PriorStrength: 0.5, EvidenceSources: []string{"P2", "P5"}},
		{PropositionID: "P15", FromConstruct: "MU", ToConstruct: "RR", Direction: types.Positive, PriorStrength: 0.5, EvidenceSources: []string{"P2"}},
	}
}
