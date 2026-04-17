package persistence

import (
	"context"
	"sync"

	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/topology"
)

// InMemoryMetricStore is a thread-safe in-memory implementation of MetricStore.
type InMemoryMetricStore struct {
	mu      sync.RWMutex
	samples []metrics.MetricSample
}

// NewInMemoryMetricStore returns a new empty InMemoryMetricStore.
func NewInMemoryMetricStore() *InMemoryMetricStore {
	return &InMemoryMetricStore{}
}

// Write appends samples to the store.
func (s *InMemoryMetricStore) Write(_ context.Context, samples []metrics.MetricSample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = append(s.samples, samples...)
	return nil
}

// Query filters samples by EntityIDs (if non-empty), edge source/target IDs
// carried in labels (if non-empty), MetricName (if non-empty), and time range
// (if non-zero). Returns a copy of the matching samples.
func (s *InMemoryMetricStore) Query(_ context.Context, q query.MetricQuery) ([]metrics.MetricSample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build entity ID set for O(1) lookup when filter is non-empty.
	var entitySet map[string]struct{}
	if len(q.EntityIDs) > 0 {
		entitySet = make(map[string]struct{}, len(q.EntityIDs))
		for _, id := range q.EntityIDs {
			entitySet[string(id)] = struct{}{}
		}
	}

	var result []metrics.MetricSample
	for _, sample := range s.samples {
		if entitySet != nil {
			if _, ok := entitySet[string(sample.EntityID)]; !ok {
				continue
			}
		}
		if q.MetricName != "" && sample.MetricName != q.MetricName {
			continue
		}
		if q.EdgeSourceID != "" && sample.Labels["edge_client_id"] != string(q.EdgeSourceID) {
			continue
		}
		if q.EdgeTargetID != "" && sample.Labels["edge_server_id"] != string(q.EdgeTargetID) {
			continue
		}
		if !q.From.IsZero() && sample.Timestamp.Before(q.From) {
			continue
		}
		if !q.To.IsZero() && sample.Timestamp.After(q.To) {
			continue
		}
		result = append(result, sample)
	}
	return result, nil
}

// InMemoryGraphStore is a thread-safe in-memory implementation of GraphStore.
type InMemoryGraphStore struct {
	mu       sync.RWMutex
	snapshot *topology.Snapshot
	edges    map[string]topology.Edge // keyed by edge ID
	nodes    map[string]topology.Node // keyed by entity ID string
}

// NewInMemoryGraphStore returns a new empty InMemoryGraphStore.
func NewInMemoryGraphStore() *InMemoryGraphStore {
	return &InMemoryGraphStore{
		edges: make(map[string]topology.Edge),
		nodes: make(map[string]topology.Node),
	}
}

// PutSnapshot replaces the current snapshot and merges its edges and nodes into
// the accumulated maps. For edges already present, LastSeen is updated if the
// incoming edge's LastSeen is later. New edges and nodes are added to their maps.
func (s *InMemoryGraphStore) PutSnapshot(_ context.Context, snapshot topology.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshot = &snapshot

	for _, e := range snapshot.Graph.Edges {
		existing, ok := s.edges[e.ID]
		if !ok {
			s.edges[e.ID] = e
			continue
		}
		if e.LastSeen.After(existing.LastSeen) {
			existing.LastSeen = e.LastSeen
			s.edges[e.ID] = existing
		}
	}

	for _, n := range snapshot.Graph.Nodes {
		s.nodes[string(n.ID)] = n
	}

	return nil
}

// GetLatest returns the latest snapshot with the current accumulated edges and nodes.
// Returns nil, nil if no snapshot has been put.
func (s *InMemoryGraphStore) GetLatest(_ context.Context) (*topology.Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.snapshot == nil {
		return nil, nil
	}

	edges := make([]topology.Edge, 0, len(s.edges))
	for _, e := range s.edges {
		edges = append(edges, e)
	}

	nodes := make([]topology.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		nodes = append(nodes, n)
	}

	result := *s.snapshot
	result.Graph.Edges = edges
	result.Graph.Nodes = nodes
	return &result, nil
}

// QueryEdges filters edges by Source and/or Target (zero value = no filter).
func (s *InMemoryGraphStore) QueryEdges(_ context.Context, q query.EdgeQuery) ([]topology.Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []topology.Edge
	for _, e := range s.edges {
		if q.Source != "" && e.Source != q.Source {
			continue
		}
		if q.Target != "" && e.Target != q.Target {
			continue
		}
		result = append(result, e)
	}
	return result, nil
}
