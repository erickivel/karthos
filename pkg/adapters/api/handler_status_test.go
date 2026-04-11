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
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func freshGraphStore(t *testing.T) persistence.GraphStore {
	t.Helper()
	gs := persistence.NewInMemoryGraphStore()
	require.NoError(t, gs.PutSnapshot(context.Background(), topology.Snapshot{
		Version:   schema.CurrentVersion,
		CreatedAt: time.Now(),
		Graph:     topology.Graph{Timestamp: time.Now()},
	}))
	return gs
}

func TestStatusNominalWithFreshSnapshot(t *testing.T) {
	srv := newTestServer(t, Config{
		MetricStore:    persistence.NewInMemoryMetricStore(),
		GraphStore:     freshGraphStore(t),
		MetadataStore:  persistence.NewInMemoryMetadataStore(),
		EntityRegistry: persistence.NewInMemoryEntityRegistry(),
	})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	require.Equal(t, "nominal", data["system_status"])
	require.Equal(t, false, data["data_stale"])
	require.NotNil(t, data["snapshot_freshness_seconds"])
	require.NotNil(t, data["last_snapshot_at"])

	reasons, ok := data["reasons"].([]any)
	require.True(t, ok)
	require.Empty(t, reasons)

	components := data["components"].(map[string]any)
	require.Equal(t, "available", components["metric_store"])
	require.Equal(t, "available", components["graph_store"])
	require.Equal(t, "available", components["metadata_store"])
	require.Equal(t, "available", components["entity_registry"])
}

func TestStatusDegradedWhenNoSnapshot(t *testing.T) {
	srv := newTestServer(t, Config{
		MetricStore:    persistence.NewInMemoryMetricStore(),
		GraphStore:     persistence.NewInMemoryGraphStore(),
		MetadataStore:  persistence.NewInMemoryMetadataStore(),
		EntityRegistry: persistence.NewInMemoryEntityRegistry(),
	})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data := parseResponseData(t, resp)
	require.Equal(t, "degraded", data["system_status"])
	require.Equal(t, true, data["data_stale"])
}

func TestStatusDisconnectedWhenGraphStoreNil(t *testing.T) {
	srv := newTestServer(t, Config{
		MetricStore:    persistence.NewInMemoryMetricStore(),
		MetadataStore:  persistence.NewInMemoryMetadataStore(),
		EntityRegistry: persistence.NewInMemoryEntityRegistry(),
	})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data := parseResponseData(t, resp)
	require.Equal(t, "disconnected", data["system_status"])

	reasons := data["reasons"].([]any)
	require.Contains(t, reasons, "graph_store unavailable")
}

func TestStatusDegradedWhenMetricStoreNil(t *testing.T) {
	srv := newTestServer(t, Config{
		GraphStore:     freshGraphStore(t),
		MetadataStore:  persistence.NewInMemoryMetadataStore(),
		EntityRegistry: persistence.NewInMemoryEntityRegistry(),
	})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data := parseResponseData(t, resp)
	require.Equal(t, "degraded", data["system_status"])

	reasons := data["reasons"].([]any)
	require.Contains(t, reasons, "metric_store unavailable")
}

func TestStatusIncludesDiscoveryTimestamp(t *testing.T) {
	ms := persistence.NewInMemoryMetadataStore()
	discoveredAt := time.Date(2026, 3, 31, 1, 35, 36, 0, time.UTC)
	require.NoError(t, ms.PutDiscovery(context.Background(), profile.ClusterProfile{
		ClusterName:  "test",
		DiscoveredAt: discoveredAt,
	}))

	srv := newTestServer(t, Config{
		MetricStore:    persistence.NewInMemoryMetricStore(),
		GraphStore:     freshGraphStore(t),
		MetadataStore:  ms,
		EntityRegistry: persistence.NewInMemoryEntityRegistry(),
	})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data := parseResponseData(t, resp)
	require.NotNil(t, data["last_discovery_at"])
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

func parsePaginatedData(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var envelope PaginatedResponse
	require.NoError(t, json.Unmarshal(body, &envelope))

	data, ok := envelope.Data.(map[string]any)
	require.True(t, ok, "expected data to be a map, got %T", envelope.Data)
	return data
}

func parsePaginatedDataWithToken(t *testing.T, resp *http.Response) (map[string]any, string) {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var envelope PaginatedResponse
	require.NoError(t, json.Unmarshal(body, &envelope))

	data, ok := envelope.Data.(map[string]any)
	require.True(t, ok, "expected data to be a map, got %T", envelope.Data)
	return data, envelope.NextToken
}
