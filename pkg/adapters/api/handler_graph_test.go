package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
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
	require.NoError(t, gs.PutSnapshot(context.Background(), snap))
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
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
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
	require.EqualValues(t, 2, data["total"])
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
