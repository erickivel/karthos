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

func newSnapshot(edges ...topology.Edge) topology.Snapshot {
	return topology.Snapshot{
		Graph: topology.Graph{
			Edges:     edges,
			Timestamp: time.Now(),
		},
	}
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

func TestGraphStore_PutAndGetLatest(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()

	idA := svc("a")
	idB := svc("b")
	idC := svc("c")
	now := time.Now()

	eAB := topology.NewEdge(idA, idB, topology.RelCalls, now)
	eAC := topology.NewEdge(idA, idC, topology.RelCalls, now)

	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(eAB, eAC)))

	snap, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.Len(t, snap.Graph.Edges, 2)
}

func TestGraphStore_UsesLatestStructuralSnapshotAndRetainsCurrentObservations(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	now := time.Now().UTC()

	ingressID := entities.ComputeEntityID(entities.KindIngress, "cluster", "demo", "public-api")
	serviceID := entities.ComputeEntityID(entities.KindService, "cluster", "demo", "api")
	databaseID := entities.ComputeEntityID(entities.KindService, "cluster", "demo", "database")
	route := topology.NewEdge(ingressID, serviceID, topology.RelRoutesTo, now)
	calls := topology.NewEdge(serviceID, databaseID, topology.RelCalls, now)

	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{
		Graph: topology.Graph{
			Nodes: []topology.Node{{ID: ingressID, Kind: entities.KindIngress}, {ID: serviceID, Kind: entities.KindService}},
			Edges: []topology.Edge{route},
		},
		CreatedAt: now,
	}))
	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(calls)))

	// The later API projection no longer contains the deleted ingress or its route.
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{
		Graph:     topology.Graph{Nodes: []topology.Node{{ID: serviceID, Kind: entities.KindService}}},
		CreatedAt: now.Add(time.Minute),
	}))

	got, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Len(t, store.structuralSnapshots, 2, "each structural refresh remains in history")
	assert.ElementsMatch(t, []entities.EntityID{serviceID}, nodeIDs(got.Graph.Nodes))
	assert.Len(t, got.Graph.Edges, 1)
	assert.Equal(t, topology.RelCalls, got.Graph.Edges[0].Relation)
}

func nodeIDs(nodes []topology.Node) []entities.EntityID {
	ids := make([]entities.EntityID, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return ids
}

func TestGraphStore_EdgeDeduplication(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()

	idA := svc("a")
	idB := svc("b")

	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Minute)

	e1 := topology.NewEdge(idA, idB, topology.RelCalls, t1)
	e2 := topology.NewEdge(idA, idB, topology.RelCalls, t1)
	e2.LastSeen = t2

	// Same edge ID, different LastSeen.
	assert.Equal(t, e1.ID, e2.ID)

	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(e1)))
	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(e2)))

	snap, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Len(t, snap.Graph.Edges, 1)
	assert.Equal(t, t2, snap.Graph.Edges[0].LastSeen)
}

func TestGraphStore_RetiredRunsOnIsHiddenFromLatestAndImmutable(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	podID := entities.ComputeEntityID(entities.KindPod, "cluster", "default", "job-0")
	nodeID := entities.ComputeEntityID(entities.KindNode, "cluster", "node-a")
	firstSeen := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	retiredAt := firstSeen.Add(time.Minute)
	staleAt := retiredAt.Add(time.Minute)

	runsOn := lifecycleRunsOnEdge(podID, nodeID, "uid-old", firstSeen)
	contains := topology.NewEdge(podID, nodeID, topology.RelContains, firstSeen)
	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(runsOn, contains)))
	require.NoError(t, store.RetireRunsOn(ctx, podID, nodeID, "uid-old", retiredAt))

	staleRunsOn := lifecycleRunsOnEdge(podID, nodeID, "uid-old", firstSeen)
	staleRunsOn.LastSeen = staleAt
	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(staleRunsOn)))

	latest, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Len(t, latest.Graph.Edges, 1)
	require.Equal(t, topology.RelContains, latest.Graph.Edges[0].Relation)

}

func TestGraphStore_ReplacementUIDBecomesCurrentAssignment(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	podID := entities.ComputeEntityID(entities.KindPod, "cluster", "default", "job-0")
	nodeID := entities.ComputeEntityID(entities.KindNode, "cluster", "node-a")
	firstSeen := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	old := lifecycleRunsOnEdge(podID, nodeID, "uid-old", firstSeen)
	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(old)))
	require.NoError(t, store.RetireRunsOn(ctx, podID, nodeID, "uid-old", firstSeen.Add(time.Minute)))

	newAssignment := lifecycleRunsOnEdge(podID, nodeID, "uid-new", firstSeen.Add(2*time.Minute))
	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(newAssignment)))

	latest, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Len(t, latest.Graph.Edges, 1)
	require.Equal(t, newAssignment.ID, latest.Graph.Edges[0].ID)

}

func TestGraphStore_RetirementBeforeFirstSnapshotKeepsLaterBufferedEdgeRetired(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	podID := entities.ComputeEntityID(entities.KindPod, "cluster", "default", "job-0")
	nodeID := entities.ComputeEntityID(entities.KindNode, "cluster", "node-a")
	retiredAt := time.Date(2026, 7, 25, 12, 1, 0, 0, time.UTC)

	require.NoError(t, store.RetireRunsOn(ctx, podID, nodeID, "uid-a", retiredAt))
	buffered := lifecycleRunsOnEdge(podID, nodeID, "uid-a", retiredAt.Add(time.Minute))
	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(buffered)))

	latest, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Empty(t, latest.Graph.Edges)

}

func TestGraphStore_TerminalGraceSurvivesReconciliationUntilItsDeadline(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	observedAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	deadline := observedAt.Add(time.Minute)
	current := observedAt.Add(30 * time.Second)
	store.nowFn = func() time.Time { return current }
	podID := entities.ComputeEntityID(entities.KindPod, "cluster-a", "default", "job-0")
	nodeID := entities.ComputeEntityID(entities.KindNode, "cluster-a", "node-a")
	edge := lifecycleRunsOnEdge(podID, nodeID, "uid-terminal", observedAt)
	edge.Metadata["k8s.cluster.name"] = "cluster-a"
	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(edge)))
	_, err := store.MarkRunsOnTerminal(ctx, RunsOnTerminalGrace{
		ClusterName: "cluster-a",
		Namespace:   "default",
		PodName:     "job-0",
		PodUID:      "uid-terminal",
		NodeName:    "node-a",
		PodID:       podID,
		NodeID:      nodeID,
		ObservedAt:  observedAt,
		Deadline:    deadline,
		Confirmed:   true,
	})
	require.NoError(t, err)

	// A replacement/deletion means the old UID is absent from the restarted
	// tracker, but it remains latest-active throughout its persisted grace.
	pending, err := store.ReconcileRunsOn(ctx, "cluster-a", nil, current)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, deadline, pending[0].Deadline)
	latest, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Len(t, latest.Graph.Edges, 1)

	current = deadline
	pending, err = store.ReconcileRunsOn(ctx, "cluster-a", nil, current)
	require.NoError(t, err)
	require.Empty(t, pending)
	latest, err = store.GetLatest(ctx)
	require.NoError(t, err)
	require.Empty(t, latest.Graph.Edges)
}

func TestGraphStore_RetirementWinsOverDelayedTerminalMarker(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	observedAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	podID := entities.ComputeEntityID(entities.KindPod, "cluster-a", "default", "job-0")
	nodeID := entities.ComputeEntityID(entities.KindNode, "cluster-a", "node-a")
	edge := lifecycleRunsOnEdge(podID, nodeID, "uid-old", observedAt)
	edge.Metadata["k8s.cluster.name"] = "cluster-a"
	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(edge)))
	require.NoError(t, store.RetireRunsOn(ctx, podID, nodeID, "uid-old", observedAt.Add(time.Second)))

	state, err := store.MarkRunsOnTerminal(ctx, RunsOnTerminalGrace{
		ClusterName: "cluster-a", Namespace: "default", PodName: "job-0", PodUID: "uid-old", NodeName: "node-a",
		PodID: podID, NodeID: nodeID, ObservedAt: observedAt, Deadline: observedAt.Add(time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, RunsOnTerminalRetired, state)
	latest, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Empty(t, latest.Graph.Edges)
}

func TestGraphStore_ReconcileUnconfirmedTerminalMarkerWithReplacementRetiresImmediately(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	podID := entities.ComputeEntityID(entities.KindPod, "cluster-a", "default", "job-0")
	nodeID := entities.ComputeEntityID(entities.KindNode, "cluster-a", "node-a")
	old := lifecycleRunsOnEdge(podID, nodeID, "uid-old", now)
	old.Metadata["k8s.cluster.name"] = "cluster-a"
	newAssignment := lifecycleRunsOnEdge(podID, nodeID, "uid-new", now.Add(time.Second))
	newAssignment.Metadata["k8s.cluster.name"] = "cluster-a"
	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(old, newAssignment)))
	_, err := store.MarkRunsOnTerminal(ctx, RunsOnTerminalGrace{
		ClusterName: "cluster-a", Namespace: "default", PodName: "job-0", PodUID: "uid-old", NodeName: "node-a",
		PodID: podID, NodeID: nodeID, ObservedAt: now, Deadline: now.Add(time.Minute), Confirmed: false,
	})
	require.NoError(t, err)

	pending, err := store.ReconcileRunsOn(ctx, "cluster-a", []RunsOnAssignment{{PodID: podID, NodeID: nodeID, PodUID: "uid-new"}}, now.Add(2*time.Second))
	require.NoError(t, err)
	require.Empty(t, pending)
	latest, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Len(t, latest.Graph.Edges, 1)
	require.Equal(t, newAssignment.ID, latest.Graph.Edges[0].ID)
}

func TestGraphStore_ReconcileRunsOnDoesNotCrossClusterBoundary(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	retiredAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	clusterA := lifecycleRunsOnEdge(
		entities.ComputeEntityID(entities.KindPod, "cluster-a", "default", "api"),
		entities.ComputeEntityID(entities.KindNode, "cluster-a", "node-a"),
		"uid-a", retiredAt,
	)
	clusterA.Metadata["k8s.cluster.name"] = "cluster-a"
	clusterB := lifecycleRunsOnEdge(
		entities.ComputeEntityID(entities.KindPod, "cluster-b", "default", "api"),
		entities.ComputeEntityID(entities.KindNode, "cluster-b", "node-b"),
		"uid-b", retiredAt,
	)
	clusterB.Metadata["k8s.cluster.name"] = "cluster-b"
	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(clusterA, clusterB)))

	_, err := store.ReconcileRunsOn(ctx, "cluster-a", nil, retiredAt.Add(time.Minute))
	require.NoError(t, err)

	latest, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Len(t, latest.Graph.Edges, 1)
	require.Equal(t, clusterB.ID, latest.Graph.Edges[0].ID)
}

func TestGraphStore_NonRunsOnRetiredAtMetadataRemainsLatestAndRefreshable(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()
	firstSeen := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	later := firstSeen.Add(time.Minute)
	edge := topology.NewEdge(svc("api"), svc("db"), topology.RelCalls, firstSeen)
	edge.Metadata = map[string]string{"retired_at": "application-level-marker"}
	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(edge)))

	refresh := edge
	refresh.LastSeen = later
	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(refresh)))

	latest, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Len(t, latest.Graph.Edges, 1)
	require.Equal(t, later, latest.Graph.Edges[0].LastSeen)
}

func lifecycleRunsOnEdge(source, target entities.EntityID, podUID string, firstSeen time.Time) topology.Edge {
	edge := topology.NewEdge(source, target, topology.RelRunsOn, firstSeen)
	edge.ID = topology.ComputeEdgeIDWithDiscriminator(source, target, topology.RelRunsOn, podUID)
	edge.Metadata = map[string]string{"k8s.pod.uid": podUID}
	return edge
}

func TestGraphStore_QueryEdgesFiltersCurrentComposedGraph(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()

	idA := svc("a")
	idB := svc("b")
	idC := svc("c")
	idD := svc("d")
	idE := svc("e")
	now := time.Now()

	eAB := topology.NewEdge(idA, idB, topology.RelCalls, now)
	eAC := topology.NewEdge(idA, idC, topology.RelCalls, now)
	eDE := topology.NewEdge(idD, idE, topology.RelCalls, now)
	eStructural := topology.NewEdge(idA, idE, topology.RelRoutesTo, now)

	require.NoError(t, store.PutObservedSnapshot(ctx, newSnapshot(eAB, eAC, eDE)))
	require.NoError(t, store.PutStructuralSnapshot(ctx, newSnapshot(eStructural)))

	got, err := store.QueryEdges(ctx, query.EdgeQuery{Source: idA})
	require.NoError(t, err)
	assert.Len(t, got, 3)
	for _, e := range got {
		assert.Equal(t, idA, e.Source)
	}
	routes, err := store.QueryEdges(ctx, query.EdgeQuery{Relation: topology.RelRoutesTo})
	require.NoError(t, err)
	require.Equal(t, []topology.Edge{eStructural}, routes)
}

func TestGraphStore_NilOnEmpty(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()

	snap, err := store.GetLatest(ctx)
	assert.NoError(t, err)
	assert.Nil(t, snap)
}

func TestGraphStore_StoresObservedNodes(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()

	nodeID := entities.ComputeEntityID(entities.KindPod, "cluster", "ns", "pod")
	snap := topology.Snapshot{
		Graph: topology.Graph{
			Nodes: []topology.Node{{ID: nodeID, Kind: entities.KindPod}},
			Edges: []topology.Edge{},
		},
	}
	require.NoError(t, store.PutObservedSnapshot(ctx, snap))

	got, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Graph.Nodes, 1)
	assert.Equal(t, nodeID, got.Graph.Nodes[0].ID)
	assert.Equal(t, entities.KindPod, got.Graph.Nodes[0].Kind)
}

func TestGraphStore_NodeDeduplication(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()

	nodeID := entities.ComputeEntityID(entities.KindNode, "cluster", "my-node")
	snap1 := topology.Snapshot{
		Graph: topology.Graph{
			Nodes: []topology.Node{{ID: nodeID, Kind: entities.KindNode}},
		},
	}
	snap2 := topology.Snapshot{
		Graph: topology.Graph{
			Nodes: []topology.Node{{ID: nodeID, Kind: entities.KindNode}},
		},
	}
	require.NoError(t, store.PutObservedSnapshot(ctx, snap1))
	require.NoError(t, store.PutObservedSnapshot(ctx, snap2))

	got, err := store.GetLatest(ctx)
	require.NoError(t, err)
	// Same node written twice should appear once
	assert.Len(t, got.Graph.Nodes, 1)
}

func TestGraphStore_RetainsObservedNodesAcrossFlushes(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()

	serviceID := entities.ComputeEntityID(entities.KindService, "cluster", "demo", "gateway")
	podID := entities.ComputeEntityID(entities.KindPod, "cluster", "demo", "gateway-pod")

	firstObservation := topology.Snapshot{
		Graph: topology.Graph{
			Nodes: []topology.Node{
				{
					ID:   serviceID,
					Kind: entities.KindService,
					Labels: map[string]string{
						"name":      "gateway",
						"namespace": "demo",
					},
				},
			},
		},
	}
	runtime := topology.Snapshot{
		Graph: topology.Graph{
			Nodes: []topology.Node{
				{
					ID:   podID,
					Kind: entities.KindPod,
					Labels: map[string]string{
						"name":      "gateway-pod",
						"namespace": "demo",
					},
				},
			},
		},
	}

	require.NoError(t, store.PutObservedSnapshot(ctx, firstObservation))
	require.NoError(t, store.PutObservedSnapshot(ctx, runtime))

	got, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Graph.Nodes, 2)
	assert.ElementsMatch(t, []entities.EntityID{serviceID, podID}, []entities.EntityID{
		got.Graph.Nodes[0].ID,
		got.Graph.Nodes[1].ID,
	})
}

func TestGraphStore_NodeTTLAgesOutStaleNodes(t *testing.T) {
	store := NewInMemoryGraphStore(WithInMemoryNodeTTL(1 * time.Hour))
	ctx := context.Background()

	// Advance a controllable clock.
	base := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	current := base
	store.nowFn = func() time.Time { return current }

	staleID := entities.ComputeEntityID(entities.KindPod, "cluster", "demo", "old-pod")
	freshID := entities.ComputeEntityID(entities.KindPod, "cluster", "demo", "new-pod")

	// Write a snapshot containing the stale node at t=base.
	require.NoError(t, store.PutObservedSnapshot(ctx, topology.Snapshot{
		Graph: topology.Graph{
			Nodes: []topology.Node{{ID: staleID, Kind: entities.KindPod}},
		},
	}))

	// Advance past the TTL, then write a new snapshot with just the fresh node.
	current = base.Add(2 * time.Hour)
	require.NoError(t, store.PutObservedSnapshot(ctx, topology.Snapshot{
		Graph: topology.Graph{
			Nodes: []topology.Node{{ID: freshID, Kind: entities.KindPod}},
		},
	}))

	got, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Len(t, got.Graph.Nodes, 1)
	assert.Equal(t, freshID, got.Graph.Nodes[0].ID)
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
