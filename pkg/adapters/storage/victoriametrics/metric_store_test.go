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
		},
	}

	got := string(samplesToPrometheusText(samples))

	assert.Contains(t, got, "k8s_pod_cpu_usage{")
	assert.Contains(t, got, `entity_id="abc123"`)
	assert.Contains(t, got, `source="otlp"`)
	assert.Contains(t, got, "42.5")
	// Verify timestamp is present as milliseconds.
	expectedTsMs := strconv.FormatInt(ts.UnixMilli(), 10)
	assert.Contains(t, got, expectedTsMs)
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
			expected: `k8s_pod_cpu_usage{entity_id=~"abc123"}`,
		},
		{
			name: "metric name with multiple entity IDs",
			query: query.MetricQuery{
				MetricName: "k8s.pod.memory.working_set",
				EntityIDs:  []entities.EntityID{"abc", "def"},
			},
			expected: `k8s_pod_memory_working_set{entity_id=~"abc|def"}`,
		},
		{
			name: "scalar metric with edge source and target filters",
			query: query.MetricQuery{
				MetricName:   "servicegraph.request.error.count",
				EdgeSourceID: entities.EntityID("svc-a"),
				EdgeTargetID: entities.EntityID("svc-b"),
			},
			expected: `rate(servicegraph_request_error_count{edge_client_id="svc-a",edge_server_id="svc-b"}[1m])`,
		},
		{
			name: "request rate metric uses one minute rate window",
			query: query.MetricQuery{
				MetricName: "servicegraph.request.count",
			},
			expected: `rate(servicegraph_request_count[1m])`,
		},
		{
			name: "request rate metric keeps entity filters inside rate query",
			query: query.MetricQuery{
				MetricName: "servicegraph.request.count",
				EntityIDs:  []entities.EntityID{"abc123"},
			},
			expected: `rate(servicegraph_request_count{entity_id=~"abc123"}[1m])`,
		},
		{
			name: "request rate metric keeps edge filters inside rate query",
			query: query.MetricQuery{
				MetricName:   "servicegraph.request.count",
				EdgeSourceID: entities.EntityID("svc-a"),
				EdgeTargetID: entities.EntityID("svc-b"),
			},
			expected: `rate(servicegraph_request_count{edge_client_id="svc-a",edge_server_id="svc-b"}[1m])`,
		},
		{
			name: "error rate metric uses one minute rate window",
			query: query.MetricQuery{
				MetricName: "servicegraph.request.error.count",
			},
			expected: `rate(servicegraph_request_error_count[1m])`,
		},
		{
			name: "error ratio uses request and error rate expressions",
			query: query.MetricQuery{
				MetricName:   "servicegraph.request.error.ratio",
				EdgeSourceID: entities.EntityID("svc-a"),
				EdgeTargetID: entities.EntityID("svc-b"),
			},
			expected: `rate(servicegraph_request_error_count{edge_client_id="svc-a",edge_server_id="svc-b"}[1m]) / clamp_min(rate(servicegraph_request_count{edge_client_id="svc-a",edge_server_id="svc-b"}[1m]), 1e-9)`,
		},
		{
			name: "http.server.request.duration derives rate-weighted mean from sum/count",
			query: query.MetricQuery{
				MetricName: "http.server.request.duration",
				EntityIDs:  []entities.EntityID{"svc-1"},
			},
			expected: `rate(http_server_request_duration_sum{entity_id=~"svc-1"}[1m]) / clamp_min(rate(http_server_request_duration_count{entity_id=~"svc-1"}[1m]), 1)`,
		},
		{
			name: "servicegraph.request.duration derives rate-weighted mean from sum/count with edge filters",
			query: query.MetricQuery{
				MetricName:   "servicegraph.request.duration",
				EdgeSourceID: entities.EntityID("svc-a"),
				EdgeTargetID: entities.EntityID("svc-b"),
			},
			expected: `rate(servicegraph_request_duration_sum{edge_client_id="svc-a",edge_server_id="svc-b"}[1m]) / clamp_min(rate(servicegraph_request_duration_count{edge_client_id="svc-a",edge_server_id="svc-b"}[1m]), 1)`,
		},
		{
			name: "container health status composes ready and restart-rate per container",
			query: query.MetricQuery{
				MetricName: "oksg.container.health.status",
				EntityIDs:  []entities.EntityID{"container-1"},
			},
			expected: `(rate(k8s_container_restarts{entity_id=~"container-1"}[5m]) > 0) * 0 + 3 or (k8s_container_ready{entity_id=~"container-1"} == 0) * 0 + 2 or (k8s_container_ready{entity_id=~"container-1"} == 1) * 0 + 1`,
		},
		{
			name: "pod health status maps phase enum to public health enum",
			query: query.MetricQuery{
				MetricName: "oksg.pod.health.status",
				EntityIDs:  []entities.EntityID{"pod-1"},
			},
			expected: `(k8s_pod_phase{entity_id=~"pod-1"} == 2) * 0 + 1 or (k8s_pod_phase{entity_id=~"pod-1"} == 1) * 0 + 4 or (k8s_pod_phase{entity_id=~"pod-1"} == 3) * 0 + 5 or (k8s_pod_phase{entity_id=~"pod-1"} == 4) * 0 + 6 or (k8s_pod_phase{entity_id=~"pod-1"} == 5) * 0 + 7`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPromQL(tt.query)
			assert.Equal(t, tt.expected, got)
		})
	}
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
					"source": "otlp"
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
	assert.NotContains(t, samples[0].Labels, "__name__")
	assert.NotContains(t, samples[0].Labels, "entity_id")
}

func TestParseQueryRangeResponse_ErrorStatus(t *testing.T) {
	respJSON := `{"status": "error", "data": {}}`
	_, err := parseQueryRangeResponse(strings.NewReader(respJSON))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error")
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
