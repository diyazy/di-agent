package semmap

import (
	"github.com/DiyazY/di-agent/pkg/contracts"
	"github.com/DiyazY/di-agent/pkg/types"
)

// metricTypeToConstruct routes one MetricType to its primary construct, per
// ARCHITECTURE.md §5 "MetricType catalogue". Unknown MetricTypes are intentionally
// absent — the Bridge silently ignores them (forward-compat with future types).
var metricTypeToConstruct = map[types.MetricType]string{
	types.CPUUtilization:      "RC",
	types.MemoryUtilization:   "RC",
	types.CPUThrottleRatio:    "RC",
	types.BlockIOUtil:         "RC",
	types.EnergyJoules:        "RC",
	types.PodStartupMs:        "PS",
	types.SchedulingLatencyMs: "PS",
	types.NetworkRxBps:        "CO",
	types.NetworkTxBps:        "CO",
	types.NetworkLossRatio:    "CO",
	types.NetworkLatencyMs:    "CO",
}

// edgeUpdater is the slim subset of UpdaterContract the Bridge depends on.
// Defined separately so tests can supply a lightweight stub that counts calls
// without implementing UpdateNode/Reset.
type edgeUpdater interface {
	UpdateEdge(fromID, toID string, observation float64, eventID string) (*types.EdgeDescriptor, error)
}

// Bridge fans a single MetricSample out to every UpdateEdge call its primary
// construct touches. It is stateless: the routing decision is fully determined
// by the metric type and the ontology's current backbone.
//
// Behavior:
//   - The sample's MetricType is mapped to its primary construct via
//     metricTypeToConstruct. Unknown types return (nil, nil) silently —
//     a no-op for forward compatibility.
//   - ontology.Relationships(construct) returns every proposition touching
//     that construct (incoming OR outgoing). Bridge calls UpdateEdge once per
//     unique (from, to) endpoint pair — the Updater fans out internally to
//     every proposition sharing that pair (e.g. P2 and P3 on RC→PS).
//   - Per-edge errors do not abort the loop; they are returned (first wins)
//     so callers can log without short-circuiting the rest of the sample.
func Bridge(
	sample *types.MetricSample,
	ontology contracts.OntologyContract,
	updater edgeUpdater,
) error {
	if sample == nil {
		return nil
	}
	construct, ok := metricTypeToConstruct[sample.MetricType]
	if !ok {
		// Unknown MetricType — Bridge silently ignores (per §5).
		return nil
	}

	props, err := ontology.Relationships(construct)
	if err != nil {
		return err
	}

	// De-duplicate by (from, to). The multigraph storage holds one
	// EdgeDescriptor per proposition, but UpdateEdge already fans out to
	// every proposition sharing the same endpoint pair — calling it once
	// per unique pair is sufficient and avoids double-counting.
	type pair struct{ from, to string }
	seen := make(map[pair]struct{}, len(props))
	var firstErr error
	for _, p := range props {
		if p == nil || p.Deprecated {
			continue
		}
		pr := pair{p.FromConstruct, p.ToConstruct}
		if _, dup := seen[pr]; dup {
			continue
		}
		seen[pr] = struct{}{}
		if _, err := updater.UpdateEdge(p.FromConstruct, p.ToConstruct, sample.Value, sample.EventID); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			// Log-and-continue: keep processing the remaining edges so a
			// single bad pair does not silence the whole sample.
			continue
		}
	}
	return firstErr
}
