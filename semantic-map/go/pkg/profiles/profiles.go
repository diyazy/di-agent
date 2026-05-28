// Package profiles wires contract implementations into a ready SemanticMap.
// Agent code calls Build("edge-minimal") — it never imports internal packages.
package profiles

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/DiyazY/di-agent/internal/minimal"
	"github.com/DiyazY/di-agent/pkg/semmap"
	"github.com/DiyazY/di-agent/pkg/types"
)

// Config holds profile-specific tuning parameters.
type Config struct {
	// EMAAlpha is the decay factor for the EMA updater (0 < alpha < 1).
	// Higher = reacts faster to change; lower = more stable. Default: 0.2.
	EMAAlpha float64

	// ConvergenceThreshold is the number of observations at which an edge's
	// confidence reaches 1.0. Default: 500.
	ConvergenceThreshold float64

	// MinTrustScore is the minimum trust score a peer must have to be
	// considered for task offloading. Default: 0.5.
	MinTrustScore float64

	// PriorWeightsPath is an optional path to a prior_weights.json file
	// produced by the Python prior initialization pipeline. When set, the
	// proposition PriorStrength values loaded from the file override the
	// hardcoded Di-Select defaults.  Empty string = use hardcoded defaults.
	PriorWeightsPath string
}

func DefaultConfig() Config {
	return Config{
		EMAAlpha:             0.2,
		ConvergenceThreshold: 500,
		MinTrustScore:        0.5,
		PriorWeightsPath:     "",
	}
}

// ── prior_weights.json schema ─────────────────────────────────────────────────

// priorWeightsFile mirrors the top-level structure of prior_weights.json.
type priorWeightsFile struct {
	Version      string                       `json:"version"`
	GeneratedAt  string                       `json:"generated_at"`
	Propositions map[string]propositionPrior  `json:"propositions"`
}

type propositionPrior struct {
	PriorStrength float64 `json:"prior_strength"`
	Direction     string  `json:"direction"`
	Method        string  `json:"method"`
}

// loadPriorWeights reads prior_weights.json from the given path.
// Returns nil (no error) if the path is empty — caller uses hardcoded defaults.
func loadPriorWeights(path string) (*priorWeightsFile, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("prior_weights: cannot read %q: %w", path, err)
	}
	var pw priorWeightsFile
	if err := json.Unmarshal(data, &pw); err != nil {
		return nil, fmt.Errorf("prior_weights: malformed JSON in %q: %w", path, err)
	}
	return &pw, nil
}

// ── Build ─────────────────────────────────────────────────────────────────────

// Build constructs and returns a fully wired SemanticMap for the named profile.
func Build(profileName string, cfg Config) (*semmap.SemanticMap, error) {
	pw, err := loadPriorWeights(cfg.PriorWeightsPath)
	if err != nil {
		return nil, err
	}
	switch profileName {
	case "edge-minimal":
		return buildEdgeMinimal(cfg, pw), nil
	default:
		return nil, fmt.Errorf("unknown profile %q", profileName)
	}
}

func buildEdgeMinimal(cfg Config, pw *priorWeightsFile) *semmap.SemanticMap {
	storage  := minimal.NewInMemoryStorage()
	ontology := minimal.NewStaticDiSelectOntology()
	updater  := minimal.NewEMAUpdater(storage, cfg.EMAAlpha, cfg.ConvergenceThreshold)
	reasoner := minimal.NewRuleEngineReasoner(storage, ontology, cfg.MinTrustScore)
	proposer := minimal.NewDisabledProposer()

	// Apply calibrated priors from pipeline output before seeding storage.
	if pw != nil {
		applyPriorWeights(ontology, pw)
	}

	seedFromOntology(storage, ontology)

	return semmap.New(storage, ontology, updater, reasoner, proposer)
}

// applyPriorWeights overwrites the proposition PriorStrength values in the
// ontology with those from prior_weights.json.  Unknown proposition IDs are
// silently ignored so old files remain compatible with new code.
func applyPriorWeights(ontology *minimal.StaticDiSelectOntology, pw *priorWeightsFile) {
	props, _ := ontology.Propositions()
	for _, p := range props {
		if entry, ok := pw.Propositions[p.PropositionID]; ok {
			p.PriorStrength = entry.PriorStrength
		}
	}
}

// seedFromOntology pre-populates storage with one node per construct and one
// edge per proposition, using Di-Select prior strengths as the initial values.
func seedFromOntology(storage *minimal.InMemoryStorage, ontology *minimal.StaticDiSelectOntology) {
	constructs, _ := ontology.Constructs()
	for _, c := range constructs {
		_ = storage.PutNode(&types.NodeDescriptor{
			NodeID:        c.ConstructID,
			ConstructType: c.Name,
			PriorValue:    0.5, // neutral prior; per-distribution values seeded by Netdata adapter
			EMAValue:      0.5,
			Confidence:    0.0,
			NObservations: 0,
		})
	}

	propositions, _ := ontology.Propositions()
	for _, p := range propositions {
		_ = storage.PutEdge(&types.EdgeDescriptor{
			FromID:        p.FromConstruct,
			ToID:          p.ToConstruct,
			PropositionID: p.PropositionID,
			Direction:     p.Direction,
			PriorWeight:   p.PriorStrength,
			EMAWeight:     p.PriorStrength, // starts equal to prior
			Confidence:    0.0,
			NObservations: 0,
		})
	}
}
