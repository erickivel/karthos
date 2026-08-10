package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/metrics"
)

func serveMetricsRequest(t *testing.T, store persistence.MetricStore, target string) *httptest.ResponseRecorder {
	t.Helper()
	logger, err := logging.NewWithWriter("metrics-test", "debug", io.Discard)
	require.NoError(t, err)
	mux := http.NewServeMux()
	newMetricsHandler(store, logger).RegisterRoutes(mux)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func seedMetrics(t *testing.T) *persistence.InMemoryMetricStore {
	t.Helper()
	ms := persistence.NewInMemoryMetricStore()

	entityA := entities.ComputeEntityID(entities.KindPod, "cluster", "default", "pod-a")
	entityB := entities.ComputeEntityID(entities.KindPod, "cluster", "default", "pod-b")
	ts1 := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 3, 30, 11, 0, 0, 0, time.UTC)

	require.NoError(t, ms.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "k8s.pod.cpu.usage", EntityID: entityA, Value: 250, Timestamp: ts1},
		{MetricName: "k8s.pod.memory.working_set", EntityID: entityA, Value: 1024000, Timestamp: ts1},
		{MetricName: "k8s.pod.cpu.usage", EntityID: entityB, Value: 500, Timestamp: ts2},
	}))
	return ms
}

func TestQueryMetricsRequiresMetricName(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestQueryMetricsAggregationMetadataAndModeAwareUnits(t *testing.T) {
	store := persistence.NewInMemoryMetricStore()
	end := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	labels := map[string]string{"temporality": "cumulative"}
	require.NoError(t, store.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "servicegraph.request.count", EntityID: entities.ComputeEntityID(entities.KindService, "cluster", "default", "api"), Value: 100, Timestamp: end.Add(-100 * time.Second), Labels: labels, Unit: metrics.UnitPerSecond},
		{MetricName: "servicegraph.request.count", EntityID: entities.ComputeEntityID(entities.KindService, "cluster", "default", "api"), Value: 105, Timestamp: end.Add(-80 * time.Second), Labels: labels, Unit: metrics.UnitPerSecond},
		{MetricName: "servicegraph.request.count", EntityID: entities.ComputeEntityID(entities.KindService, "cluster", "default", "api"), Value: 110, Timestamp: end.Add(-50 * time.Second), Labels: labels, Unit: metrics.UnitPerSecond},
		{MetricName: "servicegraph.request.count", EntityID: entities.ComputeEntityID(entities.KindService, "cluster", "default", "api"), Value: 140, Timestamp: end, Labels: labels, Unit: metrics.UnitPerSecond},
	}))

	resp := serveMetricsRequest(t, store, "/api/v1/metrics?metric_name=servicegraph.request.count&aggregation=increase&window=90s&to="+end.Format(time.RFC3339))
	require.Equal(t, http.StatusOK, resp.Code)
	var body PaginatedResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	encoded, err := json.Marshal(body.Data)
	require.NoError(t, err)
	var data map[string]any
	require.NoError(t, json.Unmarshal(encoded, &data))
	queryMeta := data["query"].(map[string]any)
	assert.Equal(t, "increase", queryMeta["aggregation"])
	assert.Equal(t, "1m30s", queryMeta["window"])
	samples := data["samples"].([]any)
	require.Len(t, samples, 1)
	assert.Equal(t, "count", samples[0].(map[string]any)["unit"])

	seriesResp := serveMetricsRequest(t, store, "/api/v1/metrics/series?metric_name=servicegraph.request.count&window=1m&to="+end.Format(time.RFC3339))
	require.Equal(t, http.StatusOK, seriesResp.Code)
	assert.Contains(t, seriesResp.Body.String(), `"aggregation":"window_rate"`)
	assert.Contains(t, seriesResp.Body.String(), `"unit":"per_second"`)
}

func TestQueryMetricsRejectsInvalidAggregationControls(t *testing.T) {
	store := persistence.NewInMemoryMetricStore()
	for _, target := range []string{
		"/api/v1/metrics?metric_name=servicegraph.request.count&aggregation=bogus",
		"/api/v1/metrics?metric_name=servicegraph.request.count&aggregation=increase&window=bad",
		"/api/v1/metrics?metric_name=servicegraph.request.count&aggregation=increase&window=59s",
		"/api/v1/metrics?metric_name=servicegraph.request.count&aggregation=increase&window=61m",
		"/api/v1/metrics?metric_name=servicegraph.request.count&aggregation=latest&window=1m",
		"/api/v1/metrics?metric_name=k8s.pod.cpu.usage&aggregation=latest",
		"/api/v1/metrics?metric_name=k8s.pod.cpu.usage&window=1m",
	} {
		resp := serveMetricsRequest(t, store, target)
		assert.Equal(t, http.StatusBadRequest, resp.Code, target)
	}
}

func TestQueryMetricsReturnsWithUnit(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics?metric_name=k8s.pod.cpu.usage")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 2, data["total"])

	samples := data["samples"].([]any)
	sample := samples[0].(map[string]any)
	require.NotEmpty(t, sample["unit"])
}

func TestQueryMetricsReturnsReadablePodStatusReason(t *testing.T) {
	store := persistence.NewInMemoryMetricStore()
	require.NoError(t, store.Write(context.Background(), []metrics.MetricSample{{
		MetricName: "k8s.pod.status_reason",
		EntityID:   entities.ComputeEntityID(entities.KindPod, "cluster", "default", "api"),
		Value:      6,
		Timestamp:  time.Now().UTC(),
		Unit:       metrics.UnitCount,
	}}))

	resp := serveMetricsRequest(t, store, "/api/v1/metrics?metric_name=k8s.pod.status_reason")
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), `"value_label":"unknown"`)
}

func TestQueryMetricsFilterByEntityID(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	entityA := entities.ComputeEntityID(entities.KindPod, "cluster", "default", "pod-a")
	resp := doGet(t, srv, "/api/v1/metrics?metric_name=k8s.pod.cpu.usage&entity_id="+string(entityA))
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 1, data["total"])
}

func TestQueryMetricsFilterByEdgeIDs(t *testing.T) {
	ms := persistence.NewInMemoryMetricStore()
	now := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	sourceID := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-a")
	targetID := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-b")
	otherTargetID := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-c")
	graphID := entities.ComputeEntityID(entities.KindService, "cluster", "default", "graph")
	edgeLabels := func(target entities.EntityID) map[string]string {
		return map[string]string{
			"edge_client_id": string(sourceID),
			"edge_server_id": string(target),
			"temporality":    "cumulative",
		}
	}
	require.NoError(t, ms.Write(context.Background(), []metrics.MetricSample{
		{
			MetricName: "servicegraph.request.duration_sum", EntityID: graphID,
			Value: 100, Timestamp: now.Add(-time.Minute), Labels: edgeLabels(targetID),
		},
		{
			MetricName: "servicegraph.request.duration_sum", EntityID: graphID,
			Value: 112, Timestamp: now, Labels: edgeLabels(targetID),
		},
		{
			MetricName: "servicegraph.request.duration_count", EntityID: graphID,
			Value: 10, Timestamp: now.Add(-time.Minute), Labels: edgeLabels(targetID),
		},
		{
			MetricName: "servicegraph.request.duration_count", EntityID: graphID,
			Value: 11, Timestamp: now, Labels: edgeLabels(targetID),
		},
		{
			MetricName: "servicegraph.request.duration_sum", EntityID: graphID,
			Value: 200, Timestamp: now.Add(-time.Minute), Labels: edgeLabels(otherTargetID),
		},
		{
			MetricName: "servicegraph.request.duration_sum", EntityID: graphID,
			Value: 215, Timestamp: now, Labels: edgeLabels(otherTargetID),
		},
		{
			MetricName: "servicegraph.request.duration_count", EntityID: graphID,
			Value: 20, Timestamp: now.Add(-time.Minute), Labels: edgeLabels(otherTargetID),
		},
		{
			MetricName: "servicegraph.request.duration_count", EntityID: graphID,
			Value: 21, Timestamp: now, Labels: edgeLabels(otherTargetID),
		},
	}))
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics?metric_name=servicegraph.request.duration&edge_source_id="+string(sourceID)+"&edge_target_id="+string(targetID))
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 1, data["total"])
	sample := data["samples"].([]any)[0].(map[string]any)
	labels := sample["labels"].(map[string]any)
	require.Equal(t, string(sourceID), labels["edge_client_id"])
	require.Equal(t, string(targetID), labels["edge_server_id"])
}

func TestQueryMetricsFilterByMetricName(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics?metric_name=k8s.pod.cpu.usage")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 2, data["total"])
}

func TestQueryMetricsFilterByTimeRange(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics?metric_name=k8s.pod.cpu.usage&from=2026-03-30T10:30:00Z&to=2026-03-30T11:30:00Z")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 1, data["total"])
}

func TestQueryMetricsBadTimeReturns400(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics?from=not-a-time")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestQueryMetricsPagination(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics?metric_name=k8s.pod.cpu.usage&page_size=1")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data, nextToken := parsePaginatedDataWithToken(t, resp)
	require.EqualValues(t, 2, data["total"])
	samples := data["samples"].([]any)
	require.Len(t, samples, 1)
	require.NotEmpty(t, nextToken)
}

func TestCatalogReturnsAllMetrics(t *testing.T) {
	srv := newTestServer(t, Config{})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics/catalog")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	total := data["total"].(float64)
	require.Greater(t, total, float64(0))
	require.Equal(t, total, float64(len(metrics.DefaultCatalog())))
}

func TestCatalogFilterByCategory(t *testing.T) {
	srv := newTestServer(t, Config{})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics/catalog?category=latency")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	require.EqualValues(t, 2, data["total"])
}

func TestCatalogReturnsMetricScopes(t *testing.T) {
	srv := newTestServer(t, Config{})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics/catalog")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseResponseData(t, resp)
	metricsList := data["metrics"].([]any)

	var requestRate map[string]any
	for _, raw := range metricsList {
		entry := raw.(map[string]any)
		if entry["name"] == "servicegraph.request.count" {
			requestRate = entry
			break
		}
	}

	require.NotNil(t, requestRate)
	require.Equal(t, []any{"entity", "edge"}, requestRate["scopes"])
}

func TestMetricSeriesRequiresMetricName(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics/series")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestMetricSeriesGrouped(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics/series?metric_name=k8s.pod.cpu.usage")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 2, data["total"])

	series := data["series"].([]any)
	require.Len(t, series, 2)

	s0 := series[0].(map[string]any)
	require.Equal(t, "k8s.pod.cpu.usage", s0["metric_name"])
	require.Equal(t, "millicores", s0["unit"])
	require.Equal(t, "resource", s0["category"])
	samples := s0["samples"].([]any)
	require.Len(t, samples, 1)
}

func TestMetricSeriesFilterByMetricName(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics/series?metric_name=k8s.pod.memory.working_set")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 1, data["total"])

	series := data["series"].([]any)
	s := series[0].(map[string]any)
	require.Equal(t, "k8s.pod.memory.working_set", s["metric_name"])
	require.Equal(t, "bytes", s["unit"])
}

func TestMetricSeriesEmpty(t *testing.T) {
	ms := persistence.NewInMemoryMetricStore()
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics/series?metric_name=k8s.pod.cpu.usage")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 0, data["total"])
	series := data["series"].([]any)
	require.Empty(t, series)
}

func TestMetricSeriesGroupsEdgeMetricsPerEdge(t *testing.T) {
	ms := persistence.NewInMemoryMetricStore()
	now := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	sourceID := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-a")
	targetB := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-b")
	targetC := entities.ComputeEntityID(entities.KindService, "cluster", "default", "svc-c")
	graphID := entities.ComputeEntityID(entities.KindService, "cluster", "default", "graph")
	edgeLabels := func(target entities.EntityID) map[string]string {
		return map[string]string{
			"edge_client_id": string(sourceID),
			"edge_server_id": string(target),
			"temporality":    "cumulative",
		}
	}
	require.NoError(t, ms.Write(context.Background(), []metrics.MetricSample{
		{
			MetricName: "servicegraph.request.duration_sum", EntityID: graphID,
			Value: 100, Timestamp: now.Add(-time.Minute), Labels: edgeLabels(targetB),
		},
		{
			MetricName: "servicegraph.request.duration_sum", EntityID: graphID,
			Value: 112, Timestamp: now, Labels: edgeLabels(targetB),
		},
		{
			MetricName: "servicegraph.request.duration_count", EntityID: graphID,
			Value: 10, Timestamp: now.Add(-time.Minute), Labels: edgeLabels(targetB),
		},
		{
			MetricName: "servicegraph.request.duration_count", EntityID: graphID,
			Value: 11, Timestamp: now, Labels: edgeLabels(targetB),
		},
		{
			MetricName: "servicegraph.request.duration_sum", EntityID: graphID,
			Value: 200, Timestamp: now.Add(-time.Minute), Labels: edgeLabels(targetC),
		},
		{
			MetricName: "servicegraph.request.duration_sum", EntityID: graphID,
			Value: 215, Timestamp: now, Labels: edgeLabels(targetC),
		},
		{
			MetricName: "servicegraph.request.duration_count", EntityID: graphID,
			Value: 20, Timestamp: now.Add(-time.Minute), Labels: edgeLabels(targetC),
		},
		{
			MetricName: "servicegraph.request.duration_count", EntityID: graphID,
			Value: 21, Timestamp: now, Labels: edgeLabels(targetC),
		},
	}))
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics/series?metric_name=servicegraph.request.duration&edge_source_id="+string(sourceID))
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 2, data["total"])
	series := data["series"].([]any)
	require.Len(t, series, 2)
}

func TestCatalogStatsZeroFillsAndJoins(t *testing.T) {
	ms := persistence.NewInMemoryMetricStore()
	// Pick a metric whose catalog declaration has a single source (kubelet),
	// and one with multiple sources (servicegraph.request.count: mesh.istio +
	// traces.servicegraph). Write samples for the kubelet metric, leave the
	// traces.servicegraph entry of the second metric empty to exercise zero-fill.
	now := time.Now().UTC()
	require.NoError(t, ms.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "k8s.pod.cpu.usage", EntityID: "e1", Value: 0.5, Timestamp: now.Add(-10 * time.Minute), Labels: map[string]string{"source": "kubelet"}},
		{MetricName: "k8s.pod.cpu.usage", EntityID: "e1", Value: 0.6, Timestamp: now.Add(-5 * time.Minute), Labels: map[string]string{"source": "kubelet"}},
		{MetricName: "servicegraph.request.count", EntityID: "e2", Value: 1, Timestamp: now.Add(-1 * time.Minute), Labels: map[string]string{"source": "mesh.istio"}},
	}))

	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics/catalog/stats")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope struct {
		Data struct {
			LookbackSeconds int `json:"lookback_seconds"`
			Stats           []struct {
				MetricName string `json:"metric_name"`
				BySource   []struct {
					SourceID    string  `json:"source_id"`
					Priority    int     `json:"priority"`
					SampleCount int64   `json:"sample_count"`
					LastSeenAt  *string `json:"last_seen_at"`
				} `json:"by_source"`
			} `json:"stats"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	body := envelope.Data
	require.Equal(t, 3600, body.LookbackSeconds)

	byName := map[string]int{}
	for i, e := range body.Stats {
		byName[e.MetricName] = i
	}

	// Catalog has 100+ entries; just verify the two we exercised plus zero-fill.
	cpu := body.Stats[byName["k8s.pod.cpu.usage"]]
	require.Len(t, cpu.BySource, 1)
	assert.Equal(t, "kubelet", cpu.BySource[0].SourceID)
	assert.EqualValues(t, 2, cpu.BySource[0].SampleCount)
	require.NotNil(t, cpu.BySource[0].LastSeenAt)

	traffic := body.Stats[byName["servicegraph.request.count"]]
	require.Len(t, traffic.BySource, 2)
	// mesh.istio populated, traces.servicegraph zero-filled.
	var meshCount, tracesCount int64
	var meshSeen, tracesSeen *string
	for _, s := range traffic.BySource {
		switch s.SourceID {
		case "mesh.istio":
			meshCount = s.SampleCount
			meshSeen = s.LastSeenAt
		case "traces.servicegraph":
			tracesCount = s.SampleCount
			tracesSeen = s.LastSeenAt
		}
	}
	assert.EqualValues(t, 1, meshCount)
	require.NotNil(t, meshSeen)
	assert.EqualValues(t, 0, tracesCount)
	assert.Nil(t, tracesSeen)
}
