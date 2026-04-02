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
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func TestStatusReturnsOKWithComponents(t *testing.T) {
	srv := newTestServer(t, Config{
		MetricStore:    persistence.NewInMemoryMetricStore(),
		GraphStore:     persistence.NewInMemoryGraphStore(),
		MetadataStore:  persistence.NewInMemoryMetadataStore(),
		EntityRegistry: persistence.NewInMemoryEntityRegistry(),
	})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	require.Equal(t, "ok", data["status"])

	components := data["components"].(map[string]any)
	require.Equal(t, "available", components["metric_store"])
	require.Equal(t, "available", components["graph_store"])
	require.Equal(t, "available", components["metadata_store"])
	require.Equal(t, "available", components["entity_registry"])
}

func TestStatusReportsUnavailableComponents(t *testing.T) {
	srv := newTestServer(t, Config{})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	components := data["components"].(map[string]any)
	require.Equal(t, "unavailable", components["metric_store"])
	require.Equal(t, "unavailable", components["graph_store"])
}

func TestDiscoveryReturnsProfileSummary(t *testing.T) {
	ms := persistence.NewInMemoryMetadataStore()
	require.NoError(t, ms.PutDiscovery(context.Background(), profile.ClusterProfile{
		ClusterID:   entities.ComputeEntityID(entities.KindCluster, "test-cluster"),
		ClusterName: "test-cluster",
		Nodes:       []entities.Node{{EntityID: "n1"}},
		Namespaces:  []entities.Namespace{{EntityID: "ns1"}, {EntityID: "ns2"}},
		Workloads:   []entities.Workload{{EntityID: "w1"}, {EntityID: "w2"}, {EntityID: "w3"}},
		Services:    []entities.Service{{EntityID: "s1"}},
	}))

	srv := newTestServer(t, Config{MetadataStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/discovery")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	require.Equal(t, "test-cluster", data["cluster_name"])
	require.EqualValues(t, 1, data["node_count"])
	require.EqualValues(t, 2, data["namespace_count"])
	require.EqualValues(t, 3, data["workload_count"])
	require.EqualValues(t, 1, data["service_count"])
}

func TestDiscoveryReturns404WhenEmpty(t *testing.T) {
	srv := newTestServer(t, Config{MetadataStore: persistence.NewInMemoryMetadataStore()})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/discovery")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// --- test helpers ---

type testServer struct {
	server *Server
	cancel context.CancelFunc
}

func newTestServer(t *testing.T, cfg Config) testServer {
	t.Helper()
	cfg.ListenAddress = "127.0.0.1:0"
	logger, err := logging.NewWithWriter("api-test", "debug", io.Discard)
	require.NoError(t, err)

	srv, err := NewServer(cfg, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Run(ctx) }()

	// Give the server a moment to start accepting connections.
	time.Sleep(10 * time.Millisecond)

	t.Cleanup(func() {
		cancel()
		if err := <-errCh; err != nil {
			t.Logf("server error: %v", err)
		}
	})

	return testServer{server: srv, cancel: cancel}
}

func doGet(t *testing.T, srv testServer, path string) *http.Response {
	t.Helper()
	resp, err := http.Get("http://" + srv.server.Address() + path)
	require.NoError(t, err)
	return resp
}

func parseResponseData(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var envelope Response
	require.NoError(t, json.Unmarshal(body, &envelope))

	data, ok := envelope.Data.(map[string]any)
	require.True(t, ok, "expected data to be a map, got %T", envelope.Data)
	return data
}
