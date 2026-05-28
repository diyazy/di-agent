package minimal

import (
	"fmt"
	"sync"

	"github.com/DiyazY/di-agent/pkg/types"
)

// InMemoryStorage is the edge-minimal StorageContract implementation.
// All state lives in process memory; data is lost on restart.
// For the edge-standard profile, replace with SQLiteStorage — same interface.
type InMemoryStorage struct {
	mu    sync.RWMutex
	nodes map[string]*types.NodeDescriptor
	edges map[string]*types.EdgeDescriptor // key: "fromID→toID"
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

func (s *InMemoryStorage) GetEdge(fromID, toID string) (*types.EdgeDescriptor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d := s.edges[edgeKey(fromID, toID)]
	if d == nil {
		return nil, nil
	}
	cp := *d
	return &cp, nil
}

func (s *InMemoryStorage) PutEdge(d *types.EdgeDescriptor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *d
	s.edges[edgeKey(d.FromID, d.ToID)] = &cp
	return nil
}

func (s *InMemoryStorage) Neighbors(nodeID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	prefix := nodeID + "→"
	for k, e := range s.edges {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
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

func edgeKey(from, to string) string {
	return fmt.Sprintf("%s→%s", from, to)
}
