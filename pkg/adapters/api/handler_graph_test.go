package api

import (
	"context"
	"encoding/json"
	"io"
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

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	snap := topology.Snapshot{
		From: now,
		To:   now,
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

	data := parseResponseData(t, resp)
	require.EqualValues(t, 2, data["total"])
}

func TestListNodesFilterByNamespace(t *testing.T) {
	gs := seedGraph(t)
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph/nodes?namespace=kube-system")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	require.EqualValues(t, 1, data["total"])
}

func TestGetNodeReturnsEdges(t *testing.T) {
	gs := seedGraph(t)
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	svcA := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-a")
	resp := doGet(t, srv, "/api/v1/graph/nodes/"+string(svcA))
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	node := data["node"].(map[string]any)
	require.Equal(t, string(svcA), node["id"])

	edges := data["edges"].(map[string]any)
	outbound := edges["outbound"].([]any)
	require.Len(t, outbound, 1)
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

	data := parseResponseData(t, resp)
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

	data := parseResponseData(t, resp)
	require.EqualValues(t, 1, data["total"])
}

func TestListEdgesFilterByRelation(t *testing.T) {
	gs := seedGraph(t)
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph/edges?relation=calls")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	require.EqualValues(t, 1, data["total"])

	// Also verify runs_on filter works
	resp2 := doGet(t, srv, "/api/v1/graph/edges?relation=runs_on")
	defer func() { require.NoError(t, resp2.Body.Close()) }()
	data2 := parseResponseData(t, resp2)
	require.EqualValues(t, 1, data2["total"])

	// Non-existent relation returns empty
	resp3 := doGet(t, srv, "/api/v1/graph/edges?relation=mounts")
	defer func() { require.NoError(t, resp3.Body.Close()) }()
	data3 := parseResponseData(t, resp3)
	require.EqualValues(t, 0, data3["total"])
}

func TestListEdgesEmptyGraph(t *testing.T) {
	gs := persistence.NewInMemoryGraphStore()
	srv := newTestServer(t, Config{GraphStore: gs})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/graph/edges")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify the response contains an empty array, not null
	rawBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var envelope Response
	require.NoError(t, json.Unmarshal(rawBody, &envelope))
	data := envelope.Data.(map[string]any)
	require.EqualValues(t, 0, data["total"])
}
