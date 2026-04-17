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
			MetricName: "request_rate_per_second",
			EntityID:   svc("servicegraph"),
			Value:      1.0,
			Timestamp:  now,
			Labels: map[string]string{
				"edge_client_id": string(sourceA),
				"edge_server_id": string(targetB),
			},
		},
		{
			MetricName: "request_rate_per_second",
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
		MetricName:   "request_rate_per_second",
		EdgeSourceID: sourceA,
		EdgeTargetID: targetB,
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, string(targetB), got[0].Labels["edge_server_id"])
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

	require.NoError(t, store.PutSnapshot(ctx, newSnapshot(eAB, eAC)))

	snap, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.Len(t, snap.Graph.Edges, 2)
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

	require.NoError(t, store.PutSnapshot(ctx, newSnapshot(e1)))
	require.NoError(t, store.PutSnapshot(ctx, newSnapshot(e2)))

	snap, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Len(t, snap.Graph.Edges, 1)
	assert.Equal(t, t2, snap.Graph.Edges[0].LastSeen)
}

func TestGraphStore_QueryEdgesBySource(t *testing.T) {
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

	require.NoError(t, store.PutSnapshot(ctx, newSnapshot(eAB, eAC, eDE)))

	got, err := store.QueryEdges(ctx, query.EdgeQuery{Source: idA})
	require.NoError(t, err)
	assert.Len(t, got, 2)
	for _, e := range got {
		assert.Equal(t, idA, e.Source)
	}
}

func TestGraphStore_NilOnEmpty(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()

	snap, err := store.GetLatest(ctx)
	assert.NoError(t, err)
	assert.Nil(t, snap)
}

func TestGraphStore_AccumulatesNodes(t *testing.T) {
	store := NewInMemoryGraphStore()
	ctx := context.Background()

	nodeID := entities.ComputeEntityID(entities.KindPod, "cluster", "ns", "pod")
	snap := topology.Snapshot{
		Graph: topology.Graph{
			Nodes: []topology.Node{{ID: nodeID, Kind: entities.KindPod}},
			Edges: []topology.Edge{},
		},
	}
	require.NoError(t, store.PutSnapshot(ctx, snap))

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
	require.NoError(t, store.PutSnapshot(ctx, snap1))
	require.NoError(t, store.PutSnapshot(ctx, snap2))

	got, err := store.GetLatest(ctx)
	require.NoError(t, err)
	// Same node written twice should appear once
	assert.Len(t, got.Graph.Nodes, 1)
}
