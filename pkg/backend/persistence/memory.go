package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/core/topology"
)

// InMemoryMetricStore is a thread-safe in-memory implementation of MetricStore.
type InMemoryMetricStore struct {
	mu      sync.RWMutex
	samples []metrics.MetricSample
	nowFn   func() time.Time
}

// NewInMemoryMetricStore returns a new empty InMemoryMetricStore.
func NewInMemoryMetricStore() *InMemoryMetricStore {
	return &InMemoryMetricStore{nowFn: func() time.Time { return time.Now().UTC() }}
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

	if q.Aggregation != "" && q.Aggregation != metrics.AggregationLatest {
		if q.Aggregation == metrics.AggregationWindowMean {
			return s.queryWindowMean(q), nil
		}
		return aggregateMemorySamples(s.filteredSamples(q, false), q), nil
	}

	result := s.filteredSamples(q, true)
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

func (s *InMemoryMetricStore) filteredSamples(q query.MetricQuery, applyFrom bool) []metrics.MetricSample {
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
		if applyFrom && !q.From.IsZero() && sample.Timestamp.Before(q.From) {
			continue
		}
		if !q.To.IsZero() && sample.Timestamp.After(q.To) {
			continue
		}
		result = append(result, sample)
	}
	return result
}

func aggregateMemorySamples(samples []metrics.MetricSample, q query.MetricQuery) []metrics.MetricSample {
	groups := map[string][]metrics.MetricSample{}
	for _, sample := range samples {
		groups[memorySeriesKey(sample, false)] = append(groups[memorySeriesKey(sample, false)], sample)
	}
	var result []metrics.MetricSample
	for _, series := range groups {
		if sample, ok := aggregateMemorySeries(series, q); ok {
			result = append(result, sample)
		}
	}
	return result
}

func aggregateMemorySeries(series []metrics.MetricSample, q query.MetricQuery) (metrics.MetricSample, bool) {
	sort.Slice(series, func(i, j int) bool { return series[i].Timestamp.Before(series[j].Timestamp) })
	end := q.To
	if end.IsZero() {
		end = series[len(series)-1].Timestamp
	}
	start := end.Add(-q.Window)
	delta := series[0].Labels["temporality"] == "delta"
	var current []metrics.MetricSample
	for _, sample := range series {
		if !sample.Timestamp.Before(start) && !sample.Timestamp.After(end) {
			current = append(current, sample)
		}
	}
	if len(current) == 0 || (delta && q.Aggregation == metrics.AggregationActiveRate) {
		return metrics.MetricSample{}, false
	}
	out := current[len(current)-1]
	out.Timestamp, out.Unit = end, q.Unit
	if delta {
		out.Value = 0
		for _, sample := range current {
			out.Value += sample.Value
		}
	} else if q.Aggregation == metrics.AggregationActiveRate {
		if len(current) < 2 {
			return metrics.MetricSample{}, false
		}
		elapsed := current[len(current)-1].Timestamp.Sub(current[0].Timestamp).Seconds()
		if elapsed <= 0 {
			return metrics.MetricSample{}, false
		}
		out.Value = resetAwareIncrease(current) / elapsed
	} else {
		historyStart := end.Add(-2 * q.Window)
		baseline := -1
		historyFirst := -1
		historyLast := -1
		for i := range series {
			if series[i].Timestamp.Before(historyStart) || series[i].Timestamp.After(end) {
				continue
			}
			if historyFirst < 0 {
				historyFirst = i
			}
			historyLast = i
			if !series[i].Timestamp.After(start) {
				baseline = i
			}
		}
		if baseline < 0 || len(current) < 2 || historyFirst < 0 || historyLast < 0 ||
			series[historyLast].Timestamp.Sub(series[historyFirst].Timestamp) < q.Window {
			return metrics.MetricSample{}, false
		}
		windowSeries := append([]metrics.MetricSample(nil), series[baseline])
		for _, sample := range series[baseline+1:] {
			if !sample.Timestamp.After(end) {
				windowSeries = append(windowSeries, sample)
			}
		}
		out.Value = resetAwareIncrease(windowSeries)
	}
	if q.Aggregation == metrics.AggregationWindowRate {
		out.Value /= q.Window.Seconds()
	}
	return out, true
}

func resetAwareIncrease(series []metrics.MetricSample) float64 {
	var total float64
	for i := 1; i < len(series); i++ {
		if series[i].Value >= series[i-1].Value {
			total += series[i].Value - series[i-1].Value
		} else {
			total += series[i].Value
		}
	}
	return total
}

func (s *InMemoryMetricStore) queryWindowMean(q query.MetricQuery) []metrics.MetricSample {
	branch := func(suffix string) map[string]metrics.MetricSample {
		branchQuery := q
		branchQuery.MetricName = q.MetricName + suffix
		branchQuery.Aggregation = metrics.AggregationIncrease
		grouped := map[string][]metrics.MetricSample{}
		for _, sample := range s.filteredSamples(branchQuery, false) {
			grouped[memorySeriesKey(sample, false)] = append(grouped[memorySeriesKey(sample, false)], sample)
		}
		result := make(map[string]metrics.MetricSample, len(grouped))
		for _, series := range grouped {
			sample, ok := aggregateMemorySeries(series, query.MetricQuery{
				Aggregation: metrics.AggregationIncrease,
				Window:      q.Window,
				To:          q.To,
				Unit:        q.Unit,
			})
			if !ok {
				continue
			}
			sample.MetricName = q.MetricName
			result[memorySeriesKey(sample, false)] = sample
		}
		return result
	}

	sums := branch("_sum")
	counts := branch("_count")
	result := make([]metrics.MetricSample, 0, len(sums))
	for key, sum := range sums {
		count, ok := counts[key]
		if !ok || count.Value <= 0 {
			continue
		}
		sum.Value = sum.Value / count.Value
		sum.MetricName = q.MetricName
		sum.Unit = q.Unit
		result = append(result, sum)
	}
	return result
}

func memorySeriesKey(sample metrics.MetricSample, excludeFailed bool) string {
	labels := make(map[string]string, len(sample.Labels))
	for key, value := range sample.Labels {
		if excludeFailed && key == "failed" {
			continue
		}
		labels[key] = value
	}
	encoded, _ := json.Marshal(labels)
	return sample.MetricName + "|" + string(sample.EntityID) + "|" + string(encoded)
}

// QueryCatalogStats aggregates per-(metric, source) sample counts and
// last-seen timestamps across samples that fall inside [now-lookback, now].
func (s *InMemoryMetricStore) QueryCatalogStats(_ context.Context, lookback time.Duration) ([]CatalogStat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := s.nowFn()
	cutoff := now.Add(-lookback)

	type key struct {
		metric string
		source string
	}
	type agg struct {
		count    int64
		lastSeen time.Time
	}
	buckets := map[key]*agg{}

	for _, sample := range s.samples {
		if sample.Timestamp.Before(cutoff) || sample.Timestamp.After(now) {
			continue
		}
		k := key{metric: sample.MetricName, source: sample.Labels[semconv.LabelSource]}
		b, ok := buckets[k]
		if !ok {
			b = &agg{}
			buckets[k] = b
		}
		b.count++
		if sample.Timestamp.After(b.lastSeen) {
			b.lastSeen = sample.Timestamp
		}
	}

	// Group by metric name. We don't sort here — the API layer joins against
	// the catalog and decides the surfaced ordering.
	grouped := map[string][]SourceStat{}
	for k, b := range buckets {
		grouped[k.metric] = append(grouped[k.metric], SourceStat{
			SourceID:    k.source,
			SampleCount: b.count,
			LastSeenAt:  b.lastSeen,
		})
	}

	result := make([]CatalogStat, 0, len(grouped))
	for name, sources := range grouped {
		result = append(result, CatalogStat{MetricName: name, BySource: sources})
	}
	return result, nil
}

// InMemoryGraphStore is a thread-safe in-memory implementation of GraphStore.
type InMemoryGraphStore struct {
	mu                  sync.RWMutex
	structuralSnapshots []topology.Snapshot
	observedSnapshot    *topology.Snapshot
	edges               map[string]topology.Edge // observed edges keyed by edge ID
	nodes               map[string]topology.Node // observed nodes keyed by entity ID string
	lastSeen            map[string]time.Time     // observed node entity ID -> most recent observation time
	nodeTTL             time.Duration            // 0 = no TTL
	nowFn               func() time.Time
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

// PutStructuralSnapshot appends one complete Kubernetes projection. Earlier
// projections remain available as history while GetLatest uses only the newest.
func (s *InMemoryGraphStore) PutStructuralSnapshot(_ context.Context, snapshot topology.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.structuralSnapshots = append(s.structuralSnapshots, snapshot)
	return nil
}

// PutObservedSnapshot merges telemetry-derived and lifecycle observations into
// current state. It never changes structural snapshot history.
func (s *InMemoryGraphStore) PutObservedSnapshot(_ context.Context, snapshot topology.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.observedSnapshot = &snapshot

	for _, e := range snapshot.Graph.Edges {
		existing, ok := s.edges[e.ID]
		if !ok {
			s.edges[e.ID] = e
			continue
		}
		if edgeRetired(existing) || edgeTerminalDeadline(existing) != nil {
			if e.FirstSeen.Before(existing.FirstSeen) {
				existing.FirstSeen = e.FirstSeen
			}
			if !edgeRetired(existing) && e.LastSeen.After(existing.LastSeen) {
				existing.LastSeen = e.LastSeen
			}
			if edgeRetired(existing) && e.LastSeen.Before(existing.LastSeen) {
				existing.LastSeen = e.LastSeen
			}
			existing.Metadata = mergeRetiredEdgeMetadata(existing.Metadata, e.Metadata)
			existing.Confidence = e.Confidence
			existing.Provenance = mergeRetiredEdgeProvenance(existing.Provenance, e.Provenance)
			s.edges[e.ID] = existing
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

// GetLatest returns the newest complete structural projection composed with
// current telemetry/lifecycle observations. Returns nil, nil before any write.
func (s *InMemoryGraphStore) GetLatest(_ context.Context) (*topology.Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.structuralSnapshots) == 0 && s.observedSnapshot == nil {
		return nil, nil
	}

	var result topology.Snapshot
	if len(s.structuralSnapshots) > 0 {
		result = s.structuralSnapshots[len(s.structuralSnapshots)-1]
	} else {
		result = *s.observedSnapshot
	}

	edgesByID := make(map[string]topology.Edge, len(result.Graph.Edges)+len(s.edges))
	for _, e := range s.edges {
		if edgeRetired(e) || edgeTerminalDeadlineExpired(e, s.nowFn()) {
			continue
		}
		edgesByID[e.ID] = e
	}
	if len(s.structuralSnapshots) > 0 {
		for _, edge := range result.Graph.Edges {
			edgesByID[edge.ID] = edge
		}
	}
	edges := make([]topology.Edge, 0, len(edgesByID))
	for _, edge := range edgesByID {
		edges = append(edges, edge)
	}

	var cutoff time.Time
	if s.nodeTTL > 0 {
		cutoff = s.nowFn().Add(-s.nodeTTL)
	}
	nodesByID := make(map[string]topology.Node, len(result.Graph.Nodes)+len(s.nodes))
	for id, n := range s.nodes {
		if s.nodeTTL > 0 {
			if ts, ok := s.lastSeen[id]; !ok || ts.Before(cutoff) {
				continue
			}
		}
		nodesByID[id] = n
	}
	if len(s.structuralSnapshots) > 0 {
		for _, node := range result.Graph.Nodes {
			nodesByID[string(node.ID)] = node
		}
	}
	nodes := make([]topology.Node, 0, len(nodesByID))
	for _, node := range nodesByID {
		nodes = append(nodes, node)
	}

	result.Graph.Edges = edges
	result.Graph.Nodes = nodes
	return &result, nil
}

// RetireRunsOn records the terminal lifecycle timestamp without changing the
// edge's observations. Retired runs_on edges are excluded from GetLatest but
// remain queryable through QueryEdges for audit use.
func (s *InMemoryGraphStore) RetireRunsOn(_ context.Context, podID, nodeID entities.EntityID, podUID string, retiredAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retireRunsOnLocked(podID, nodeID, podUID, retiredAt)
	return nil
}

// ReconcileRunsOn retires persisted active assignments absent from the
// authoritative Pod List/Watch state observed at backend startup.
func (s *InMemoryGraphStore) MarkRunsOnTerminal(_ context.Context, grace RunsOnTerminalGrace) (RunsOnTerminalState, error) {
	if grace.ClusterName == "" || grace.PodUID == "" || grace.Deadline.IsZero() {
		return RunsOnTerminalPending, fmt.Errorf("terminal runs_on grace requires cluster, pod UID, and deadline")
	}
	edgeID := topology.ComputeEdgeIDWithDiscriminator(grace.PodID, grace.NodeID, topology.RelRunsOn, grace.PodUID)
	s.mu.Lock()
	defer s.mu.Unlock()
	edge, ok := s.edges[edgeID]
	if !ok {
		edge = topology.NewEdge(grace.PodID, grace.NodeID, topology.RelRunsOn, grace.ObservedAt)
		edge.ID = edgeID
	}
	if edgeRetired(edge) {
		return RunsOnTerminalRetired, nil
	}
	markerMetadata := map[string]string{
		"k8s.cluster.name":   grace.ClusterName,
		"k8s.namespace.name": grace.Namespace,
		"k8s.pod.name":       grace.PodName,
		"k8s.pod.uid":        grace.PodUID,
		"k8s.node.name":      grace.NodeName,
		"terminal_deadline":  grace.Deadline.UTC().Format(time.RFC3339Nano),
	}
	if grace.Confirmed {
		markerMetadata["terminal_grace_confirmed"] = "true"
	}
	metadata := mergeRetiredEdgeMetadata(edge.Metadata, markerMetadata)
	if existing := edgeTerminalDeadline(edge); existing != nil && existing.Before(grace.Deadline) {
		metadata["terminal_deadline"] = existing.UTC().Format(time.RFC3339Nano)
	}
	edge.Metadata = metadata
	s.edges[edgeID] = edge
	return RunsOnTerminalPending, nil
}

func (s *InMemoryGraphStore) ReconcileRunsOn(_ context.Context, clusterName string, active []RunsOnAssignment, retiredAt time.Time) ([]RunsOnTerminalGrace, error) {
	activeIDs := make(map[string]struct{}, len(active))
	for _, assignment := range active {
		activeIDs[topology.ComputeEdgeIDWithDiscriminator(assignment.PodID, assignment.NodeID, topology.RelRunsOn, assignment.PodUID)] = struct{}{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	pending := make([]RunsOnTerminalGrace, 0)
	for _, edge := range s.edges {
		if edge.Relation != topology.RelRunsOn || edgeRetired(edge) {
			continue
		}
		if edge.Metadata["k8s.cluster.name"] != clusterName {
			continue
		}
		podUID := edge.Metadata["k8s.pod.uid"]
		if podUID == "" {
			continue
		}
		if deadline := edgeTerminalDeadline(edge); deadline != nil {
			if edge.Metadata["terminal_grace_confirmed"] != "true" {
				if _, active := activeIDs[edge.ID]; !active {
					s.retireRunsOnLocked(edge.Source, edge.Target, podUID, retiredAt)
					continue
				}
			}
			if deadline.After(retiredAt) {
				pending = append(pending, RunsOnTerminalGrace{
					ClusterName: clusterName,
					Namespace:   edge.Metadata["k8s.namespace.name"],
					PodName:     edge.Metadata["k8s.pod.name"],
					PodUID:      podUID,
					NodeName:    edge.Metadata["k8s.node.name"],
					PodID:       edge.Source,
					NodeID:      edge.Target,
					Deadline:    *deadline,
					Confirmed:   edge.Metadata["terminal_grace_confirmed"] == "true",
				})
				continue
			}
			s.retireRunsOnLocked(edge.Source, edge.Target, podUID, *deadline)
			continue
		}
		if _, ok := activeIDs[edge.ID]; ok {
			continue
		}
		s.retireRunsOnLocked(edge.Source, edge.Target, podUID, retiredAt)
	}
	return pending, nil
}

func (s *InMemoryGraphStore) retireRunsOnLocked(podID, nodeID entities.EntityID, podUID string, retiredAt time.Time) {
	edgeID := topology.ComputeEdgeIDWithDiscriminator(podID, nodeID, topology.RelRunsOn, podUID)

	edge, ok := s.edges[edgeID]
	if !ok {
		edge = topology.NewEdge(podID, nodeID, topology.RelRunsOn, retiredAt)
		edge.ID = edgeID
		edge.Metadata = map[string]string{"k8s.pod.uid": podUID}
	}
	if edge.Relation != topology.RelRunsOn || edgeRetired(edge) {
		return
	}
	if edge.LastSeen.After(retiredAt) {
		edge.LastSeen = retiredAt
	}
	metadata := make(map[string]string, len(edge.Metadata)+1)
	for key, value := range edge.Metadata {
		metadata[key] = value
	}
	metadata["retired_at"] = retiredAt.UTC().Format(time.RFC3339Nano)
	edge.Metadata = metadata
	s.edges[edgeID] = edge
}

func edgeRetired(edge topology.Edge) bool {
	return edge.Relation == topology.RelRunsOn && edge.Metadata != nil && edge.Metadata["retired_at"] != ""
}

func edgeTerminalDeadline(edge topology.Edge) *time.Time {
	if edge.Relation != topology.RelRunsOn || edge.Metadata == nil || edge.Metadata["terminal_deadline"] == "" {
		return nil
	}
	deadline, err := time.Parse(time.RFC3339Nano, edge.Metadata["terminal_deadline"])
	if err != nil {
		return nil
	}
	return &deadline
}

func edgeTerminalDeadlineExpired(edge topology.Edge, now time.Time) bool {
	deadline := edgeTerminalDeadline(edge)
	return deadline != nil && !deadline.After(now)
}

func mergeRetiredEdgeMetadata(existing, incoming map[string]string) map[string]string {
	merged := make(map[string]string, len(existing)+len(incoming))
	for key, value := range existing {
		merged[key] = value
	}
	for key, value := range incoming {
		merged[key] = value
	}
	return merged
}

func mergeRetiredEdgeProvenance(existing, incoming []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	merged := make([]string, 0, len(existing)+len(incoming))
	for _, source := range append(existing, incoming...) {
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
		merged = append(merged, source)
	}
	return merged
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
