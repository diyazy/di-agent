// DTOs for the agent HTTP API.
//
// These types define the JSON wire shape served by the daemon's new
// /graph, /edges, /constructs, /propositions, /history and /ontology/*
// endpoints. They are deliberately distinct from pkg/types so the wire
// format can evolve independently of the internal data model.
//
// Critical wire decision: types.Direction (an int with values 0/1) is
// serialized as the strings "+" / "-". This keeps the JSON readable for
// curl, CLI tables, and the embedded UI.

package main

import (
	"time"

	"github.com/DiyazY/di-agent/pkg/types"
)

// ── Request DTOs ──────────────────────────────────────────────────────────────

// SetStrengthRequest is the body of POST /ontology/strength.
type SetStrengthRequest struct {
	PropositionID string  `json:"proposition_id"`
	Strength      float64 `json:"strength"`
}

// DeprecateRequest is the body of POST /ontology/deprecate.
type DeprecateRequest struct {
	PropositionID string `json:"proposition_id"`
	Reason        string `json:"reason"`
}

// AddConstructRequest is the body of POST /ontology/construct.
type AddConstructRequest struct {
	ConstructID string `json:"construct_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// AddPropositionRequest is the body of POST /ontology/proposition.
// Direction is "+" (positive) or "-" (negative); the handler converts it.
type AddPropositionRequest struct {
	PropositionID string  `json:"proposition_id"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	Direction     string  `json:"direction"`
	PriorStrength float64 `json:"prior_strength"`
}

// ResetRequest is the body of POST /agent/reset.
type ResetRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ── Response DTOs ─────────────────────────────────────────────────────────────

// GraphSnapshot is the top-level response of GET /graph.
type GraphSnapshot struct {
	Constructs   []ConstructDTO   `json:"constructs"`
	Propositions []PropositionDTO `json:"propositions"`
	Edges        []EdgeDTO        `json:"edges"`
}

// ConstructDTO mirrors types.Construct for wire output.
type ConstructDTO struct {
	ConstructID string `json:"construct_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PropositionDTO mirrors types.Proposition. Direction is rendered as "+"/"-".
type PropositionDTO struct {
	PropositionID    string   `json:"proposition_id"`
	FromConstruct    string   `json:"from"`
	ToConstruct      string   `json:"to"`
	Direction        string   `json:"direction"`
	PriorStrength    float64  `json:"prior_strength"`
	EvidenceSources  []string `json:"evidence_sources,omitempty"`
	Deprecated       bool     `json:"deprecated"`
	DeprecatedReason string   `json:"deprecated_reason,omitempty"`
}

// EdgeDTO mirrors types.EdgeDescriptor. Direction is rendered as "+"/"-";
// Mu and Sigma encode as null when the Gaussian descriptor is unavailable.
type EdgeDTO struct {
	FromID        string   `json:"from"`
	ToID          string   `json:"to"`
	PropositionID string   `json:"proposition_id"`
	Direction     string   `json:"direction"`
	PriorWeight   float64  `json:"prior_weight"`
	EMAWeight     float64  `json:"ema_weight"`
	Confidence    float64  `json:"confidence"`
	NObservations int      `json:"n_observations"`
	Mu            *float64 `json:"mu"`
	Sigma         *float64 `json:"sigma"`
}

// OntologyEventDTO mirrors types.OntologyEvent.
type OntologyEventDTO struct {
	Timestamp time.Time      `json:"timestamp"`
	Actor     string         `json:"actor"`
	Kind      string         `json:"kind"`
	TargetID  string         `json:"target_id"`
	Detail    map[string]any `json:"detail,omitempty"`
}

// HealthResponse is the body of GET /healthz.
type HealthResponse struct {
	OK bool `json:"ok"`
}

// VersionResponse is the body of GET /version.
type VersionResponse struct {
	AgentVersion        string `json:"agent_version"`
	GoVersion           string `json:"go_version"`
	BuildCommit         string `json:"build_commit"`
	SemmapConstructs    int    `json:"semmap_constructs"`
	SemmapPropositions  int    `json:"semmap_propositions"`
}

// ErrorResponse is the body of any 4xx/5xx returned by a NEW endpoint.
type ErrorResponse struct {
	Error string `json:"error"`
}

// ── Mappers ───────────────────────────────────────────────────────────────────

// directionToString converts the internal Direction enum to its wire form.
func directionToString(d types.Direction) string {
	if d == types.Negative {
		return "-"
	}
	return "+"
}

// directionFromString parses a wire direction string back to the enum.
// Returns (Positive, true) for "+", (Negative, true) for "-", (0, false)
// for anything else.
func directionFromString(s string) (types.Direction, bool) {
	switch s {
	case "+":
		return types.Positive, true
	case "-":
		return types.Negative, true
	}
	return 0, false
}

func constructToDTO(c *types.Construct) ConstructDTO {
	return ConstructDTO{
		ConstructID: c.ConstructID,
		Name:        c.Name,
		Description: c.Description,
	}
}

func propositionToDTO(p *types.Proposition) PropositionDTO {
	return PropositionDTO{
		PropositionID:    p.PropositionID,
		FromConstruct:    p.FromConstruct,
		ToConstruct:      p.ToConstruct,
		Direction:        directionToString(p.Direction),
		PriorStrength:    p.PriorStrength,
		EvidenceSources:  p.EvidenceSources,
		Deprecated:       p.Deprecated,
		DeprecatedReason: p.DeprecatedReason,
	}
}

func edgeToDTO(e *types.EdgeDescriptor) EdgeDTO {
	return EdgeDTO{
		FromID:        e.FromID,
		ToID:          e.ToID,
		PropositionID: e.PropositionID,
		Direction:     directionToString(e.Direction),
		PriorWeight:   e.PriorWeight,
		EMAWeight:     e.EMAWeight,
		Confidence:    e.Confidence,
		NObservations: e.NObservations,
		Mu:            e.Mu,
		Sigma:         e.Sigma,
	}
}

func eventToDTO(e *types.OntologyEvent) OntologyEventDTO {
	return OntologyEventDTO{
		Timestamp: e.Timestamp,
		Actor:     e.Actor,
		Kind:      string(e.Kind),
		TargetID:  e.TargetID,
		Detail:    e.Detail,
	}
}
