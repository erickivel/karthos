package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/topology"
)

// helpers

func svc(name string) entities.EntityID {
	return entities.ComputeEntityID(entities.KindService, "default", name)
}

// ─── MetricStore tests ────────────────────────────────────────────────────────

func TestMetricStore_WriteQueryRoundTrip(t *testing.T) {
	store := NewInMemoryMetricStore()
	ctx := context.Background()

	idA := svc("a")
	now := time.Now()
	samples := []metrics.MetricSample{
		{MetricName: "cpu", EntityID: idA, Value: 1.0, Timestamp: now},
		{MetricName: "mem", EntityID: idA, Value: 2.0, Timestamp: now},
		{MetricName: "cpu", EntityID: svc("b"), Value: 3.0, Timestamp: now},
	}

	require.NoError(t, store.Write(ctx, samples))

	got, err := store.Query(ctx, query.MetricQuery{})
	require.NoError(t, err)
	assert.Len(t, got, 3)
}

func TestMetricStoreOwnsWrittenAndReturnedSampleLabels(t *testing.T) {
	store := NewInMemoryMetricStore()
	labels := map[string]string{"source": "kubelet"}
	require.NoError(t, store.Write(context.Background(), []metrics.MetricSample{{
		MetricName: "cpu", EntityID: svc("copied-sample"), Value: 1, Timestamp: time.Now(), Labels: labels,
	}}))
	labels["source"] = "mutated"

	first, err := store.Query(context.Background(), query.MetricQuery{MetricName: "cpu"})
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, "kubelet", first[0].Labels["source"])
	first[0].Labels["source"] = "returned-mutation"

	second, err := store.Query(context.Background(), query.MetricQuery{MetricName: "cpu"})
	require.NoError(t, err)
	assert.Equal(t, "kubelet", second[0].Labels["source"])
}

func TestMetricStore_FilterByMetricName(t *testing.T) {
	store := NewInMemoryMetricStore()
	ctx := context.Background()

	idA := svc("a")
	now := time.Now()
	require.NoError(t, store.Write(ctx, []metrics.MetricSample{
		{MetricName: "cpu", EntityID: idA, Value: 1.0, Timestamp: now},
		{MetricName: "mem", EntityID: idA, Value: 2.0, Timestamp: now},
		{MetricName: "cpu", EntityID: svc("b"), Value: 3.0, Timestamp: now},
	}))

	got, err := store.Query(ctx, query.MetricQuery{MetricName: "cpu"})
	require.NoError(t, err)
	assert.Len(t, got, 2)
	for _, s := range got {
		assert.Equal(t, "cpu", s.MetricName)
	}
}

func TestMetricStore_FilterByEntityID(t *testing.T) {
	store := NewInMemoryMetricStore()
	ctx := context.Background()

	idA := svc("a")
	idB := svc("b")
	now := time.Now()
	require.NoError(t, store.Write(ctx, []metrics.MetricSample{
		{MetricName: "cpu", EntityID: idA, Value: 1.0, Timestamp: now},
		{MetricName: "cpu", EntityID: idB, Value: 2.0, Timestamp: now},
		{MetricName: "mem", EntityID: idA, Value: 3.0, Timestamp: now},
	}))

	got, err := store.Query(ctx, query.MetricQuery{EntityIDs: []entities.EntityID{idA}})
	require.NoError(t, err)
	assert.Len(t, got, 2)
	for _, s := range got {
		assert.Equal(t, idA, s.EntityID)
	}
}

func TestMetricStore_TimeRangeFilter(t *testing.T) {
	store := NewInMemoryMetricStore()
	ctx := context.Background()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	idA := svc("a")
	require.NoError(t, store.Write(ctx, []metrics.MetricSample{
		{MetricName: "cpu", EntityID: idA, Value: 1.0, Timestamp: base},
		{MetricName: "cpu", EntityID: idA, Value: 2.0, Timestamp: base.Add(time.Minute)},
		{MetricName: "cpu", EntityID: idA, Value: 3.0, Timestamp: base.Add(2 * time.Minute)},
	}))

	got, err := store.Query(ctx, query.MetricQuery{
		From: base.Add(30 * time.Second),
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, s := range got {
		assert.True(t, !s.Timestamp.Before(base.Add(30*time.Second)))
	}
}

func TestMetricStore_ResolvedTrendGridUsesLatestFreshObservation(t *testing.T) {
	store := NewInMemoryMetricStore()
	ctx := context.Background()
	base := time.Date(2026, 8, 25, 15, 0, 0, 0, time.UTC)
	id := svc("grid")
	require.NoError(t, store.Write(ctx, []metrics.MetricSample{
		{MetricName: "k8s.pod.cpu.usage", EntityID: id, Value: 42, Timestamp: base.Add(3 * time.Second)},
		{MetricName: "k8s.pod.cpu.usage", EntityID: id, Value: 68, Timestamp: base.Add(33 * time.Second)},
	}))
	resolved, err := query.ResolveMetricQuery(query.MetricQuery{
		MetricName: "k8s.pod.cpu.usage",
		From:       base,
		To:         base.Add(45 * time.Second),
	}, base)
	require.NoError(t, err)

	got, err := store.Query(ctx, resolved.MetricQuery)

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, []time.Time{base.Add(15 * time.Second), base.Add(30 * time.Second), base.Add(45 * time.Second)}, []time.Time{got[0].Timestamp, got[1].Timestamp, got[2].Timestamp})
	assert.Equal(t, []float64{42, 42, 68}, []float64{got[0].Value, got[1].Value, got[2].Value})
}

func TestMetricStore_ResolvedTrendGridOmitsExpiredObservation(t *testing.T) {
	store := NewInMemoryMetricStore()
	ctx := context.Background()
	base := time.Date(2026, 8, 25, 15, 0, 0, 0, time.UTC)
	require.NoError(t, store.Write(ctx, []metrics.MetricSample{{
		MetricName: "k8s.pod.cpu.usage", EntityID: svc("stale"), Value: 42, Timestamp: base,
	}}))
	resolved, err := query.ResolveMetricQuery(query.MetricQuery{
		MetricName: "k8s.pod.cpu.usage",
		From:       base.Add(5*time.Minute + 15*time.Second),
		To:         base.Add(5*time.Minute + 30*time.Second),
	}, base)
	require.NoError(t, err)

	got, err := store.Query(ctx, resolved.MetricQuery)

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestMetricStore_ResolvedTrendGridEvaluatesCounterIncreaseFromRawObservations(t *testing.T) {
	store := NewInMemoryMetricStore()
	ctx := context.Background()
	base := time.Date(2026, 8, 25, 15, 0, 0, 0, time.UTC)
	require.NoError(t, store.Write(ctx, []metrics.MetricSample{
		{MetricName: "servicegraph.request.count", EntityID: svc("counter-grid"), Value: 5, Timestamp: base.Add(3 * time.Second), Labels: map[string]string{"temporality": "delta"}},
		{MetricName: "servicegraph.request.count", EntityID: svc("counter-grid"), Value: 7, Timestamp: base.Add(33 * time.Second), Labels: map[string]string{"temporality": "delta"}},
	}))
	resolved, err := query.ResolveMetricQuery(query.MetricQuery{
		MetricName:  "servicegraph.request.count",
		From:        base,
		To:          base.Add(45 * time.Second),
		Aggregation: metrics.AggregationIncrease,
		Window:      time.Minute,
		Unit:        metrics.UnitCount,
	}, base)
	require.NoError(t, err)

	got, err := store.Query(ctx, resolved.MetricQuery)

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, []time.Time{base.Add(15 * time.Second), base.Add(30 * time.Second), base.Add(45 * time.Second)}, []time.Time{got[0].Timestamp, got[1].Timestamp, got[2].Timestamp})
	assert.Equal(t, []float64{5, 5, 12}, []float64{got[0].Value, got[1].Value, got[2].Value})
}

func TestMetricStore_FilterByEdgeIDs(t *testing.T) {
	store := NewInMemoryMetricStore()
	ctx := context.Background()

	sourceA := entities.ComputeEntityID(entities.KindService, "cluster", "default", "service-a")
	targetB := entities.ComputeEntityID(entities.KindService, "cluster", "default", "service-b")
	targetC := entities.ComputeEntityID(entities.KindService, "cluster", "default", "service-c")
	now := time.Now()
	require.NoError(t, store.Write(ctx, []metrics.MetricSample{
		{
			MetricName: "servicegraph.request.count",
			EntityID:   svc("servicegraph"),
			Value:      1.0,
			Timestamp:  now,
			Labels: map[string]string{
				"edge_client_id": string(sourceA),
				"edge_server_id": string(targetB),
			},
		},
		{
			MetricName: "servicegraph.request.count",
			EntityID:   svc("servicegraph"),
			Value:      2.0,
			Timestamp:  now,
			Labels: map[string]string{
				"edge_client_id": string(sourceA),
				"edge_server_id": string(targetC),
			},
		},
	}))

	got, err := store.Query(ctx, query.MetricQuery{
		MetricName:   "servicegraph.request.count",
		EdgeSourceID: sourceA,
		EdgeTargetID: targetB,
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, string(targetB), got[0].Labels["edge_server_id"])
}

func TestMetricStore_QueryCatalogStats(t *testing.T) {
	store := NewInMemoryMetricStore()
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	store.nowFn = func() time.Time { return now }
	ctx := context.Background()

	idA := svc("a")
	idB := svc("b")
	require.NoError(t, store.Write(ctx, []metrics.MetricSample{
		// In window, kubelet source × 3 samples for cpu.usage
		{MetricName: "k8s.pod.cpu.usage", EntityID: idA, Value: 0.1, Timestamp: now.Add(-30 * time.Minute), Labels: map[string]string{"source": "kubelet"}},
		{MetricName: "k8s.pod.cpu.usage", EntityID: idA, Value: 0.2, Timestamp: now.Add(-20 * time.Minute), Labels: map[string]string{"source": "kubelet"}},
		{MetricName: "k8s.pod.cpu.usage", EntityID: idB, Value: 0.3, Timestamp: now.Add(-5 * time.Minute), Labels: map[string]string{"source": "kubelet"}},
		// In window, mesh.istio source × 1 sample for request.count
		{MetricName: "servicegraph.request.count", EntityID: idA, Value: 1.0, Timestamp: now.Add(-10 * time.Minute), Labels: map[string]string{"source": "mesh.istio"}},
		// Out of window — must be excluded
		{MetricName: "k8s.pod.cpu.usage", EntityID: idA, Value: 9.9, Timestamp: now.Add(-2 * time.Hour), Labels: map[string]string{"source": "kubelet"}},
	}))

	stats, err := store.QueryCatalogStats(ctx, time.Hour)
	require.NoError(t, err)

	byMetric := make(map[string][]SourceStat, len(stats))
	for _, s := range stats {
		byMetric[s.MetricName] = s.BySource
	}

	cpu := byMetric["k8s.pod.cpu.usage"]
	require.Len(t, cpu, 1)
	assert.Equal(t, "kubelet", cpu[0].SourceID)
	assert.EqualValues(t, 3, cpu[0].SampleCount)
	assert.Equal(t, now.Add(-5*time.Minute), cpu[0].LastSeenAt)

	traffic := byMetric["servicegraph.request.count"]
	require.Len(t, traffic, 1)
	assert.Equal(t, "mesh.istio", traffic[0].SourceID)
	assert.EqualValues(t, 1, traffic[0].SampleCount)

}

// ─── GraphStore tests ─────────────────────────────────────────────────────────

func nodeIDs(nodes []topology.Node) []entities.EntityID {
	ids := make([]entities.EntityID, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return ids
}

func TestGraphStore_MaterializesAuthoritativeStructuralSnapshot(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	store.nowFn = func() time.Time { return now }

	serviceA := svc("api")
	serviceB := svc("worker")
	freshCalls := topology.NewEdge(serviceA, serviceB, topology.RelCalls, now.Add(-time.Minute))

	require.NoError(t, store.PutObservedEdges(ctx, []topology.Edge{freshCalls}))
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{
		CreatedAt: now,
		Graph: topology.Graph{Nodes: []topology.Node{
			{ID: serviceA, Kind: entities.KindService},
			{ID: serviceB, Kind: entities.KindService},
		}},
	}))

	materialized, err := store.Materialize(ctx, now)
	require.NoError(t, err)
	require.NotZero(t, materialized.ID)
	require.ElementsMatch(t, []entities.EntityID{serviceA, serviceB}, nodeIDs(materialized.Graph.Nodes))
	require.Len(t, materialized.Graph.Edges, 1)
	require.Equal(t, freshCalls.ID, materialized.Graph.Edges[0].ID)

	selected, err := store.GetAt(ctx, now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, materialized.ID, selected.ID)
}

func TestGraphStore_SelectsRetainedSnapshotAtRequestedTime(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	firstAt := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(5 * time.Minute)
	firstService := svc("first")
	secondService := svc("second")

	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{
		CreatedAt: firstAt,
		Graph:     topology.Graph{Nodes: []topology.Node{{ID: firstService, Kind: entities.KindService}}},
	}))
	first, err := store.Materialize(ctx, firstAt)
	require.NoError(t, err)
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{
		CreatedAt: secondAt,
		Graph:     topology.Graph{Nodes: []topology.Node{{ID: secondService, Kind: entities.KindService}}},
	}))
	second, err := store.Materialize(ctx, secondAt)
	require.NoError(t, err)

	latest, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Equal(t, second.ID, latest.ID)
	require.Equal(t, []entities.EntityID{secondService}, nodeIDs(latest.Graph.Nodes))

	atFirst, err := store.GetAt(ctx, firstAt.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, first.ID, atFirst.ID)
	require.Equal(t, []entities.EntityID{firstService}, nodeIDs(atFirst.Graph.Nodes))
}

func TestGraphStore_OnlyIncludesFreshObservedEdgesWithStructuralEndpoints(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	serviceA, serviceB, staleService := svc("a"), svc("b"), svc("stale")
	fresh := topology.NewEdge(serviceA, serviceB, topology.RelCalls, now.Add(-time.Minute))
	stale := topology.NewEdge(serviceA, serviceB, topology.RelCalls, now.Add(-6*time.Minute))
	missingEndpoint := topology.NewEdge(serviceA, staleService, topology.RelCalls, now.Add(-time.Minute))

	require.NoError(t, store.PutObservedEdges(ctx, []topology.Edge{fresh, stale, missingEndpoint}))
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{
		CreatedAt: now,
		Graph:     topology.Graph{Nodes: []topology.Node{{ID: serviceA, Kind: entities.KindService}, {ID: serviceB, Kind: entities.KindService}}},
	}))

	snapshot, err := store.Materialize(ctx, now)
	require.NoError(t, err)
	require.Len(t, snapshot.Graph.Edges, 1)
	require.Equal(t, fresh.ID, snapshot.Graph.Edges[0].ID)
	require.Equal(t, fresh.LastSeen, snapshot.Graph.Edges[0].LastSeen)
}

func TestGraphStore_MaterializedSnapshotOwnsReturnedValues(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	at := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	labels := map[string]string{"name": "api"}
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{CreatedAt: at, Graph: topology.Graph{Nodes: []topology.Node{{
		ID: svc("api"), Kind: entities.KindService, Labels: labels,
	}}}}))
	_, err := store.Materialize(ctx, at)
	require.NoError(t, err)
	labels["name"] = "mutated"

	first, err := store.GetLatest(ctx)
	require.NoError(t, err)
	first.Graph.Nodes[0].Labels["name"] = "returned-mutation"
	second, err := store.GetLatest(ctx)
	require.NoError(t, err)
	assert.Equal(t, "api", second.Graph.Nodes[0].Labels["name"])
}

func TestGraphStore_ExcludesRetiredRunsOnFromMaterializedSnapshot(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	at := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	podID := entities.ComputeEntityID(entities.KindPod, "cluster", "default", "job-0")
	nodeID := entities.ComputeEntityID(entities.KindNode, "cluster", "node-a")
	runsOn := lifecycleRunsOnEdge(podID, nodeID, "uid-old", at.Add(-time.Minute))
	require.NoError(t, store.PutObservedEdges(ctx, []topology.Edge{runsOn}))
	require.NoError(t, store.RetireRunsOn(ctx, podID, nodeID, "uid-old", at))
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{CreatedAt: at, Graph: topology.Graph{
		Nodes: []topology.Node{{ID: podID, Kind: entities.KindPod}, {ID: nodeID, Kind: entities.KindNode}},
	}}))

	snapshot, err := store.Materialize(ctx, at)
	require.NoError(t, err)
	assert.Empty(t, snapshot.Graph.Edges)
}

func TestGraphStore_PrunesMaterializedSnapshotsAfterRetention(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	firstAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(graphSnapshotRetention + time.Second)
	service := svc("api")
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{
		CreatedAt: firstAt,
		Graph:     topology.Graph{Nodes: []topology.Node{{ID: service, Kind: entities.KindService}}},
	}))
	_, err := store.Materialize(ctx, firstAt)
	require.NoError(t, err)
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{
		CreatedAt: secondAt,
		Graph:     topology.Graph{Nodes: []topology.Node{{ID: service, Kind: entities.KindService}}},
	}))
	_, err = store.Materialize(ctx, secondAt)
	require.NoError(t, err)

	old, err := store.GetAt(ctx, firstAt)
	require.NoError(t, err)
	assert.Nil(t, old)
}

func lifecycleRunsOnEdge(source, target entities.EntityID, podUID string, firstSeen time.Time) topology.Edge {
	edge := topology.NewEdge(source, target, topology.RelRunsOn, firstSeen)
	edge.ID = topology.ComputeEdgeIDWithDiscriminator(source, target, topology.RelRunsOn, podUID)
	edge.Metadata = map[string]string{"k8s.pod.uid": podUID}
	return edge
}

func TestMetricStoreExplicitCounterAggregations(t *testing.T) {
	store := NewInMemoryMetricStore()
	end := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	labels := map[string]string{"temporality": "cumulative", "route": "/checkout"}
	var samples []metrics.MetricSample
	for _, point := range []struct {
		offset time.Duration
		value  float64
	}{{-70 * time.Second, 100}, {-55 * time.Second, 105}, {-20 * time.Second, 130}, {0, 140}} {
		samples = append(samples, metrics.MetricSample{MetricName: "servicegraph.request.count", EntityID: svc("a"), Value: point.value, Timestamp: end.Add(point.offset), Labels: labels})
	}
	require.NoError(t, store.Write(context.Background(), samples))

	q := query.MetricQuery{MetricName: "servicegraph.request.count", Aggregation: metrics.AggregationIncrease, Window: time.Minute, To: end, Unit: metrics.UnitCount}
	increase, err := store.Query(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, increase, 1)
	assert.Equal(t, 40.0, increase[0].Value)

	q.Aggregation, q.Unit = metrics.AggregationWindowRate, metrics.UnitPerSecond
	rate, err := store.Query(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, rate, 1)
	assert.InDelta(t, 40.0/60.0, rate[0].Value, 1e-9)

	q.Aggregation = metrics.AggregationActiveRate
	active, err := store.Query(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.InDelta(t, 35.0/55.0, active[0].Value, 1e-9)
}

func TestMetricStoreCounterRequiresMatureCumulativeBaseline(t *testing.T) {
	store := NewInMemoryMetricStore()
	end := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "servicegraph.request.count", EntityID: svc("new"), Value: 1, Timestamp: end.Add(-30 * time.Second)},
		{MetricName: "servicegraph.request.count", EntityID: svc("new"), Value: 10, Timestamp: end},
	}))
	got, err := store.Query(context.Background(), query.MetricQuery{MetricName: "servicegraph.request.count", Aggregation: metrics.AggregationIncrease, Window: time.Minute, To: end})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestMetricStoreDeltaTotalsAndNoActiveRate(t *testing.T) {
	store := NewInMemoryMetricStore()
	end := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	labels := map[string]string{"temporality": "delta"}
	require.NoError(t, store.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "servicegraph.request.count", EntityID: svc("delta"), Value: 15, Timestamp: end.Add(-30 * time.Second), Labels: labels},
		{MetricName: "servicegraph.request.count", EntityID: svc("delta"), Value: 25, Timestamp: end, Labels: labels},
	}))
	q := query.MetricQuery{MetricName: "servicegraph.request.count", Aggregation: metrics.AggregationIncrease, Window: time.Minute, To: end}
	got, err := store.Query(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 40.0, got[0].Value)
	q.Aggregation = metrics.AggregationActiveRate
	got, err = store.Query(context.Background(), q)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestMetricStoreResetAwareIncrease(t *testing.T) {
	store := NewInMemoryMetricStore()
	end := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "servicegraph.request.count", EntityID: svc("reset"), Value: 100, Timestamp: end.Add(-70 * time.Second)},
		{MetricName: "servicegraph.request.count", EntityID: svc("reset"), Value: 10, Timestamp: end.Add(-50 * time.Second)},
		{MetricName: "servicegraph.request.count", EntityID: svc("reset"), Value: 40, Timestamp: end},
	}))
	got, err := store.Query(context.Background(), query.MetricQuery{MetricName: "servicegraph.request.count", Aggregation: metrics.AggregationIncrease, Window: time.Minute, To: end})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 40.0, got[0].Value)
}

func TestMetricStoreCumulativeIncreaseRequiresBaselineInsideDoubleWindowHistory(t *testing.T) {
	end := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	q := query.MetricQuery{
		MetricName:  "servicegraph.request.count",
		Aggregation: metrics.AggregationIncrease,
		Window:      time.Minute,
		To:          end,
		Unit:        metrics.UnitCount,
	}

	t.Run("rejects a sparse baseline older than the VM history gate", func(t *testing.T) {
		store := NewInMemoryMetricStore()
		require.NoError(t, store.Write(context.Background(), []metrics.MetricSample{
			{MetricName: q.MetricName, EntityID: svc("sparse-old"), Value: 100, Timestamp: end.Add(-3 * time.Minute)},
			{MetricName: q.MetricName, EntityID: svc("sparse-old"), Value: 110, Timestamp: end.Add(-30 * time.Second)},
			{MetricName: q.MetricName, EntityID: svc("sparse-old"), Value: 140, Timestamp: end},
		}))

		got, err := store.Query(context.Background(), q)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("accepts an inclusive baseline at the requested window boundary", func(t *testing.T) {
		store := NewInMemoryMetricStore()
		require.NoError(t, store.Write(context.Background(), []metrics.MetricSample{
			{MetricName: q.MetricName, EntityID: svc("boundary"), Value: 100, Timestamp: end.Add(-time.Minute)},
			{MetricName: q.MetricName, EntityID: svc("boundary"), Value: 110, Timestamp: end.Add(-30 * time.Second)},
			{MetricName: q.MetricName, EntityID: svc("boundary"), Value: 140, Timestamp: end},
		}))

		got, err := store.Query(context.Background(), q)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, 40.0, got[0].Value)
	})
}

func TestMetricStoreWindowMeanCumulative(t *testing.T) {
	store := NewInMemoryMetricStore()
	end := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	labels := map[string]string{"temporality": "cumulative", "route": "/checkout"}
	require.NoError(t, store.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "http.server.request.duration_sum", EntityID: svc("mean"), Value: 100, Timestamp: end.Add(-time.Minute), Labels: labels},
		{MetricName: "http.server.request.duration_sum", EntityID: svc("mean"), Value: 220, Timestamp: end, Labels: labels},
		{MetricName: "http.server.request.duration_count", EntityID: svc("mean"), Value: 10, Timestamp: end.Add(-time.Minute), Labels: labels},
		{MetricName: "http.server.request.duration_count", EntityID: svc("mean"), Value: 14, Timestamp: end, Labels: labels},
	}))

	got, err := store.Query(context.Background(), query.MetricQuery{MetricName: "http.server.request.duration", Aggregation: metrics.AggregationWindowMean, Window: time.Minute, To: end, Unit: metrics.UnitNanoseconds})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 30.0, got[0].Value)
	assert.Equal(t, "http.server.request.duration", got[0].MetricName)
	assert.Equal(t, metrics.UnitNanoseconds, got[0].Unit)
	assert.Equal(t, end, got[0].Timestamp)
	assert.Equal(t, labels, got[0].Labels)
}

func TestMetricStoreWindowMeanResetAwareCumulative(t *testing.T) {
	store := NewInMemoryMetricStore()
	end := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	labels := map[string]string{"temporality": "cumulative", "route": "/reset"}
	require.NoError(t, store.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "http.server.request.duration_sum", EntityID: svc("mean-reset"), Value: 100, Timestamp: end.Add(-time.Minute), Labels: labels},
		{MetricName: "http.server.request.duration_sum", EntityID: svc("mean-reset"), Value: 20, Timestamp: end.Add(-30 * time.Second), Labels: labels},
		{MetricName: "http.server.request.duration_sum", EntityID: svc("mean-reset"), Value: 50, Timestamp: end, Labels: labels},
		{MetricName: "http.server.request.duration_count", EntityID: svc("mean-reset"), Value: 10, Timestamp: end.Add(-time.Minute), Labels: labels},
		{MetricName: "http.server.request.duration_count", EntityID: svc("mean-reset"), Value: 2, Timestamp: end.Add(-30 * time.Second), Labels: labels},
		{MetricName: "http.server.request.duration_count", EntityID: svc("mean-reset"), Value: 5, Timestamp: end, Labels: labels},
	}))

	got, err := store.Query(context.Background(), query.MetricQuery{MetricName: "http.server.request.duration", Aggregation: metrics.AggregationWindowMean, Window: time.Minute, To: end})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 10.0, got[0].Value)
}

func TestMetricStoreWindowMeanDelta(t *testing.T) {
	store := NewInMemoryMetricStore()
	end := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	labels := map[string]string{"temporality": "delta", "route": "/delta"}
	require.NoError(t, store.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "http.server.request.duration_sum", EntityID: svc("mean-delta"), Value: 40, Timestamp: end.Add(-30 * time.Second), Labels: labels},
		{MetricName: "http.server.request.duration_sum", EntityID: svc("mean-delta"), Value: 80, Timestamp: end, Labels: labels},
		{MetricName: "http.server.request.duration_count", EntityID: svc("mean-delta"), Value: 1, Timestamp: end.Add(-30 * time.Second), Labels: labels},
		{MetricName: "http.server.request.duration_count", EntityID: svc("mean-delta"), Value: 3, Timestamp: end, Labels: labels},
	}))

	got, err := store.Query(context.Background(), query.MetricQuery{MetricName: "http.server.request.duration", Aggregation: metrics.AggregationWindowMean, Window: time.Minute, To: end})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 30.0, got[0].Value)
}

func TestMetricStoreWindowMeanOmitsMissingIncompleteAndZeroCountBranches(t *testing.T) {
	end := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	queryMean := query.MetricQuery{MetricName: "http.server.request.duration", Aggregation: metrics.AggregationWindowMean, Window: time.Minute, To: end}
	tests := []struct {
		name    string
		samples []metrics.MetricSample
	}{
		{
			name: "missing count branch",
			samples: []metrics.MetricSample{
				{MetricName: "http.server.request.duration_sum", EntityID: svc("missing"), Value: 10, Timestamp: end.Add(-time.Minute)},
				{MetricName: "http.server.request.duration_sum", EntityID: svc("missing"), Value: 20, Timestamp: end},
			},
		},
		{
			name: "incomplete count branch",
			samples: []metrics.MetricSample{
				{MetricName: "http.server.request.duration_sum", EntityID: svc("incomplete"), Value: 10, Timestamp: end.Add(-time.Minute)},
				{MetricName: "http.server.request.duration_sum", EntityID: svc("incomplete"), Value: 20, Timestamp: end},
				{MetricName: "http.server.request.duration_count", EntityID: svc("incomplete"), Value: 1, Timestamp: end.Add(-30 * time.Second)},
				{MetricName: "http.server.request.duration_count", EntityID: svc("incomplete"), Value: 2, Timestamp: end},
			},
		},
		{
			name: "zero count increase",
			samples: []metrics.MetricSample{
				{MetricName: "http.server.request.duration_sum", EntityID: svc("zero"), Value: 10, Timestamp: end.Add(-time.Minute)},
				{MetricName: "http.server.request.duration_sum", EntityID: svc("zero"), Value: 20, Timestamp: end},
				{MetricName: "http.server.request.duration_count", EntityID: svc("zero"), Value: 1, Timestamp: end.Add(-time.Minute)},
				{MetricName: "http.server.request.duration_count", EntityID: svc("zero"), Value: 1, Timestamp: end},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewInMemoryMetricStore()
			require.NoError(t, store.Write(context.Background(), tt.samples))
			got, err := store.Query(context.Background(), queryMean)
			require.NoError(t, err)
			assert.Empty(t, got)
		})
	}
}
