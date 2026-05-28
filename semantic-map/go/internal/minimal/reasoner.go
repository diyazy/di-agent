package minimal

import (
	"fmt"
	"math"
	"strings"

	"github.com/DiyazY/di-agent/pkg/contracts"
	"github.com/DiyazY/di-agent/pkg/types"
)

// RuleEngineReasoner is the edge-minimal ReasonerContract implementation.
//
// All decisions are deterministic: the reasoner traverses the Semantic Map
// graph using current blended values (prior + EMA weighted by confidence)
// and applies proposition-based rules. No ML model is involved.
//
// Blending formula:
//
//	effective = (1 - confidence) * prior + confidence * ema
type RuleEngineReasoner struct {
	storage      contracts.StorageContract
	ontology     contracts.OntologyContract
	minTrustScore float64
}

func NewRuleEngineReasoner(
	storage contracts.StorageContract,
	ontology contracts.OntologyContract,
	minTrustScore float64,
) *RuleEngineReasoner {
	return &RuleEngineReasoner{storage, ontology, minTrustScore}
}

func (r *RuleEngineReasoner) CostOfAction(taskType, nodeID string) (*types.ActionCost, error) {
	props, err := r.ontology.Propositions()
	if err != nil {
		return nil, err
	}

	var cpuCost, energyCost, latency float64
	var confidence float64
	var path []string

	for _, p := range props {
		edge, err := r.storage.GetEdge(p.FromConstruct, p.ToConstruct)
		if err != nil || edge == nil {
			continue
		}
		effective := blend(edge)
		path = append(path, fmt.Sprintf("%s→%s(%.2f)", p.FromConstruct, p.ToConstruct, effective))
		confidence += edge.Confidence

		switch p.ToConstruct {
		case "RC":
			energyCost += effective * sign(p.Direction)
		case "PS":
			latency += effective * sign(p.Direction)
		}
	}

	if len(props) > 0 {
		confidence /= float64(len(props))
	}
	cpuCost = latency * 0.1 // lightweight proxy; replaced by P4 prior initialization

	return &types.ActionCost{
		CPUCost:         math.Max(0, cpuCost),
		EnergyCost:      math.Max(0, energyCost),
		LatencyEstimate: math.Max(0, latency),
		Confidence:      confidence,
		Rationale:       fmt.Sprintf("task=%s node=%s path=[%s]", taskType, nodeID, strings.Join(path, ", ")),
		GraphPathUsed:   path,
	}, nil
}

func (r *RuleEngineReasoner) RecommendPeer(ctx *types.OffloadContext) (*types.PeerRecommendation, error) {
	neighbors, err := r.storage.Neighbors(ctx.SourceNodeID)
	if err != nil {
		return nil, err
	}

	var bestPeer string
	bestSavings := math.Inf(-1)
	var bestPath []string

	myCost, err := r.CostOfAction(ctx.TaskType, ctx.SourceNodeID)
	if err != nil {
		return nil, err
	}

	for _, peer := range neighbors {
		peerCost, err := r.CostOfAction(ctx.TaskType, peer)
		if err != nil {
			continue
		}
		savings := myCost.EnergyCost - peerCost.EnergyCost
		if savings > bestSavings {
			bestSavings = savings
			bestPeer = peer
			bestPath = peerCost.GraphPathUsed
		}
	}

	if bestPeer == "" || bestSavings <= 0 {
		return nil, contracts.ErrInsufficientTrust
	}

	return &types.PeerRecommendation{
		PeerID:          bestPeer,
		ExpectedSavings: bestSavings,
		Rationale:       fmt.Sprintf("peer=%s saves %.3f energy vs local execution; path=[%s]", bestPeer, bestSavings, strings.Join(bestPath, ", ")),
		GraphPathUsed:   bestPath,
	}, nil
}

func (r *RuleEngineReasoner) SimulateOutcome(ctx *types.OffloadContext, targetNodeID string) (*types.OutcomeSimulation, error) {
	cost, err := r.CostOfAction(ctx.TaskType, targetNodeID)
	if err != nil {
		return nil, err
	}

	var riskFlags []string
	if ctx.LatencyBudgetMs > 0 && cost.LatencyEstimate > ctx.LatencyBudgetMs {
		riskFlags = append(riskFlags, fmt.Sprintf("latency %.1fms exceeds budget %.1fms", cost.LatencyEstimate, ctx.LatencyBudgetMs))
	}
	if ctx.EnergyBudgetJoules != nil && cost.EnergyCost > *ctx.EnergyBudgetJoules {
		riskFlags = append(riskFlags, fmt.Sprintf("energy %.3fJ exceeds budget %.3fJ", cost.EnergyCost, *ctx.EnergyBudgetJoules))
	}

	return &types.OutcomeSimulation{
		ExpectedLatency: cost.LatencyEstimate,
		ExpectedEnergy:  cost.EnergyCost,
		Confidence:      cost.Confidence,
		GraphPathUsed:   cost.GraphPathUsed,
		RiskFlags:       riskFlags,
		// P95 estimates require Gaussian descriptors (edge-standard+); nil here.
	}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func blend(e *types.EdgeDescriptor) float64 {
	return (1-e.Confidence)*e.PriorWeight + e.Confidence*e.EMAWeight
}

func sign(d types.Direction) float64 {
	if d == types.Positive {
		return 1.0
	}
	return -1.0
}
