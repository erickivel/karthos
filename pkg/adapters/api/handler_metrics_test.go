package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
)

func seedMetrics(t *testing.T) *persistence.InMemoryMetricStore {
	t.Helper()
	ms := persistence.NewInMemoryMetricStore()

	entityA := entities.ComputeEntityID(entities.KindPod, "cluster", "default", "pod-a")
	entityB := entities.ComputeEntityID(entities.KindPod, "cluster", "default", "pod-b")
	ts1 := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 3, 30, 11, 0, 0, 0, time.UTC)

	require.NoError(t, ms.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "cpu_usage_millicores", EntityID: entityA, Value: 250, Timestamp: ts1},
		{MetricName: "memory_usage_bytes", EntityID: entityA, Value: 1024000, Timestamp: ts1},
		{MetricName: "cpu_usage_millicores", EntityID: entityB, Value: 500, Timestamp: ts2},
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

func TestQueryMetricsReturnsWithUnit(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics?metric_name=cpu_usage_millicores")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 2, data["total"])

	samples := data["samples"].([]any)
	sample := samples[0].(map[string]any)
	require.NotEmpty(t, sample["unit"])
}

func TestQueryMetricsFilterByEntityID(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	entityA := entities.ComputeEntityID(entities.KindPod, "cluster", "default", "pod-a")
	resp := doGet(t, srv, "/api/v1/metrics?metric_name=cpu_usage_millicores&entity_id="+string(entityA))
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 1, data["total"])
}

func TestQueryMetricsFilterByMetricName(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics?metric_name=cpu_usage_millicores")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 2, data["total"])
}

func TestQueryMetricsFilterByTimeRange(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics?metric_name=cpu_usage_millicores&from=2026-03-30T10:30:00Z&to=2026-03-30T11:30:00Z")
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

	resp := doGet(t, srv, "/api/v1/metrics?metric_name=cpu_usage_millicores&page_size=1")
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

	resp := doGet(t, srv, "/api/v1/metrics/series?metric_name=cpu_usage_millicores")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 1, data["total"])

	series := data["series"].([]any)
	require.Len(t, series, 1)

	s0 := series[0].(map[string]any)
	require.Equal(t, "cpu_usage_millicores", s0["metric_name"])
	require.Equal(t, "millicores", s0["unit"])
	require.Equal(t, "resource", s0["category"])
	samples := s0["samples"].([]any)
	require.Len(t, samples, 2)
}

func TestMetricSeriesFilterByMetricName(t *testing.T) {
	ms := seedMetrics(t)
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics/series?metric_name=memory_usage_bytes")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 1, data["total"])

	series := data["series"].([]any)
	s := series[0].(map[string]any)
	require.Equal(t, "memory_usage_bytes", s["metric_name"])
	require.Equal(t, "bytes", s["unit"])
}

func TestMetricSeriesEmpty(t *testing.T) {
	ms := persistence.NewInMemoryMetricStore()
	srv := newTestServer(t, Config{MetricStore: ms})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/metrics/series?metric_name=cpu_usage_millicores")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 0, data["total"])
	series := data["series"].([]any)
	require.Empty(t, series)
}
