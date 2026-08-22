package victoriametrics

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
)

func TestSamplesToPrometheusText(t *testing.T) {
	ts := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	samples := []metrics.MetricSample{
		{
			MetricName: "k8s.pod.cpu.usage",
			EntityID:   entities.EntityID("abc123"),
			Value:      42.5,
			Timestamp:  ts,
			Labels:     map[string]string{"source": "otlp"},
			Unit:       metrics.UnitMillicores,
		},
	}

	got := string(samplesToPrometheusText(samples))

	assert.Contains(t, got, "k8s_pod_cpu_usage{")
	assert.Contains(t, got, `entity_id="abc123"`)
	assert.Contains(t, got, `source="otlp"`)
	assert.Contains(t, got, `oksg_unit="millicores"`)
	assert.Contains(t, got, "42.5")
	// Verify timestamp is present as milliseconds.
	expectedTsMs := strconv.FormatInt(ts.UnixMilli(), 10)
	assert.Contains(t, got, expectedTsMs)
}

func TestSamplesToPrometheusText_PersistsHistogramComponentUnits(t *testing.T) {
	ts := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	samples := []metrics.MetricSample{
		{MetricName: "http.server.request.duration_sum", EntityID: "svc1", Value: 1_500_000_000, Timestamp: ts, Unit: metrics.UnitNanoseconds},
		{MetricName: "http.server.request.duration_count", EntityID: "svc1", Value: 3, Timestamp: ts, Unit: metrics.UnitCount},
		{MetricName: "servicegraph.request.body.size_sum", EntityID: "edge1", Value: 4096, Timestamp: ts, Unit: metrics.UnitBytes},
		{MetricName: "servicegraph.request.body.size_count", EntityID: "edge1", Value: 5, Timestamp: ts, Unit: metrics.UnitCount},
	}

	got := string(samplesToPrometheusText(samples))
	assert.Contains(t, got, `http_server_request_duration_sum{entity_id="svc1",oksg_unit="nanoseconds"}`)
	assert.Contains(t, got, `http_server_request_duration_count{entity_id="svc1",oksg_unit="count"}`)
	assert.Contains(t, got, `servicegraph_request_body_size_sum{entity_id="edge1",oksg_unit="bytes"}`)
	assert.Contains(t, got, `servicegraph_request_body_size_count{entity_id="edge1",oksg_unit="count"}`)
}

func TestSamplesToPrometheusText_EscapesLabelValues(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	samples := []metrics.MetricSample{
		{
			MetricName: "test_metric",
			EntityID:   entities.EntityID("id1"),
			Value:      1.0,
			Timestamp:  ts,
			Labels:     map[string]string{"desc": `has "quotes" and\back`},
		},
	}

	got := string(samplesToPrometheusText(samples))
	assert.Contains(t, got, `has \"quotes\" and\\back`)
}

func TestSamplesToPrometheusText_PreservesSourceDimensions(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	samples := []metrics.MetricSample{
		{
			MetricName: "http.server.request.count",
			EntityID:   entities.EntityID("svc1"),
			Value:      7,
			Timestamp:  ts,
			Labels: map[string]string{
				"source":           "prometheus.app",
				"namespace":        "oksg-demo",
				"service":          "gateway-go",
				"pod":              "gateway-go-abc123",
				"method":           "GET",
				"http_status_code": "500",
			},
		},
	}

	got := string(samplesToPrometheusText(samples))

	assert.Contains(t, got, `source="prometheus.app"`)
	assert.Contains(t, got, `namespace="oksg-demo"`)
	assert.Contains(t, got, `service="gateway-go"`)
	assert.Contains(t, got, `pod="gateway-go-abc123"`)
	assert.Contains(t, got, `method="GET"`)
	assert.Contains(t, got, `http_status_code="500"`)
}

func TestSamplesToPrometheusText_PreservesDistinctServicegraphContexts(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	baseLabels := map[string]string{
		"source":          "traces.servicegraph",
		"edge_client":     "service-a",
		"edge_client_id":  "svc-a",
		"edge_server":     "service-b",
		"edge_server_id":  "svc-b",
		"connection_type": "database",
		"virtual_node":    "server",
		"failed":          "false",
	}
	labelsFor := func(method string) map[string]string {
		labels := make(map[string]string, len(baseLabels)+2)
		for key, value := range baseLabels {
			labels[key] = value
		}
		labels["client_http_method"] = method
		labels["server_http_method"] = method
		return labels
	}
	samples := []metrics.MetricSample{
		{MetricName: "servicegraph.request.count", EntityID: "svc-a", Value: 4, Timestamp: ts, Labels: labelsFor("GET")},
		{MetricName: "servicegraph.request.count", EntityID: "svc-a", Value: 7, Timestamp: ts, Labels: labelsFor("POST")},
	}

	got := string(samplesToPrometheusText(samples))
	lines := strings.Split(strings.TrimSpace(got), "\n")
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0]+lines[1], `failed="false"`)
	assert.Contains(t, lines[0]+lines[1], `connection_type="database"`)
	assert.Contains(t, lines[0]+lines[1], `virtual_node="server"`)
	assert.Contains(t, got, `client_http_method="GET"`)
	assert.Contains(t, got, `client_http_method="POST"`)
	assert.NotEqual(t, lines[0], lines[1])
}

func TestSamplesToPrometheusText_Empty(t *testing.T) {
	got := samplesToPrometheusText(nil)
	assert.Empty(t, got)
}

func TestBuildPromQL(t *testing.T) {
	tests := []struct {
		name     string
		query    query.MetricQuery
		expected string
	}{
		{
			name:     "empty metric name",
			query:    query.MetricQuery{},
			expected: "",
		},
		{
			name:     "metric name only",
			query:    query.MetricQuery{MetricName: "k8s.pod.cpu.usage"},
			expected: "k8s_pod_cpu_usage",
		},
		{
			name: "metric name with single entity ID",
			query: query.MetricQuery{
				MetricName: "k8s.pod.cpu.usage",
				EntityIDs:  []entities.EntityID{"abc123"},
			},
			expected: `k8s_pod_cpu_usage{entity_id=~"^(?:abc123)$"}`,
		},
		{
			name: "metric name with multiple entity IDs",
			query: query.MetricQuery{
				MetricName: "k8s.pod.memory.working_set",
				EntityIDs:  []entities.EntityID{"abc", "def"},
			},
			expected: `k8s_pod_memory_working_set{entity_id=~"^(?:abc|def)$"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, buildPromQL(tt.query))
		})
	}
}

func TestBuildPromQLDefaultCountersUseExactWindowAndPreserveFilters(t *testing.T) {
	tests := []struct {
		name     string
		query    query.MetricQuery
		selector string
	}{
		{
			name:     "unfiltered request count",
			query:    query.MetricQuery{MetricName: "servicegraph.request.count"},
			selector: "servicegraph_request_count",
		},
		{
			name: "entity-filtered host counter",
			query: query.MetricQuery{
				MetricName: "system.disk.operations",
				EntityIDs:  []entities.EntityID{"node-1"},
			},
			selector: `system_disk_operations{entity_id=~"^(?:node-1)$",temporality!="delta"}`,
		},
		{
			name: "edge-filtered error count",
			query: query.MetricQuery{
				MetricName:   "servicegraph.request.error.count",
				EdgeSourceID: entities.EntityID("svc-a"),
				EdgeTargetID: entities.EntityID("svc-b"),
			},
			selector: `servicegraph_request_error_count{edge_client_id="svc-a",edge_server_id="svc-b",temporality!="delta"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPromQL(tt.query)
			assert.Contains(t, got, "increase(")
			assert.Contains(t, got, tt.selector)
			assert.Contains(t, got, "count_over_time(")
			assert.Contains(t, got, "tfirst_over_time(")
			assert.Contains(t, got, `temporality="delta"`)
			assert.Contains(t, got, "/ 60)")
		})
	}
}

func TestBuildPromQLExplicitCounterModes(t *testing.T) {
	base := query.MetricQuery{MetricName: "servicegraph.request.count", Window: 90 * time.Second}

	base.Aggregation = metrics.AggregationIncrease
	increase := buildPromQL(base)
	assert.Contains(t, increase, "increase(servicegraph_request_count")
	assert.Contains(t, increase, "[90s]")
	assert.Contains(t, increase, "[180s]")
	assert.Contains(t, increase, ">= 90")
	assert.Contains(t, increase, "sum_over_time(")

	base.Aggregation = metrics.AggregationWindowRate
	assert.Contains(t, buildPromQL(base), "/ 90)")

	base.Aggregation = metrics.AggregationActiveRate
	active := buildPromQL(base)
	assert.Contains(t, active, "rate(servicegraph_request_count")
	assert.NotContains(t, active, `temporality="delta"`)

	base.Aggregation = metrics.AggregationLatest
	assert.Equal(t, "servicegraph_request_count", buildPromQL(base))
}

func TestBuildPromQLWindowMeanRequiresPositiveCountIncrease(t *testing.T) {
	got := buildPromQL(query.MetricQuery{
		MetricName:  "http.server.request.duration",
		Aggregation: metrics.AggregationWindowMean,
		Window:      time.Minute,
	})

	assert.Contains(t, got, "http_server_request_duration_sum")
	assert.Contains(t, got, "http_server_request_duration_count")
	assert.Contains(t, got, "ignoring(oksg_unit)")
	assert.Contains(t, got, "> 0")
	assert.NotContains(t, got, "clamp_min")
}

func TestParseQueryRangeResponse(t *testing.T) {
	respJSON := `{
		"status": "success",
		"data": {
			"resultType": "matrix",
			"result": [{
				"metric": {
					"__name__": "k8s_pod_cpu_usage",
					"entity_id": "abc123",
					"source": "otlp",
					"oksg_unit": "millicores"
				},
				"values": [
					[1742644800, "42.5"],
					[1742644815, "43.0"]
				]
			}]
		}
	}`

	samples, err := parseQueryRangeResponse(strings.NewReader(respJSON))
	require.NoError(t, err)
	require.Len(t, samples, 2)

	assert.Equal(t, "k8s.pod.cpu.usage", samples[0].MetricName)
	assert.Equal(t, entities.EntityID("abc123"), samples[0].EntityID)
	assert.Equal(t, 42.5, samples[0].Value)
	assert.Equal(t, "otlp", samples[0].Labels["source"])
	assert.Equal(t, metrics.UnitMillicores, samples[0].Unit)
	assert.NotContains(t, samples[0].Labels, "__name__")
	assert.NotContains(t, samples[0].Labels, "entity_id")
	assert.NotContains(t, samples[0].Labels, "oksg_unit")
}

func TestParseQueryRangeResponse_HidesInvalidOrMissingStoredUnit(t *testing.T) {
	respJSON := `{
		"status": "success",
		"data": {
			"resultType": "matrix",
			"result": [
				{"metric": {"__name__": "k8s_resource_quota_used", "entity_id": "quota", "resource": "cpu", "oksg_unit": "cores"}, "values": [[1742644800, "2"]]},
				{"metric": {"__name__": "k8s_resource_quota_used", "entity_id": "quota", "resource": "pods"}, "values": [[1742644800, "3"]]}
			]
		}
	}`

	samples, err := parseQueryRangeResponse(strings.NewReader(respJSON))
	require.NoError(t, err)
	require.Len(t, samples, 2)
	for _, sample := range samples {
		assert.Empty(t, sample.Unit)
		assert.NotContains(t, sample.Labels, "oksg_unit")
	}
}

func TestParseQueryRangeResponse_ErrorStatus(t *testing.T) {
	respJSON := `{"status": "error", "data": {}}`
	_, err := parseQueryRangeResponse(strings.NewReader(respJSON))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error")
}

func TestParseQueryRangeResponse_SkipsNonFiniteValues(t *testing.T) {
	respJSON := `{
		"status": "success",
		"data": {
			"resultType": "matrix",
			"result": [{
				"metric": {
					"__name__": "rpc_server_request_count",
					"entity_id": "svc1",
					"source": "traces.servicegraph"
				},
				"values": [
					[1742644800, "+Inf"],
					[1742644815, "NaN"],
					[1742644830, "42"]
				]
			}]
		}
	}`

	samples, err := parseQueryRangeResponse(strings.NewReader(respJSON))
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, 42.0, samples[0].Value)
}

func TestVMMetricStore_Write(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/import/prometheus", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "text/plain", r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	store := NewVMMetricStore(srv.URL)
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	err := store.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "cpu", EntityID: "e1", Value: 10.0, Timestamp: ts},
	})

	require.NoError(t, err)
	assert.Contains(t, receivedBody, "cpu{")
	assert.Contains(t, receivedBody, `entity_id="e1"`)
}

func TestVMMetricStore_WriteEmpty(t *testing.T) {
	store := NewVMMetricStore("http://should-not-be-called")
	err := store.Write(context.Background(), nil)
	assert.NoError(t, err)
}

func TestVMMetricStore_WriteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer srv.Close()

	store := NewVMMetricStore(srv.URL)
	err := store.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "cpu", EntityID: "e1", Value: 1.0, Timestamp: time.Now()},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestVMMetricStore_Query(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/query_range", r.URL.Path)
		assert.Equal(t, "cpu_usage", r.URL.Query().Get("query"))
		assert.Equal(t, "1", r.URL.Query().Get("nocache"))

		resp := queryRangeResponse{Status: "success"}
		resp.Data.ResultType = "matrix"
		resp.Data.Result = []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
		}{
			{
				Metric: map[string]string{
					"__name__":  "cpu_usage",
					"entity_id": "e1",
				},
				Values: [][]interface{}{
					{float64(1742644800), "42.5"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	store := NewVMMetricStore(srv.URL)
	samples, err := store.Query(context.Background(), query.MetricQuery{
		MetricName: "cpu_usage",
		From:       time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		To:         time.Date(2026, 3, 22, 1, 0, 0, 0, time.UTC),
	})

	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, 42.5, samples[0].Value)
}

func TestVMMetricStore_QueryEmptyMetricName(t *testing.T) {
	store := NewVMMetricStore("http://should-not-be-called")
	samples, err := store.Query(context.Background(), query.MetricQuery{})
	assert.NoError(t, err)
	assert.Nil(t, samples)
}

func TestVMMetricStore_QueryCatalogStats(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/query", r.URL.Path)
		q := r.URL.Query().Get("query")
		queries = append(queries, q)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(q, "sum by") {
			// count response: 2 samples for k8s_pod_memory_working_set from
			// kubelet, 1 sample for servicegraph_request_count from mesh.istio,
			// and a histogram-backed duration metric represented by _sum/_count.
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
				{"metric":{"__name__":"k8s_pod_memory_working_set","source":"kubelet"},"value":[1747396800,"2"]},
				{"metric":{"__name__":"servicegraph_request_count","source":"mesh.istio"},"value":[1747396800,"1"]},
				{"metric":{"__name__":"servicegraph_request_duration_sum","source":"mesh.istio"},"value":[1747396800,"4"]},
				{"metric":{"__name__":"servicegraph_request_duration_count","source":"mesh.istio"},"value":[1747396800,"4"]}
			]}}`))
			return
		}
		// last_seen response: timestamps as scalar seconds.
		lastSeenTs := strconv.FormatInt(now.Add(-5*time.Minute).Unix(), 10)
		histogramSumLastSeenTs := strconv.FormatInt(now.Add(-7*time.Minute).Unix(), 10)
		histogramCountLastSeenTs := strconv.FormatInt(now.Add(-2*time.Minute).Unix(), 10)
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
			{"metric":{"__name__":"k8s_pod_memory_working_set","source":"kubelet"},"value":[1747396800,"` + lastSeenTs + `"]},
			{"metric":{"__name__":"servicegraph_request_count","source":"mesh.istio"},"value":[1747396800,"` + lastSeenTs + `"]},
			{"metric":{"__name__":"servicegraph_request_duration_sum","source":"mesh.istio"},"value":[1747396800,"` + histogramSumLastSeenTs + `"]},
			{"metric":{"__name__":"servicegraph_request_duration_count","source":"mesh.istio"},"value":[1747396800,"` + histogramCountLastSeenTs + `"]}
		]}}`))
	}))
	defer srv.Close()

	store := NewVMMetricStore(srv.URL)
	stats, err := store.QueryCatalogStats(context.Background(), time.Hour)
	require.NoError(t, err)
	require.Len(t, queries, 2)
	assert.Contains(t, queries[0], "count_over_time")
	assert.Contains(t, queries[0], "[3600s]")
	assert.Contains(t, queries[0], "keep_metric_names")
	assert.Contains(t, queries[1], "tlast_over_time")
	assert.Contains(t, queries[1], "keep_metric_names")

	byMetric := map[string]persistence.SourceStat{}
	for _, s := range stats {
		require.Len(t, s.BySource, 1)
		byMetric[s.MetricName] = s.BySource[0]
	}
	memory := byMetric["k8s.pod.memory.working_set"]
	assert.Equal(t, "kubelet", memory.SourceID)
	assert.EqualValues(t, 2, memory.SampleCount)
	assert.Equal(t, now.Add(-5*time.Minute).Unix(), memory.LastSeenAt.Unix())

	traffic := byMetric["servicegraph.request.count"]
	assert.Equal(t, "mesh.istio", traffic.SourceID)
	assert.EqualValues(t, 1, traffic.SampleCount)

	duration := byMetric["servicegraph.request.duration"]
	assert.Equal(t, "mesh.istio", duration.SourceID)
	assert.EqualValues(t, 4, duration.SampleCount)
	assert.Equal(t, now.Add(-2*time.Minute).Unix(), duration.LastSeenAt.Unix())
}

func TestVMMetricStore_Ping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := NewVMMetricStore(srv.URL)
	assert.NoError(t, store.Ping(context.Background()))
}

func TestVMMetricStore_PingError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	store := NewVMMetricStore(srv.URL)
	err := store.Ping(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}
