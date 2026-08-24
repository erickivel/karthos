package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/topology"
)

func seedGraph(t *testing.T) *persistence.InMemoryGraphStore {
	t.Helper()
	gs := persistence.NewInMemoryGraphStore()

	svcA := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-a")
	svcB := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-b")
	podX := entities.ComputeEntityID(entities.KindPod, "cluster", "kube-system", "pod-x")
	nodeN := entities.ComputeEntityID(entities.KindNode, "cluster", "node-1")

	now := time.Now().UTC()
	snap := topology.Snapshot{
		From:      now,
		To:        now,
		CreatedAt: now,
		Graph: topology.Graph{
			Timestamp: now,
			Nodes: []topology.Node{
				{ID: svcA, Kind: entities.KindService, Labels: map[string]string{"namespace": "default", "name": "svc-a"}},
				{ID: svcB, Kind: entities.KindService, Labels: map[string]string{"namespace": "default", "name": "svc-b"}},
				{ID: podX, Kind: entities.KindPod, Labels: map[string]string{"namespace": "kube-system", "name": "pod-x"}},
				{ID: nodeN, Kind: entities.KindNode, Labels: map[string]string{"name": "node-1"}},
			},
			Edges: []topology.Edge{
				topology.NewEdge(svcA, svcB, topology.RelCalls, now),
				topology.NewEdge(podX, nodeN, topology.RelRunsOn, now),
			},
		},
	}
	require.NoError(t, gs.PutObservedSnapshot(context.Background(), snap))
	return gs
}

func seedEntityRegistry(t *testing.T) *persistence.InMemoryEntityRegistry {
	t.Helper()
	er := persistence.NewInMemoryEntityRegistry()

	svcA := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-a")
	svcB := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-b")
	podX := entities.ComputeEntityID(entities.KindPod, "cluster", "kube-system", "pod-x")
	nodeN := entities.ComputeEntityID(entities.KindNode, "cluster", "node-1")

	ctx := context.Background()
	require.NoError(t, er.UpsertEntity(ctx, &persistence.StoredEntity{EntityID: svcA, EntityKind: entities.KindService, Metadata: map[string]string{"cluster": "cluster", "namespace": "default", "name": "svc-a"}}))
	require.NoError(t, er.UpsertEntity(ctx, &persistence.StoredEntity{EntityID: svcB, EntityKind: entities.KindService, Metadata: map[string]string{"cluster": "cluster", "namespace": "default", "name": "svc-b"}}))
	require.NoError(t, er.UpsertEntity(ctx, &persistence.StoredEntity{EntityID: podX, EntityKind: entities.KindPod, Metadata: map[string]string{"cluster": "cluster", "namespace": "kube-system", "name": "pod-x"}}))
	require.NoError(t, er.UpsertEntity(ctx, &persistence.StoredEntity{EntityID: nodeN, EntityKind: entities.KindNode, Metadata: map[string]string{"cluster": "cluster", "name": "node-1"}}))
	return er
}

func TestGetGraphReturnsSnapshot(t *testing.T) {
	gs := seedGraph(t)
	srv := newTestServer(t, Config{GraphStore: gs, MetricStore: persistence.NewInMemoryMetricStore()})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	health := data["health_enrichment"].(map[string]any)
	require.Equal(t, "complete", health["status"])
	summary := data["summary"].(map[string]any)
	require.EqualValues(t, 4, summary["node_count"])
	require.EqualValues(t, 2, summary["edge_count"])

	// Expanded summary fields.
	kindCounts := summary["kind_counts"].(map[string]any)
	require.EqualValues(t, 2, kindCounts["service"])
	require.EqualValues(t, 1, kindCounts["pod"])
	require.EqualValues(t, 1, kindCounts["node"])

	relationCounts := summary["relation_counts"].(map[string]any)
	require.EqualValues(t, 1, relationCounts["calls"])
	require.EqualValues(t, 1, relationCounts["runs_on"])

	require.IsType(t, float64(0), summary["snapshot_age_seconds"])

	filters := summary["available_filters"].(map[string]any)
	require.Contains(t, filters["kinds"], "service")
	require.Contains(t, filters["kinds"], "pod")
	require.Contains(t, filters["namespaces"], "default")
	require.Contains(t, filters["namespaces"], "kube-system")
	require.Contains(t, filters["relations"], "calls")
}

func TestGetGraphReportsUnavailableHealthEnrichmentWithoutMetricStore(t *testing.T) {
	srv := newTestServer(t, Config{GraphStore: seedGraph(t)})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	health := parseResponseData(t, resp)["health_enrichment"].(map[string]any)
	require.Equal(t, "unavailable", health["status"])
}

func TestGetGraphHealthQueriesStayConstantSize(t *testing.T) {
	gs := persistence.NewInMemoryGraphStore()
	now := time.Now().UTC()
	nodes := make([]topology.Node, 0, 2000)
	for i := 0; i < 1000; i++ {
		nodes = append(nodes,
			topology.Node{ID: entities.EntityID("pod-" + strconv.Itoa(i)), Kind: entities.KindPod},
			topology.Node{ID: entities.EntityID("container-" + strconv.Itoa(i)), Kind: entities.KindContainer},
		)
	}
	require.NoError(t, gs.PutObservedSnapshot(context.Background(), topology.Snapshot{
		CreatedAt: now,
		Graph:     topology.Graph{Timestamp: now, Nodes: nodes},
	}))
	store := &recordingMetricStore{samples: map[string][]metrics.MetricSample{}}
	srv := newTestServer(t, Config{GraphStore: gs, MetricStore: store})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, store.queries, 3)
	for _, q := range store.queries {
		require.Empty(t, q.EntityIDs, "snapshot-wide health query must not grow with graph size")
	}
}

func TestGetGraphReportsPartialHealthEnrichment(t *testing.T) {
	gs := persistence.NewInMemoryGraphStore()
	now := time.Now().UTC()
	require.NoError(t, gs.PutObservedSnapshot(context.Background(), topology.Snapshot{
		CreatedAt: now,
		Graph: topology.Graph{Timestamp: now, Nodes: []topology.Node{
			{ID: "pod-1", Kind: entities.KindPod},
			{ID: "container-1", Kind: entities.KindContainer},
		}},
	}))
	store := &recordingMetricStore{
		samples: map[string][]metrics.MetricSample{},
		errors:  map[string]error{"k8s.pod.phase": errors.New("pod query failed")},
	}
	srv := newTestServer(t, Config{GraphStore: gs, MetricStore: store})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	health := parseResponseData(t, resp)["health_enrichment"].(map[string]any)
	require.Equal(t, "partial", health["status"])
	require.Equal(t, "unavailable", health["pods"])
	require.Equal(t, "complete", health["containers"])
}

func TestGetGraphReturns404WhenEmpty(t *testing.T) {
	gs := persistence.NewInMemoryGraphStore()
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestListNodesFilterByKind(t *testing.T) {
	gs := seedGraph(t)
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph/nodes?kind=service")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	health := data["health_enrichment"].(map[string]any)
	require.Equal(t, "complete", health["status"])
	require.EqualValues(t, 2, data["total"])
}

func TestListNodesWithoutSnapshotIncludesHealthEnrichment(t *testing.T) {
	srv := newTestServer(t, Config{
		GraphStore:  persistence.NewInMemoryGraphStore(),
		MetricStore: persistence.NewInMemoryMetricStore(),
	})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph/nodes")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 0, data["total"])
	health := data["health_enrichment"].(map[string]any)
	require.Equal(t, "complete", health["status"])
	require.Equal(t, "complete", health["pods"])
	require.Equal(t, "complete", health["containers"])
}

func TestListNodesFilterByNamespace(t *testing.T) {
	gs := seedGraph(t)
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph/nodes?namespace=kube-system")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 1, data["total"])
}

func TestListNodesEnriched(t *testing.T) {
	gs := seedGraph(t)
	er := seedEntityRegistry(t)
	srv := newTestServer(t, Config{GraphStore: gs, EntityRegistry: er})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph/nodes?kind=service")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data := parsePaginatedData(t, resp)
	nodes := data["nodes"].([]any)
	require.Len(t, nodes, 2)

	node := nodes[0].(map[string]any)
	require.NotEmpty(t, node["name"])
	require.Equal(t, "default", node["namespace"])
	require.Equal(t, "cluster", node["cluster"])
	require.Equal(t, "unknown", node["status"])
}

func TestListNodesPagination(t *testing.T) {
	gs := seedGraph(t)
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph/nodes?page_size=2")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data, nextToken := parsePaginatedDataWithToken(t, resp)
	require.EqualValues(t, 4, data["total"])
	nodes := data["nodes"].([]any)
	require.Len(t, nodes, 2)
	require.NotEmpty(t, nextToken)

	// Second page.
	resp2 := doGet(t, srv, "/api/v1/graph/nodes?page_size=2&next_token="+nextToken)
	defer func() { require.NoError(t, resp2.Body.Close()) }()
	data2, nextToken2 := parsePaginatedDataWithToken(t, resp2)
	nodes2 := data2["nodes"].([]any)
	require.Len(t, nodes2, 2)
	require.Empty(t, nextToken2)
}

func TestGetNodeReturnsEnrichedEdges(t *testing.T) {
	gs := seedGraph(t)
	er := seedEntityRegistry(t)
	srv := newTestServer(t, Config{GraphStore: gs, EntityRegistry: er})
	defer srv.cancel()

	svcA := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-a")
	resp := doGet(t, srv, "/api/v1/graph/nodes/"+string(svcA))
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	node := data["node"].(map[string]any)
	require.Equal(t, string(svcA), node["id"])
	require.Equal(t, "svc-a", node["name"])
	require.Equal(t, "unknown", node["status"])

	edges := data["edges"].(map[string]any)
	outbound := edges["outbound"].([]any)
	require.Len(t, outbound, 1)

	edge := outbound[0].(map[string]any)
	related := edge["related_node"].(map[string]any)
	require.Equal(t, "svc-b", related["name"])
	require.Equal(t, "service", related["kind"])
}

func TestGraphHealthEnrichmentSelectsLatestSampleRegardlessOfStoreOrder(t *testing.T) {
	now := time.Now().UTC()
	podID := entities.ComputeEntityID(entities.KindPod, "cluster", "default", "health-order")
	orders := []struct {
		name    string
		path    string
		extract func(*testing.T, map[string]any) string
		samples []metrics.MetricSample
	}{
		{
			name: "full graph ascending",
			path: "/api/v1/graph",
			extract: func(t *testing.T, data map[string]any) string {
				t.Helper()
				nodes := data["graph"].(map[string]any)["nodes"].([]any)
				return nodes[0].(map[string]any)["status"].(string)
			},
			samples: []metrics.MetricSample{
				{MetricName: "k8s.pod.phase", EntityID: podID, Value: 1, Timestamp: now.Add(-time.Minute)},
				{MetricName: "k8s.pod.phase", EntityID: podID, Value: 2, Timestamp: now},
			},
		},
		{
			name: "node list descending",
			path: "/api/v1/graph/nodes?kind=pod",
			extract: func(t *testing.T, data map[string]any) string {
				t.Helper()
				nodes := data["nodes"].([]any)
				return nodes[0].(map[string]any)["status"].(string)
			},
			samples: []metrics.MetricSample{
				{MetricName: "k8s.pod.phase", EntityID: podID, Value: 2, Timestamp: now},
				{MetricName: "k8s.pod.phase", EntityID: podID, Value: 1, Timestamp: now.Add(-time.Minute)},
			},
		},
		{
			name: "single node mixed",
			path: "/api/v1/graph/nodes/" + string(podID),
			extract: func(t *testing.T, data map[string]any) string {
				t.Helper()
				return data["node"].(map[string]any)["status"].(string)
			},
			samples: []metrics.MetricSample{
				{MetricName: "k8s.pod.phase", EntityID: podID, Value: 1, Timestamp: now.Add(-time.Minute)},
				{MetricName: "k8s.pod.phase", EntityID: podID, Value: 2, Timestamp: now},
				{MetricName: "k8s.pod.phase", EntityID: podID, Value: 4, Timestamp: now.Add(-30 * time.Second)},
			},
		},
	}

	for _, tt := range orders {
		t.Run(tt.name, func(t *testing.T) {
			gs := persistence.NewInMemoryGraphStore()
			require.NoError(t, gs.PutObservedSnapshot(context.Background(), topology.Snapshot{
				CreatedAt: now,
				Graph:     topology.Graph{Timestamp: now, Nodes: []topology.Node{{ID: podID, Kind: entities.KindPod}}},
			}))
			store := &recordingMetricStore{samples: map[string][]metrics.MetricSample{"k8s.pod.phase": tt.samples}}
			srv := newTestServer(t, Config{GraphStore: gs, MetricStore: store})
			defer srv.cancel()

			resp := doGet(t, srv, tt.path)
			defer func() { require.NoError(t, resp.Body.Close()) }()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			data := parseResponseData(t, resp)
			require.Equal(t, metrics.HealthHealthy, tt.extract(t, data))
		})
	}
}

// countingEntityRegistry wraps an EntityRegistry and counts per-id and batch
// lookups so tests can assert that handlers fold per-edge entity resolution
// into a single batch.
type countingEntityRegistry struct {
	inner      persistence.EntityRegistry
	getCalls   int
	batchCalls int
	batchedIDs int
}

func (c *countingEntityRegistry) UpsertEntity(ctx context.Context, e entities.Entity) error {
	return c.inner.UpsertEntity(ctx, e)
}
func (c *countingEntityRegistry) GetEntity(ctx context.Context, id entities.EntityID) (entities.Entity, error) {
	c.getCalls++
	return c.inner.GetEntity(ctx, id)
}
func (c *countingEntityRegistry) GetEntities(ctx context.Context, ids []entities.EntityID) (map[entities.EntityID]entities.Entity, error) {
	c.batchCalls++
	c.batchedIDs += len(ids)
	return c.inner.GetEntities(ctx, ids)
}
func (c *countingEntityRegistry) ListEntities(ctx context.Context, q persistence.EntityQuery) ([]entities.Entity, error) {
	return c.inner.ListEntities(ctx, q)
}

func TestGetNodeUsesSingleBatchedEntityLookup(t *testing.T) {
	gs := seedGraph(t)
	counting := &countingEntityRegistry{inner: seedEntityRegistry(t)}
	srv := newTestServer(t, Config{GraphStore: gs, EntityRegistry: counting})
	defer srv.cancel()

	svcA := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-a")
	resp := doGet(t, srv, "/api/v1/graph/nodes/"+string(svcA))
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.Equal(t, 1, counting.batchCalls, "handleGetNode should batch entity lookups into a single call")
	require.Equal(t, 0, counting.getCalls, "handleGetNode should not fall back to per-id GetEntity")
}

func TestGetNodeReturns404(t *testing.T) {
	gs := seedGraph(t)
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph/nodes/nonexistent")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestListEdgesFilterBySource(t *testing.T) {
	gs := seedGraph(t)
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	svcA := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-a")
	resp := doGet(t, srv, "/api/v1/graph/edges?source="+string(svcA))
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 1, data["total"])
}

func TestListEdgesFilterByTarget(t *testing.T) {
	gs := seedGraph(t)
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	nodeN := entities.ComputeEntityID(entities.KindNode, "cluster", "node-1")
	resp := doGet(t, srv, "/api/v1/graph/edges?target="+string(nodeN))
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 1, data["total"])
}

func TestListEdgesFilterByRelation(t *testing.T) {
	gs := seedGraph(t)
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph/edges?relation=calls")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 1, data["total"])
}

func TestListEdgesIncludesCurrentStructuralEdges(t *testing.T) {
	gs := persistence.NewInMemoryGraphStore()
	now := time.Now().UTC()
	ingressID := entities.ComputeEntityID(entities.KindIngress, "cluster", "default", "public", "example.test")
	serviceID := entities.ComputeEntityID(entities.KindService, "cluster", "default", "catalog")
	databaseID := entities.ComputeEntityID(entities.KindService, "cluster", "default", "database")
	route := topology.NewEdge(ingressID, serviceID, topology.RelRoutesTo, now)
	calls := topology.NewEdge(serviceID, databaseID, topology.RelCalls, now)
	require.NoError(t, gs.PutStructuralSnapshot(context.Background(), topology.Snapshot{CreatedAt: now, Graph: topology.Graph{
		Edges: []topology.Edge{route},
	}}))
	require.NoError(t, gs.PutObservedSnapshot(context.Background(), topology.Snapshot{CreatedAt: now, Graph: topology.Graph{
		Edges: []topology.Edge{calls},
	}}))

	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()
	resp := doGet(t, srv, "/api/v1/graph/edges?source="+string(ingressID)+"&relation=routes_to")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 1, data["total"])
	edge := data["edges"].([]any)[0].(map[string]any)
	require.Equal(t, route.ID, edge["id"])
}

func TestListEdgesRejectsInvalidEntityIDFilters(t *testing.T) {
	for _, filter := range []string{"source", "target"} {
		t.Run(filter, func(t *testing.T) {
			srv := newTestServer(t, Config{GraphStore: seedGraph(t)})
			defer srv.cancel()
			resp := doGet(t, srv, "/api/v1/graph/edges?"+filter+"=not-an-id")
			defer func() { require.NoError(t, resp.Body.Close()) }()
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestListEdgesPagination(t *testing.T) {
	gs := seedGraph(t)
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph/edges?page_size=1")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data, nextToken := parsePaginatedDataWithToken(t, resp)
	require.EqualValues(t, 2, data["total"])
	edges := data["edges"].([]any)
	require.Len(t, edges, 1)
	require.NotEmpty(t, nextToken)
}

func TestGetGraphIncludesStructuralKindsWhenGraphContainsThem(t *testing.T) {
	gs := persistence.NewInMemoryGraphStore()
	now := time.Now().UTC()

	serviceID := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-a")
	workloadID := entities.ComputeEntityID(entities.KindWorkload, "cluster", "default", "deployment", "app")
	configMapID := entities.ComputeEntityID(entities.KindConfigMap, "cluster", "default", "app-config")
	secretID := entities.ComputeEntityID(entities.KindSecret, "cluster", "default", "app-secret")
	pvcID := entities.ComputeEntityID(entities.KindPVC, "cluster", "default", "app-pvc")
	ingressID := entities.ComputeEntityID(entities.KindIngress, "cluster", "default", "app-ingress", "app.default.local")

	require.NoError(t, gs.PutObservedSnapshot(context.Background(), topology.Snapshot{
		From:      now,
		To:        now,
		CreatedAt: now,
		Graph: topology.Graph{
			Timestamp: now,
			Nodes: []topology.Node{
				{ID: serviceID, Kind: entities.KindService, Labels: map[string]string{"name": "svc-a", "namespace": "default"}},
				{ID: workloadID, Kind: entities.KindWorkload, Labels: map[string]string{"name": "app", "namespace": "default"}},
				{ID: configMapID, Kind: entities.KindConfigMap, Labels: map[string]string{"name": "app-config", "namespace": "default"}},
				{ID: secretID, Kind: entities.KindSecret, Labels: map[string]string{"name": "app-secret", "namespace": "default"}},
				{ID: pvcID, Kind: entities.KindPVC, Labels: map[string]string{"name": "app-pvc", "namespace": "default"}},
				{ID: ingressID, Kind: entities.KindIngress, Labels: map[string]string{"name": "app-ingress", "namespace": "default"}},
			},
			Edges: []topology.Edge{
				topology.NewEdge(serviceID, workloadID, topology.RelExposes, now),
				topology.NewEdge(workloadID, configMapID, topology.RelUses, now),
				topology.NewEdge(workloadID, secretID, topology.RelUses, now),
				topology.NewEdge(workloadID, pvcID, topology.RelMounts, now),
				topology.NewEdge(ingressID, serviceID, topology.RelRoutesTo, now),
			},
		},
	}))

	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	summary := data["summary"].(map[string]any)
	kindCounts := summary["kind_counts"].(map[string]any)
	require.EqualValues(t, 1, kindCounts["service"])
	require.EqualValues(t, 1, kindCounts["workload"])
	require.EqualValues(t, 1, kindCounts["configmap"])
	require.EqualValues(t, 1, kindCounts["secret"])
	require.EqualValues(t, 1, kindCounts["pvc"])
	require.EqualValues(t, 1, kindCounts["ingress"])

	filters := summary["available_filters"].(map[string]any)
	require.Contains(t, filters["kinds"], "service")
	require.Contains(t, filters["kinds"], "workload")
	require.Contains(t, filters["kinds"], "configmap")
	require.Contains(t, filters["kinds"], "secret")
	require.Contains(t, filters["kinds"], "pvc")
	require.Contains(t, filters["kinds"], "ingress")
}
