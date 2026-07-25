package persistence

import (
	"context"
	"encoding/json"
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
		if q.Aggregation == metrics.AggregationWindowRatio && q.MetricName == "servicegraph.request.error.ratio" {
			return s.queryErrorRatio(q), nil
		}
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

func (s *InMemoryMetricStore) queryErrorRatio(q query.MetricQuery) []metrics.MetricSample {
	requestsQ := q
	requestsQ.MetricName = "servicegraph.request.count"
	requestsQ.Aggregation = metrics.AggregationIncrease
	requestsQ.Unit = metrics.UnitCount
	errorsQ := requestsQ
	errorsQ.MetricName = "servicegraph.request.error.count"
	return completeMemoryRatios(s.filteredSamples(errorsQ, false), s.filteredSamples(requestsQ, false), q)
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

func completeMemoryRatios(errorRaw, requestRaw []metrics.MetricSample, q query.MetricQuery) []metrics.MetricSample {
	aggregate := func(raw []metrics.MetricSample) (map[string]float64, map[string]int, map[string]int, map[string]metrics.MetricSample) {
		totals, observed, mature := map[string]float64{}, map[string]int{}, map[string]int{}
		representatives := map[string]metrics.MetricSample{}
		full := map[string][]metrics.MetricSample{}
		for _, sample := range raw {
			full[memorySeriesKey(sample, false)] = append(full[memorySeriesKey(sample, false)], sample)
		}
		for _, series := range full {
			if !memorySeriesObservedInWindow(series, q) {
				continue
			}
			contextKey := memoryContextKey(series[0])
			observed[contextKey]++
			if value, ok := aggregateMemorySeries(series, query.MetricQuery{Aggregation: metrics.AggregationIncrease, Window: q.Window, To: q.To, Unit: metrics.UnitCount}); ok {
				mature[contextKey]++
				totals[contextKey] += value.Value
				value.Labels = memoryContextLabels(value.Labels)
				representatives[contextKey] = value
			}
		}
		return totals, observed, mature, representatives
	}
	errors, errorObserved, errorMature, reps := aggregate(errorRaw)
	requests, requestObserved, requestMature, _ := aggregate(requestRaw)
	var result []metrics.MetricSample
	for key, denominator := range requests {
		if denominator <= 0 || requestObserved[key] != requestMature[key] || errorObserved[key] == 0 || errorObserved[key] != errorMature[key] {
			continue
		}
		sample := reps[key]
		sample.MetricName, sample.Value, sample.Unit = q.MetricName, errors[key]/denominator, q.Unit
		result = append(result, sample)
	}
	return result
}

func memorySeriesObservedInWindow(series []metrics.MetricSample, q query.MetricQuery) bool {
	if len(series) == 0 {
		return false
	}
	end := q.To
	if end.IsZero() {
		end = series[0].Timestamp
		for _, sample := range series[1:] {
			if sample.Timestamp.After(end) {
				end = sample.Timestamp
			}
		}
	}
	start := end.Add(-q.Window)
	for _, sample := range series {
		if !sample.Timestamp.Before(start) && !sample.Timestamp.After(end) {
			return true
		}
	}
	return false
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

func memoryContextKey(sample metrics.MetricSample) string {
	labels := memoryContextLabels(sample.Labels)
	encoded, _ := json.Marshal(labels)
	return string(sample.EntityID) + "|" + string(encoded)
}

func memoryContextLabels(labels map[string]string) map[string]string {
	contextLabels := make(map[string]string, len(labels))
	for _, key := range query.ServicegraphRequestContextLabels() {
		if value, ok := labels[key]; ok {
			contextLabels[key] = value
		}
	}
	return contextLabels
}

// QueryCatalogStats aggregates per-(metric, source) sample counts and
// last-seen timestamps across samples that fall inside [now-lookback, now].
// The source dimension is read from semconv.LabelSource on each sample;
// samples with no source label collapse to SourceID "".
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
		if edgeRetired(existing) {
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
		if edgeRetired(e) {
			continue
		}
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

// RetireRunsOn records the terminal lifecycle timestamp without changing the
// edge's observations. Retired runs_on edges are excluded from GetLatest but
// remain queryable through QueryEdges for audit use.
func (s *InMemoryGraphStore) RetireRunsOn(_ context.Context, podID, nodeID entities.EntityID, podUID string, retiredAt time.Time) error {
	edgeID := topology.ComputeEdgeIDWithDiscriminator(podID, nodeID, topology.RelRunsOn, podUID)
	s.mu.Lock()
	defer s.mu.Unlock()

	edge, ok := s.edges[edgeID]
	if !ok || edge.Relation != topology.RelRunsOn || edgeRetired(edge) {
		return nil
	}
	metadata := make(map[string]string, len(edge.Metadata)+1)
	for key, value := range edge.Metadata {
		metadata[key] = value
	}
	metadata["retired_at"] = retiredAt.UTC().Format(time.RFC3339Nano)
	edge.Metadata = metadata
	s.edges[edgeID] = edge
	return nil
}

func edgeRetired(edge topology.Edge) bool {
	return edge.Relation == topology.RelRunsOn && edge.Metadata != nil && edge.Metadata["retired_at"] != ""
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
