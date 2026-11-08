package victoriametrics

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/erickivel/karthos/pkg/backend/query"
	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/sources"
)

// Requires a disposable VM1.138 instance; no production endpoint should be used.
func TestQueryViewsVictoriaMetricsIntegration(t *testing.T) {
	base := os.Getenv("KARTHOS_TEST_VM_URL")
	if base == "" {
		t.Skip("set KARTHOS_TEST_VM_URL to an isolated VictoriaMetrics1.138 endpoint")
	}
	store := NewVMMetricStore(base)
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	end := time.Now().UTC().Add(-5 * time.Minute).Truncate(15 * time.Second)
	prefix := fmt.Sprintf("query-view-integration-%d-", time.Now().UnixNano())
	type fixture struct {
		name, protocol, httpStatus, grpcStatus, temporality string
		values                                              []float64
		stale                                               bool
	}
	fixtures := []fixture{
		{name: "grpc-failure", protocol: "grpc", httpStatus: "200", grpcStatus: "13", values: []float64{0, 2, 4, 6, 8}},
		{name: "grpc-success", protocol: "grpc", httpStatus: "200", grpcStatus: "0", values: []float64{0, 2, 4, 6, 8}},
		{name: "grpc-unknown", protocol: "grpc", httpStatus: "200", values: []float64{0, 2, 4, 6, 8}},
		{name: "http-client", protocol: "http", httpStatus: "404", values: []float64{0, 2, 4, 6, 8}},
		{name: "http-server", protocol: "http", httpStatus: "500", values: []float64{0, 2, 4, 6, 8}},
		{name: "grpc-precedence", protocol: "grpc", httpStatus: "500", grpcStatus: "0", values: []float64{0, 2, 4, 6, 8}},
		{name: "counter-reset", protocol: "grpc", httpStatus: "200", grpcStatus: "13", values: []float64{0, 2, 5, 1, 4}},
		{name: "delta", protocol: "grpc", httpStatus: "200", grpcStatus: "13", temporality: "delta", values: []float64{1, 2, 3, 4, 5}},
		{name: "stale", protocol: "grpc", httpStatus: "200", grpcStatus: "0", values: []float64{0, 2, 4, 6, 8}, stale: true},
	}
	var samples []metrics.MetricSample
	for _, f := range fixtures {
		for i, value := range f.values {
			at := end.Add(time.Duration(i-4) * 30 * time.Second)
			if f.stale {
				at = at.Add(-10 * time.Minute)
			}
			labels := map[string]string{"source": metrics.SourceMeshIstio, "network.protocol.name": f.protocol, "http.response.status_code": f.httpStatus, "edge_client_id": "frontend", "edge_server_id": "checkout"}
			if f.grpcStatus != "" {
				labels["rpc.grpc.status_code"] = f.grpcStatus
			}
			if f.temporality != "" {
				labels["temporality"] = f.temporality
			}
			samples = append(samples, metrics.MetricSample{EntityID: entities.EntityID(prefix + f.name), MetricName: "servicegraph.request.count", Value: value, Timestamp: at, Labels: labels, Unit: metrics.UnitCount})
		}
	}
	// An old ingest-derived metric must not override the request-derived view.
	samples = append(samples, metrics.MetricSample{EntityID: entities.EntityID(prefix + "grpc-failure"), MetricName: "servicegraph.request.error.count", Value: 999, Timestamp: end, Labels: map[string]string{"source": metrics.SourceMeshIstio}, Unit: metrics.UnitCount})
	samples = append(samples, metrics.MetricSample{EntityID: entities.EntityID(prefix + "native"), MetricName: "servicegraph.request.error.count", Value: 7, Timestamp: end, Labels: map[string]string{"source": metrics.SourceTracesServicegraph, "failed": "true"}, Unit: metrics.UnitCount})
	for _, v := range []struct {
		name  string
		value float64
		unit  metrics.Unit
	}{{"k8s.container.cpu.usage", 123, metrics.UnitMillicores}, {"k8s.container.memory.usage", 1048576, metrics.UnitBytes}} {
		samples = append(samples, metrics.MetricSample{EntityID: entities.EntityID(prefix + "resources"), MetricName: v.name, Value: v.value, Timestamp: end, Labels: map[string]string{"source": metrics.SourceKubelet}, Unit: v.unit})
	}
	if err := store.Write(ctx, samples); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(strings.TrimRight(base, "/") + "/internal/force_flush") //nolint:gosec,noctx // explicitly selected isolated test service
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("flush status %d", resp.StatusCode)
	}
	views := sources.DefaultRegistry().QueryViews("servicegraph.request.error.count")
	if len(views) != 2 {
		t.Fatalf("expected both source views, got %d", len(views))
	}
	makeQuery := func(name string, mode metrics.AggregationMode) query.MetricQuery {
		return query.MetricQuery{MetricName: "servicegraph.request.error.count", EntityIDs: []entities.EntityID{entities.EntityID(prefix + name)}, Source: metrics.SourceMeshIstio, From: end, To: end, Step: 15 * time.Second, Window: 60 * time.Second, Aggregation: mode, MaxSampleAge: 45 * time.Second}
	}
	check := func(t *testing.T, q query.MetricQuery, want float64, present bool, derivation string) {
		t.Helper()
		got, err := store.Query(ctx, q)
		if err != nil {
			t.Fatalf("query: %v\nexpression: %s", err, RenderQuery(q))
		}
		if !present {
			if len(got) != 0 {
				t.Fatalf("expected missing evidence, got %+v", got)
			}
			return
		}
		if len(got) != 1 {
			t.Fatalf("expected one partition, got %+v\nexpression: %s", got, RenderQuery(q))
		}
		if math.Abs(got[0].Value-want) > 1e-9 {
			t.Fatalf("value %g, want %g\nexpression: %s", got[0].Value, want, RenderQuery(q))
		}
		if got[0].Derivation == nil || got[0].Derivation.ID != derivation {
			t.Fatalf("derivation %+v, want %s", got[0].Derivation, derivation)
		}
		if got[0].Labels["source"] != q.Source {
			t.Fatalf("wrong source %+v", got[0].Labels)
		}
		if !got[0].Timestamp.Equal(end) {
			t.Fatalf("timestamp %s want %s", got[0].Timestamp, end)
		}
	}
	for _, tc := range []struct {
		name    string
		want    float64
		present bool
	}{{"grpc-failure", 8, true}, {"grpc-success", 0, true}, {"grpc-unknown", 0, false}, {"http-client", 0, true}, {"http-server", 8, true}, {"grpc-precedence", 0, true}, {"stale", 0, false}} {
		t.Run(tc.name, func(t *testing.T) {
			q := makeQuery(tc.name, metrics.AggregationLatest)
			check(t, q, tc.want, tc.present, "istio.request-errors")
		})
	}
	for _, tc := range []struct {
		name     string
		increase float64
	}{{"grpc-failure", 4}, {"counter-reset", 4}, {"delta", 9}} {
		t.Run(tc.name+"-temporal", func(t *testing.T) {
			check(t, makeQuery(tc.name, metrics.AggregationIncrease), tc.increase, true, "istio.request-errors")
			check(t, makeQuery(tc.name, metrics.AggregationWindowRate), tc.increase/60, true, "istio.request-errors")
			if tc.name != "delta" {
				check(t, makeQuery(tc.name, metrics.AggregationActiveRate), tc.increase/60, true, "istio.request-errors")
			}
		})
	}
	t.Run("zero-and-unknown-temporal", func(t *testing.T) {
		for _, mode := range []metrics.AggregationMode{metrics.AggregationIncrease, metrics.AggregationWindowRate, metrics.AggregationActiveRate} {
			check(t, makeQuery("grpc-success", mode), 0, true, "istio.request-errors")
			check(t, makeQuery("grpc-unknown", mode), 0, false, "")
			check(t, makeQuery("stale", mode), 0, false, "")
		}
		check(t, makeQuery("delta", metrics.AggregationActiveRate), 0, false, "")
	})
	t.Run("edge-restrictions", func(t *testing.T) {
		q := makeQuery("grpc-failure", metrics.AggregationLatest)
		q.EdgeSourceID = "frontend"
		q.EdgeTargetID = "checkout"
		check(t, q, 8, true, "istio.request-errors")
		q.EdgeTargetID = "different-service"
		check(t, q, 0, false, "")
	})
	t.Run("native-source-and-provenance", func(t *testing.T) {
		q := makeQuery("native", metrics.AggregationLatest)
		q.Source = metrics.SourceTracesServicegraph
		check(t, q, 7, true, "servicegraph.connector-failures")
		q.Source = metrics.SourceMeshIstio
		check(t, q, 0, false, "")
	})
	t.Run("canonical-resource-roundtrip", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			value float64
			unit  metrics.Unit
		}{{"k8s.container.cpu.usage", 123, metrics.UnitMillicores}, {"k8s.container.memory.usage", 1048576, metrics.UnitBytes}} {
			q := makeQuery("resources", metrics.AggregationLatest)
			q.MetricName = tc.name
			q.Source = metrics.SourceKubelet
			got, err := store.Query(ctx, q)
			if err != nil || len(got) != 1 {
				t.Fatalf("%s: %+v %v", tc.name, got, err)
			}
			if got[0].Value != tc.value || got[0].Unit != tc.unit {
				t.Fatalf("%s: %+v", tc.name, got[0])
			}
		}
	})
}
