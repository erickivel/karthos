package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

type recordingMetricStore struct {
	samples map[string][]metrics.MetricSample
	errors  map[string]error
	queries []query.MetricQuery
}

type failingGraphStore struct {
	persistence.GraphStore
	err error
}

func (s failingGraphStore) GetLatest(context.Context) (*topology.Snapshot, error) {
	return nil, s.err
}

type failingMetadataStore struct {
	persistence.MetadataStore
	err error
}

func (s failingMetadataStore) GetDiscovery(context.Context) (*profile.ClusterProfile, error) {
	return nil, s.err
}

func (s *recordingMetricStore) Write(_ context.Context, samples []metrics.MetricSample) error {
	if s.samples == nil {
		s.samples = map[string][]metrics.MetricSample{}
	}
	for _, sample := range samples {
		s.samples[sample.MetricName] = append(s.samples[sample.MetricName], sample)
	}
	return nil
}

func (s *recordingMetricStore) Query(_ context.Context, q query.MetricQuery) ([]metrics.MetricSample, error) {
	s.queries = append(s.queries, q)
	if err := s.errors[q.MetricName]; err != nil {
		return nil, err
	}
	return s.samples[q.MetricName], nil
}

func (*recordingMetricStore) QueryCatalogStats(context.Context, time.Duration) ([]persistence.CatalogStat, error) {
	return nil, nil
}

func TestLatestSamplePerEntity(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		samples  []metrics.MetricSample
		expected map[entities.EntityID]float64
	}{
		{
			name: "keeps newest sample per entity",
			samples: []metrics.MetricSample{
				{EntityID: "node-a", Value: 0, Timestamp: base},
				{EntityID: "node-a", Value: 1, Timestamp: base.Add(30 * time.Second)},
				{EntityID: "node-b", Value: 1, Timestamp: base.Add(15 * time.Second)},
			},
			expected: map[entities.EntityID]float64{
				"node-a": 1,
				"node-b": 1,
			},
		},
		{
			name: "ignores older duplicate samples",
			samples: []metrics.MetricSample{
				{EntityID: "pod-a", Value: 2, Timestamp: base.Add(45 * time.Second)},
				{EntityID: "pod-a", Value: 1, Timestamp: base},
			},
			expected: map[entities.EntityID]float64{
				"pod-a": 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, latestSamplePerEntity(tt.samples))
		})
	}
}

func freshGraphStore(t *testing.T) persistence.GraphStore {
	t.Helper()
	gs := persistence.NewInMemoryGraphStore()
	putMaterializedGraph(t, gs, topology.Snapshot{
		Version:   schema.CurrentVersion,
		CreatedAt: time.Now(),
		Graph:     topology.Graph{Timestamp: time.Now()},
	})
	return gs
}

func putMaterializedGraph(t *testing.T, store *persistence.InMemoryGraphStore, snapshot topology.Snapshot) {
	t.Helper()
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}
	require.NoError(t, store.PutStructuralSnapshot(context.Background(), snapshot))
	_, err := store.Materialize(context.Background(), snapshot.CreatedAt)
	require.NoError(t, err)
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

func TestStatusDegradesWithSafeReasonsWhenReadsFail(t *testing.T) {
	graphFailure := errors.New("database connection refused")
	metadataFailure := errors.New("profile decode details")
	srv := newTestServer(t, Config{
		MetricStore: persistence.NewInMemoryMetricStore(),
		GraphStore: failingGraphStore{
			GraphStore: persistence.NewInMemoryGraphStore(),
			err:        graphFailure,
		},
		MetadataStore: failingMetadataStore{
			MetadataStore: persistence.NewInMemoryMetadataStore(),
			err:           metadataFailure,
		},
		EntityRegistry: persistence.NewInMemoryEntityRegistry(),
	})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	data := parseResponseData(t, resp)
	require.Equal(t, "degraded", data["system_status"])
	require.Equal(t, true, data["data_stale"])
	reasons := data["reasons"].([]any)
	require.Contains(t, reasons, "failed to read latest graph")
	require.Contains(t, reasons, "failed to read capability profile")
	require.NotContains(t, reasons, graphFailure.Error())
	require.NotContains(t, reasons, metadataFailure.Error())
	components := data["components"].(map[string]any)
	require.Equal(t, "available", components["graph_store"])
	require.Equal(t, "available", components["metadata_store"])
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

func TestStatusHealthSummaryWithMetrics(t *testing.T) {
	ms := persistence.NewInMemoryMetricStore()
	now := time.Now().UTC()
	gs := persistence.NewInMemoryGraphStore()
	putMaterializedGraph(t, gs, topology.Snapshot{
		Version:   schema.CurrentVersion,
		CreatedAt: now,
		Graph: topology.Graph{
			Timestamp: now,
			Nodes: []topology.Node{
				{ID: "node-1", Kind: entities.KindNode},
				{ID: "node-2", Kind: entities.KindNode},
				{ID: "node-3", Kind: entities.KindNode},
				{ID: "pod-1", Kind: entities.KindPod},
				{ID: "pod-2", Kind: entities.KindPod},
				{ID: "pod-3", Kind: entities.KindPod},
				{ID: "pod-4", Kind: entities.KindPod},
				{ID: "pod-5", Kind: entities.KindPod},
				{ID: "pod-6", Kind: entities.KindPod}, // no health sample → unknown
			},
		},
	})

	// Pod health buckets are derived from the direct Kubernetes pod phase.
	require.NoError(t, ms.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "k8s.pod.phase", EntityID: "pod-1", Value: 2, Timestamp: now},
		{MetricName: "k8s.pod.phase", EntityID: "pod-2", Value: 2, Timestamp: now},
		{MetricName: "k8s.pod.phase", EntityID: "pod-3", Value: 1, Timestamp: now},
		{MetricName: "k8s.pod.phase", EntityID: "pod-4", Value: 4, Timestamp: now},
		{MetricName: "k8s.pod.phase", EntityID: "pod-5", Value: 5, Timestamp: now},
		// pod-6 has no sample → falls through to unknown.
	}))

	// Node condition metrics drive the node bucket.
	require.NoError(t, ms.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "k8s.node.condition.ready", EntityID: "node-1", Value: 1, Timestamp: now},
		{MetricName: "k8s.node.condition.ready", EntityID: "node-2", Value: 1, Timestamp: now},
		{MetricName: "k8s.node.condition.ready", EntityID: "node-3", Value: 0, Timestamp: now},
	}))

	srv := newTestServer(t, Config{
		MetricStore:    ms,
		GraphStore:     gs,
		MetadataStore:  persistence.NewInMemoryMetadataStore(),
		EntityRegistry: persistence.NewInMemoryEntityRegistry(),
	})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data := parseResponseData(t, resp)

	hs, ok := data["health_summary"].(map[string]any)
	require.True(t, ok, "expected health_summary in status response")
	require.Equal(t, "complete", hs["status"])

	nodes := hs["nodes"].(map[string]any)
	require.Equal(t, "complete", nodes["status"])
	require.EqualValues(t, 2, nodes["healthy"])
	require.EqualValues(t, 1, nodes["unhealthy"])
	require.EqualValues(t, 0, nodes["unknown"])

	pods := hs["pods"].(map[string]any)
	require.Equal(t, "complete", pods["status"])
	require.EqualValues(t, 2, pods["healthy"])
	require.EqualValues(t, 1, pods["pending"])
	require.EqualValues(t, 1, pods["failed"])
	require.EqualValues(t, 2, pods["unknown"]) // pod-5 (explicit) + pod-6 (no sample)
	require.EqualValues(t, 0, pods["succeeded"])

	// Unhealthy node should cause degraded status.
	require.Equal(t, "degraded", data["system_status"])
}

func TestStatusHealthSummaryUnavailableWithoutMetricStore(t *testing.T) {
	now := time.Now().UTC()
	gs := persistence.NewInMemoryGraphStore()
	putMaterializedGraph(t, gs, topology.Snapshot{
		Version: schema.CurrentVersion, CreatedAt: now,
		Graph: topology.Graph{Timestamp: now, Nodes: []topology.Node{
			{ID: "node-1", Kind: entities.KindNode},
			{ID: "pod-1", Kind: entities.KindPod},
			{ID: "pod-2", Kind: entities.KindPod},
		}},
	})
	srv := newTestServer(t, Config{
		GraphStore:     gs,
		MetadataStore:  persistence.NewInMemoryMetadataStore(),
		EntityRegistry: persistence.NewInMemoryEntityRegistry(),
	})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data := parseResponseData(t, resp)
	hs := data["health_summary"].(map[string]any)
	require.Equal(t, "unavailable", hs["status"])
	nodes := hs["nodes"].(map[string]any)
	require.Equal(t, "unavailable", nodes["status"])
	require.EqualValues(t, 1, nodes["unknown"])
	pods := hs["pods"].(map[string]any)
	require.Equal(t, "unavailable", pods["status"])
	require.EqualValues(t, 2, pods["unknown"])
}

func TestStatusBulkHealthQueriesDoNotEmbedEntityIDs(t *testing.T) {
	now := time.Now().UTC()
	nodes := make([]topology.Node, 0, 1001)
	nodes = append(nodes, topology.Node{ID: "node-1", Kind: entities.KindNode})
	for i := 0; i < 1000; i++ {
		nodes = append(nodes, topology.Node{ID: entities.EntityID("pod-" + strconv.Itoa(i+1)), Kind: entities.KindPod})
	}
	gs := persistence.NewInMemoryGraphStore()
	putMaterializedGraph(t, gs, topology.Snapshot{
		Version: schema.CurrentVersion, CreatedAt: now, Graph: topology.Graph{Timestamp: now, Nodes: nodes},
	})
	ms := &recordingMetricStore{samples: map[string][]metrics.MetricSample{}}
	srv := newTestServer(t, Config{MetricStore: ms, GraphStore: gs, MetadataStore: persistence.NewInMemoryMetadataStore(), EntityRegistry: persistence.NewInMemoryEntityRegistry()})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, ms.queries, 2)
	for _, q := range ms.queries {
		require.Empty(t, q.EntityIDs, "snapshot-wide health queries must have constant query text")
	}
}

func TestStatusHealthSummaryDoesNotQueryContainerHealth(t *testing.T) {
	now := time.Now().UTC()
	gs := persistence.NewInMemoryGraphStore()
	putMaterializedGraph(t, gs, topology.Snapshot{
		Version: schema.CurrentVersion, CreatedAt: now,
		Graph: topology.Graph{Timestamp: now, Nodes: []topology.Node{
			{ID: "node-1", Kind: entities.KindNode},
			{ID: "pod-1", Kind: entities.KindPod},
			{ID: "container-1", Kind: entities.KindContainer},
		}},
	})
	ms := &recordingMetricStore{samples: map[string][]metrics.MetricSample{}}
	srv := newTestServer(t, Config{MetricStore: ms, GraphStore: gs, MetadataStore: persistence.NewInMemoryMetadataStore(), EntityRegistry: persistence.NewInMemoryEntityRegistry()})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, ms.queries, 2)
	queriedMetrics := []string{ms.queries[0].MetricName, ms.queries[1].MetricName}
	require.ElementsMatch(t, []string{"k8s.node.condition.ready", "k8s.pod.phase"}, queriedMetrics)
}

func TestStatusDegradesWhenPodHealthBranchFails(t *testing.T) {
	now := time.Now().UTC()
	gs := persistence.NewInMemoryGraphStore()
	putMaterializedGraph(t, gs, topology.Snapshot{
		Version: schema.CurrentVersion, CreatedAt: now,
		Graph: topology.Graph{Timestamp: now, Nodes: []topology.Node{
			{ID: "node-1", Kind: entities.KindNode}, {ID: "pod-1", Kind: entities.KindPod},
		}},
	})
	ms := &recordingMetricStore{
		samples: map[string][]metrics.MetricSample{
			"k8s.node.condition.ready": {{MetricName: "k8s.node.condition.ready", EntityID: "node-1", Value: 1, Timestamp: now}},
		},
		errors: map[string]error{"k8s.pod.phase": errors.New("query too long")},
	}
	srv := newTestServer(t, Config{MetricStore: ms, GraphStore: gs, MetadataStore: persistence.NewInMemoryMetadataStore(), EntityRegistry: persistence.NewInMemoryEntityRegistry()})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	data := parseResponseData(t, resp)
	require.Equal(t, "degraded", data["system_status"])
	require.Contains(t, data["reasons"], "pod health summary unavailable")
	hs := data["health_summary"].(map[string]any)
	require.Equal(t, "partial", hs["status"])
	require.Equal(t, "complete", hs["nodes"].(map[string]any)["status"])
	pods := hs["pods"].(map[string]any)
	require.Equal(t, "unavailable", pods["status"])
	require.EqualValues(t, 1, pods["unknown"])
}

func TestDiscoveryReturnsCapabilityProfile(t *testing.T) {
	ms := persistence.NewInMemoryMetadataStore()
	require.NoError(t, ms.PutDiscovery(context.Background(), profile.ClusterProfile{
		ClusterID:   entities.ComputeEntityID(entities.KindCluster, "test-cluster"),
		ClusterName: "test-cluster",
		Capabilities: []profile.SourceCapability{
			{SourceID: "kubernetes.structure", Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"structure"}},
			{SourceID: "prometheus.app", Kind: profile.SourceKindDirect, Status: profile.CapabilityUnavailable},
		},
	}))

	srv := newTestServer(t, Config{MetadataStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/discovery")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	require.Equal(t, "test-cluster", data["cluster_name"])
	capabilities := data["capabilities"].([]any)
	require.Len(t, capabilities, 2)
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
	logger, err := logging.NewWithWriter("api-test", "info", io.Discard)
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
