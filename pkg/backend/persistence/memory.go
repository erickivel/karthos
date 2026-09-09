package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/erickivel/karthos/pkg/backend/query"
	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/core/semconv"
	"github.com/erickivel/karthos/pkg/core/topology"
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
	s.samples = append(s.samples, cloneMetricSamples(samples)...)
	return nil
}

// Query filters samples by EntityIDs (if non-empty), edge source/target IDs
// carried in labels (if non-empty), MetricName (if non-empty), and time range
// (if non-zero). Returns a copy of the matching samples.
func (s *InMemoryMetricStore) Query(_ context.Context, q query.MetricQuery) ([]metrics.MetricSample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if q.Step > 0 {
		if q.Aggregation == "" || q.Aggregation == metrics.AggregationLatest {
			return evaluateMemoryLatestTrend(s.filteredSamples(q, false), q), nil
		}
		return s.evaluateAggregatedTrend(q), nil
	}

	if q.Aggregation != "" && q.Aggregation != metrics.AggregationLatest {
		if q.Aggregation == metrics.AggregationWindowMean {
			return s.queryWindowMean(q), nil
		}
		return aggregateMemorySamples(s.filteredSamples(q, false), q), nil
	}

	return cloneMetricSamples(s.filteredSamples(q, true)), nil
}

func (s *InMemoryMetricStore) evaluateAggregatedTrend(q query.MetricQuery) []metrics.MetricSample {
	result := make([]metrics.MetricSample, 0)
	for _, point := range (query.ResolvedMetricQuery{MetricQuery: q}).EvaluationPoints() {
		at := q
		at.From = time.Time{}
		at.To = point
		at.Step = 0
		var samples []metrics.MetricSample
		if at.Aggregation == metrics.AggregationWindowMean {
			samples = s.queryWindowMean(at)
		} else {
			samples = aggregateMemorySamples(s.filteredSamples(at, false), at)
		}
		result = append(result, samples...)
	}
	return result
}

// evaluateMemoryLatestTrend projects irregular raw observations onto the
// resolved query grid without treating held values as new source samples.
func evaluateMemoryLatestTrend(samples []metrics.MetricSample, q query.MetricQuery) []metrics.MetricSample {
	groups := map[string][]metrics.MetricSample{}
	for _, sample := range samples {
		groups[memorySeriesKey(sample)] = append(groups[memorySeriesKey(sample)], sample)
	}
	result := make([]metrics.MetricSample, 0, len(samples))
	for _, series := range groups {
		sort.Slice(series, func(i, j int) bool { return series[i].Timestamp.Before(series[j].Timestamp) })
		latest := -1
		for _, point := range (query.ResolvedMetricQuery{MetricQuery: q}).EvaluationPoints() {
			for latest+1 < len(series) && !series[latest+1].Timestamp.After(point) {
				latest++
			}
			if latest < 0 {
				continue
			}
			if q.MaxSampleAge > 0 && point.Sub(series[latest].Timestamp) > q.MaxSampleAge {
				continue
			}
			sample := cloneMetricSample(series[latest])
			sample.Timestamp = point
			result = append(result, sample)
		}
	}
	return result
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
		groups[memorySeriesKey(sample)] = append(groups[memorySeriesKey(sample)], sample)
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
	if q.MaxSampleAge > 0 && end.Sub(current[len(current)-1].Timestamp) > q.MaxSampleAge {
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
			grouped[memorySeriesKey(sample)] = append(grouped[memorySeriesKey(sample)], sample)
		}
		result := make(map[string]metrics.MetricSample, len(grouped))
		for _, series := range grouped {
			sample, ok := aggregateMemorySeries(series, query.MetricQuery{
				Aggregation:  metrics.AggregationIncrease,
				Window:       q.Window,
				To:           q.To,
				Unit:         q.Unit,
				MaxSampleAge: q.MaxSampleAge,
			})
			if !ok {
				continue
			}
			sample.MetricName = q.MetricName
			result[memorySeriesKey(sample)] = sample
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

func memorySeriesKey(sample metrics.MetricSample) string {
	labels := make(map[string]string, len(sample.Labels))
	for key, value := range sample.Labels {
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
	mu                    sync.RWMutex
	structuralSnapshots   []topology.Snapshot
	materializedSnapshots []topology.Snapshot
	nextMaterializedID    int64
	edges                 map[string]topology.Edge // observed edges keyed by edge ID
	nodeTTL               time.Duration            // 0 = no structural freshness guard
	nowFn                 func() time.Time
}

const (
	graphSnapshotRetention          = 24 * time.Hour
	graphObservationFreshnessWindow = 5 * time.Minute
)

// InMemoryGraphStoreOption configures an InMemoryGraphStore.
type InMemoryGraphStoreOption func(*InMemoryGraphStore)

// WithInMemoryNodeTTL refuses to materialize from an expired structural
// projection. Retained materialized snapshots remain queryable. A zero ttl
// disables the structural freshness guard.
func WithInMemoryNodeTTL(ttl time.Duration) InMemoryGraphStoreOption {
	return func(s *InMemoryGraphStore) { s.nodeTTL = ttl }
}

// NewInMemoryGraphStore returns a new empty InMemoryGraphStore.
func NewInMemoryGraphStore(opts ...InMemoryGraphStoreOption) *InMemoryGraphStore {
	s := &InMemoryGraphStore{
		edges: make(map[string]topology.Edge),
		nowFn: func() time.Time { return time.Now().UTC() },
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

	s.structuralSnapshots = append(s.structuralSnapshots, cloneSnapshot(snapshot))
	return nil
}

// PutObservedEdges merges telemetry-derived and lifecycle observations into
// current edge state. It never changes structural or materialized history.
func (s *InMemoryGraphStore) PutObservedEdges(_ context.Context, edges []topology.Edge) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range edges {
		e = cloneEdge(e)
		existing, ok := s.edges[e.ID]
		if !ok {
			s.edges[e.ID] = cloneEdge(e)
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
		s.edges[e.ID] = MergeObservedEdge(existing, e)
	}

	return nil
}

// Materialize stores one complete graph projection. Kubernetes structure is
// authoritative for membership; only recent active observed edges whose
// endpoints remain in that structure are included.
func (s *InMemoryGraphStore) Materialize(_ context.Context, at time.Time) (*topology.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.structuralSnapshots) == 0 {
		return nil, nil
	}
	latest := s.structuralSnapshots[len(s.structuralSnapshots)-1]
	if s.nodeTTL > 0 && !latest.CreatedAt.IsZero() && latest.CreatedAt.Before(at.Add(-s.nodeTTL)) {
		return nil, nil
	}

	result := cloneSnapshot(latest)
	nodesByID := make(map[string]topology.Node, len(result.Graph.Nodes))
	for _, node := range result.Graph.Nodes {
		nodesByID[string(node.ID)] = cloneNode(node)
	}
	edgesByID := make(map[string]topology.Edge, len(result.Graph.Edges)+len(s.edges))
	for _, edge := range result.Graph.Edges {
		edgesByID[edge.ID] = cloneEdge(edge)
	}
	cutoff := at.Add(-graphObservationFreshnessWindow)
	for _, edge := range s.edges {
		if edge.Relation != topology.RelRunsOn && edge.Relation != topology.RelCalls {
			continue
		}
		if edgeRetired(edge) || edgeTerminalDeadlineExpired(edge, at) || edge.LastSeen.Before(cutoff) {
			continue
		}
		if _, ok := nodesByID[string(edge.Source)]; !ok {
			continue
		}
		if _, ok := nodesByID[string(edge.Target)]; !ok {
			continue
		}
		edgesByID[edge.ID] = cloneEdge(edge)
	}

	result.Graph.Nodes = make([]topology.Node, 0, len(nodesByID))
	for _, node := range nodesByID {
		result.Graph.Nodes = append(result.Graph.Nodes, node)
	}
	sort.Slice(result.Graph.Nodes, func(i, j int) bool {
		return result.Graph.Nodes[i].ID < result.Graph.Nodes[j].ID
	})
	result.Graph.Edges = make([]topology.Edge, 0, len(edgesByID))
	for _, edge := range edgesByID {
		result.Graph.Edges = append(result.Graph.Edges, edge)
	}
	sort.Slice(result.Graph.Edges, func(i, j int) bool {
		return result.Graph.Edges[i].ID < result.Graph.Edges[j].ID
	})
	result.ID = s.nextMaterializedID + 1
	s.nextMaterializedID = result.ID
	result.CreatedAt = at.UTC()
	result.From = result.CreatedAt
	result.To = result.CreatedAt
	result.Graph.Timestamp = result.CreatedAt
	s.materializedSnapshots = append(s.materializedSnapshots, cloneSnapshot(result))

	retentionCutoff := result.CreatedAt.Add(-graphSnapshotRetention)
	kept := s.materializedSnapshots[:0]
	for _, snapshot := range s.materializedSnapshots {
		if !snapshot.CreatedAt.Before(retentionCutoff) {
			kept = append(kept, snapshot)
		}
	}
	s.materializedSnapshots = kept
	return &result, nil
}

// GetLatest returns the newest complete materialized graph snapshot.
func (s *InMemoryGraphStore) GetLatest(_ context.Context) (*topology.Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.materializedSnapshots) == 0 {
		return nil, nil
	}
	latest := cloneSnapshot(s.materializedSnapshots[len(s.materializedSnapshots)-1])
	return &latest, nil
}

// GetAt returns the newest retained materialized snapshot at or before at.
func (s *InMemoryGraphStore) GetAt(_ context.Context, at time.Time) (*topology.Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for index := len(s.materializedSnapshots) - 1; index >= 0; index-- {
		snapshot := s.materializedSnapshots[index]
		if !snapshot.CreatedAt.After(at) {
			cloned := cloneSnapshot(snapshot)
			return &cloned, nil
		}
	}
	return nil, nil
}

// RetireRunsOn records the terminal lifecycle timestamp without changing the
// edge's observations. Retired runs_on edges are excluded from the current
// composed graph while their lifecycle state remains available to storage
// reconciliation.
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
