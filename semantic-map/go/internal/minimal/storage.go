package minimal

import (
	"fmt"
	"sort"
	"sync"

	"github.com/DiyazY/di-agent/pkg/types"
)

// InMemoryStorage is the edge-minimal StorageContract implementation.
// All state lives in process memory; data is lost on restart.
// For the edge-standard profile, replace with SQLiteStorage — same interface.
//
// The edge backbone is a multigraph: Di-Select has three "conflict-pair"
// propositions (P2/P3 on RC→PS, P5/P6 on CO→RR, P7/P9 on CE→MU) where two
// propositions share the same (from, to) endpoints with opposite directions.
// Storage keys edges by (fromID, toID, propositionID) so each proposition
// gets its own EdgeDescriptor and tracks an independent EMA. GetEdge(from, to)
// returns a deterministic pick — the entry with the lexicographically smallest
// PropositionID — for the simple-graph case; multigraph callers use
// GetEdgesByPair to retrieve every edge between two constructs.
type InMemoryStorage struct {
	mu    sync.RWMutex
	nodes map[string]*types.NodeDescriptor
	edges map[string]*types.EdgeDescriptor // key: "fromID→toID:propositionID"
}

func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		nodes: make(map[string]*types.NodeDescriptor),
		edges: make(map[string]*types.EdgeDescriptor),
	}
}

func (s *InMemoryStorage) GetNode(nodeID string) (*types.NodeDescriptor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d := s.nodes[nodeID]
	if d == nil {
		return nil, nil
	}
	cp := *d
	return &cp, nil
}

func (s *InMemoryStorage) PutNode(d *types.NodeDescriptor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *d
	s.nodes[d.NodeID] = &cp
	return nil
}

// GetEdge returns the edge between fromID and toID with the lexicographically
// smallest PropositionID — a deterministic choice when multiple propositions
// share the same endpoints. Callers that need every edge between a pair (e.g.
// the Updater, when one observation must update both halves of a conflict
// pair) should use GetEdgesByPair instead.
func (s *InMemoryStorage) GetEdge(fromID, toID string) (*types.EdgeDescriptor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var best *types.EdgeDescriptor
	for _, e := range s.edges {
		if e.FromID != fromID || e.ToID != toID {
			continue
		}
		if best == nil || e.PropositionID < best.PropositionID {
			best = e
		}
	}
	if best == nil {
		return nil, nil
	}
	cp := *best
	return &cp, nil
}

// GetEdgesByPair returns every edge between (fromID, toID). For conflict-pair
// propositions this returns multiple descriptors (e.g. P2 and P3 both span
// RC→PS). Order is sorted by PropositionID so callers get a stable iteration.
func (s *InMemoryStorage) GetEdgesByPair(fromID, toID string) ([]*types.EdgeDescriptor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*types.EdgeDescriptor
	for _, e := range s.edges {
		if e.FromID == fromID && e.ToID == toID {
			cp := *e
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PropositionID < out[j].PropositionID })
	return out, nil
}

// PutEdge stores an edge keyed by (fromID, toID, propositionID). Two edges
// with the same endpoints but distinct PropositionIDs coexist; storing twice
// with the same PropositionID overwrites in place.
func (s *InMemoryStorage) PutEdge(d *types.EdgeDescriptor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *d
	s.edges[edgeKey(d.FromID, d.ToID, d.PropositionID)] = &cp
	return nil
}

func (s *InMemoryStorage) Neighbors(nodeID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := make(map[string]bool)
	var out []string
	for _, e := range s.edges {
		if e.FromID == nodeID && !seen[e.ToID] {
			seen[e.ToID] = true
			out = append(out, e.ToID)
		}
	}
	return out, nil
}

func (s *InMemoryStorage) AllEdges() ([]*types.EdgeDescriptor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*types.EdgeDescriptor, 0, len(s.edges))
	for _, e := range s.edges {
		cp := *e
		out = append(out, &cp)
	}
	return out, nil
}

func edgeKey(from, to, propID string) string {
	return fmt.Sprintf("%s→%s:%s", from, to, propID)
}
