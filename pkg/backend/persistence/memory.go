package persistence

import (
	"context"
	"sync"
	"time"

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
	// Mirror VMMetricStore: populate the public string label for health-status
	// enum metrics so consumers (incl. tests) see the same shape regardless of
	// backing store. Storage writes hold integer values only; the label is an
	// API/store-boundary derivation.
	if metrics.IsHealthStatusMetric(q.MetricName) {
		for i := range result {
			result[i].ValueLabel = metrics.HealthLabel(q.MetricName, result[i].Value)
		}
	}
	return result, nil
}

// InMemoryGraphStore is a thread-safe in-memory implementation of GraphStore.
type InMemoryGraphStore struct {
	mu       sync.RWMutex
	snapshot *topology.Snapshot
	edges    map[string]topology.Edge // keyed by edge ID
	nodes    map[string]topology.Node // keyed by entity ID string
	lastSeen map[string]time.Time     // node entity ID -> most recent PutSnapshot time
	nodeTTL  time.Duration            // 0 = no TTL
	nowFn    func() time.Time
}

// InMemoryGraphStoreOption configures an InMemoryGraphStore.
type InMemoryGraphStoreOption func(*InMemoryGraphStore)

// WithInMemoryNodeTTL drops nodes whose most recent sighting is older than ttl
// when reading via GetLatest. 0 disables the TTL.
func WithInMemoryNodeTTL(ttl time.Duration) InMemoryGraphStoreOption {
	return func(s *InMemoryGraphStore) { s.nodeTTL = ttl }
}

// NewInMemoryGraphStore returns a new empty InMemoryGraphStore.
func NewInMemoryGraphStore(opts ...InMemoryGraphStoreOption) *InMemoryGraphStore {
	s := &InMemoryGraphStore{
		edges:    make(map[string]topology.Edge),
		nodes:    make(map[string]topology.Node),
		lastSeen: make(map[string]time.Time),
		nowFn:    func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
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

	now := s.nowFn()
	for _, n := range snapshot.Graph.Nodes {
		s.nodes[string(n.ID)] = n
		s.lastSeen[string(n.ID)] = now
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

	var cutoff time.Time
	if s.nodeTTL > 0 {
		cutoff = s.nowFn().Add(-s.nodeTTL)
	}
	nodes := make([]topology.Node, 0, len(s.nodes))
	for id, n := range s.nodes {
		if s.nodeTTL > 0 {
			if ts, ok := s.lastSeen[id]; !ok || ts.Before(cutoff) {
				continue
			}
		}
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
