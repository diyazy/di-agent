package minimal

import (
	"fmt"
	"math"
	"sync"

	"github.com/DiyazY/di-agent/pkg/types"
)

// EMAUpdater is the edge-minimal UpdaterContract implementation.
//
// It maintains an Exponential Moving Average per edge:
//
//	new_ema = alpha * observation + (1 - alpha) * old_ema
//
// alpha controls memory speed: high alpha reacts fast (good for volatile
// environments), low alpha is stable (good for noisy sensors).
//
// Confidence grows as: min(n_observations / convergenceThreshold, 1.0)
// The agent blends prior and EMA as:
//
//	effective = (1 - confidence) * prior + confidence * ema
type EMAUpdater struct {
	storage             storageWriter
	alpha               float64
	convergenceThreshold float64

	mu   sync.Mutex
	seen map[string]struct{} // processed event IDs (idempotency)
}

type storageWriter interface {
	GetEdge(fromID, toID string) (*types.EdgeDescriptor, error)
	PutEdge(d *types.EdgeDescriptor) error
	GetNode(nodeID string) (*types.NodeDescriptor, error)
	PutNode(d *types.NodeDescriptor) error
}

// NewEMAUpdater creates an EMAUpdater.
//   - alpha: EMA decay factor, typically 0.1–0.3 for edge environments.
//   - convergenceThreshold: number of observations at which confidence reaches 1.0.
func NewEMAUpdater(storage storageWriter, alpha, convergenceThreshold float64) *EMAUpdater {
	return &EMAUpdater{
		storage:             storage,
		alpha:               alpha,
		convergenceThreshold: convergenceThreshold,
		seen:                make(map[string]struct{}),
	}
}

func (u *EMAUpdater) UpdateEdge(fromID, toID string, obs float64, eventID string) (*types.EdgeDescriptor, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	key := fmt.Sprintf("%s→%s:%s", fromID, toID, eventID)
	if _, ok := u.seen[key]; ok {
		return u.storage.GetEdge(fromID, toID) // idempotent: return unchanged state
	}

	edge, err := u.storage.GetEdge(fromID, toID)
	if err != nil {
		return nil, err
	}
	if edge == nil {
		return nil, fmt.Errorf("edge %s→%s not found in storage", fromID, toID)
	}

	updated := *edge
	updated.EMAWeight = u.alpha*obs + (1-u.alpha)*edge.EMAWeight
	updated.NObservations = edge.NObservations + 1
	updated.Confidence = math.Min(float64(updated.NObservations)/u.convergenceThreshold, 1.0)

	if err := u.storage.PutEdge(&updated); err != nil {
		return nil, err
	}
	u.seen[key] = struct{}{}
	return &updated, nil
}

func (u *EMAUpdater) UpdateNode(nodeID string, obs float64, eventID string) (*types.NodeDescriptor, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	key := fmt.Sprintf("node:%s:%s", nodeID, eventID)
	if _, ok := u.seen[key]; ok {
		return u.storage.GetNode(nodeID)
	}

	node, err := u.storage.GetNode(nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("node %s not found in storage", nodeID)
	}

	updated := *node
	updated.EMAValue = u.alpha*obs + (1-u.alpha)*node.EMAValue
	updated.NObservations = node.NObservations + 1
	updated.Confidence = math.Min(float64(updated.NObservations)/u.convergenceThreshold, 1.0)

	if err := u.storage.PutNode(&updated); err != nil {
		return nil, err
	}
	u.seen[key] = struct{}{}
	return &updated, nil
}

func (u *EMAUpdater) Reset(fromID, toID string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	edge, err := u.storage.GetEdge(fromID, toID)
	if err != nil {
		return err
	}
	if edge == nil {
		return fmt.Errorf("edge %s→%s not found", fromID, toID)
	}

	reset := *edge
	reset.EMAWeight = edge.PriorWeight
	reset.NObservations = 0
	reset.Confidence = 0.0

	// Clear seen entries for this edge so future events are processed normally.
	prefix := fmt.Sprintf("%s→%s:", fromID, toID)
	for k := range u.seen {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(u.seen, k)
		}
	}

	return u.storage.PutEdge(&reset)
}
