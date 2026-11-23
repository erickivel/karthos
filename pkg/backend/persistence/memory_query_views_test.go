package persistence_test

import (
	"context"
	"github.com/erickivel/karthos/pkg/backend/persistence"
	"github.com/erickivel/karthos/pkg/backend/query"
	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/sources"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestMemorySourceOwnedQueryViews(t *testing.T) {
	ctx := context.Background()
	end := time.Now().UTC().Truncate(time.Second)
	store := persistence.NewInMemoryMetricStore(persistence.WithMetricQueryViews(sources.DefaultRegistry().QueryViews))
	fixtures := []struct {
		name, protocol, http, grpc string
		want                       float64
		present                    bool
	}{
		{"grpc-failed", "grpc", "200", "13", 8, true}, {"grpc-success", "grpc", "200", "0", 0, true}, {"grpc-unknown", "grpc", "200", "", 0, false}, {"http-client", "http", "404", "", 0, true}, {"http-server", "http", "500", "", 8, true},
	}
	for _, f := range fixtures {
		labels := map[string]string{"source": "mesh.istio", "network.protocol.name": f.protocol, "http.response.status_code": f.http, "edge_client_id": "front", "edge_server_id": "checkout"}
		if f.grpc != "" {
			labels["rpc.grpc.status_code"] = f.grpc
		}
		for i := 0; i < 5; i++ {
			require.NoError(t, store.Write(ctx, []metrics.MetricSample{{MetricName: "servicegraph.request.count", EntityID: entities.EntityID(f.name), Value: float64(i * 2), Timestamp: end.Add(time.Duration(i-4) * 30 * time.Second), Unit: metrics.UnitCount, Labels: labels}}))
		}
		require.NoError(t, store.Write(ctx, []metrics.MetricSample{{MetricName: "servicegraph.request.error.count", EntityID: entities.EntityID(f.name), Value: 999, Timestamp: end, Unit: metrics.UnitCount, Labels: labels}}))
		q := query.MetricQuery{MetricName: "servicegraph.request.error.count", Source: "mesh.istio", EntityIDs: []entities.EntityID{entities.EntityID(f.name)}, From: end, To: end, Step: 15 * time.Second, Aggregation: metrics.AggregationLatest, MaxSampleAge: time.Minute}
		result, err := store.Query(ctx, q)
		require.NoError(t, err)
		if !f.present {
			require.Empty(t, result)
			continue
		}
		require.Len(t, result, 1)
		require.Equal(t, f.want, result[0].Value)
		require.Equal(t, "istio.request-errors", result[0].Derivation.ID)
		require.Equal(t, f.grpc, result[0].Labels["rpc.grpc.status_code"])
		for _, mode := range []metrics.AggregationMode{metrics.AggregationIncrease, metrics.AggregationWindowRate, metrics.AggregationActiveRate} {
			q.Aggregation = mode
			q.Window = time.Minute
			q.Unit = metrics.UnitCount
			got, err := store.Query(ctx, q)
			require.NoError(t, err)
			require.Len(t, got, 1)
			want := f.want / 2
			if mode != metrics.AggregationIncrease {
				want /= 60
			}
			require.InDelta(t, want, got[0].Value, 1e-9)
		}
		q.Aggregation = metrics.AggregationLatest
		q.To = end.Add(2 * time.Minute)
		q.From = q.To
		stale, err := store.Query(ctx, q)
		require.NoError(t, err)
		require.Empty(t, stale)
		q.To = end
		q.From = end
		q.Source = "traces.servicegraph"
		absent, err := store.Query(ctx, q)
		require.NoError(t, err)
		require.Empty(t, absent)
	}
	stats, err := store.QueryCatalogStats(ctx, time.Hour)
	require.NoError(t, err)
	for _, stat := range stats {
		if stat.MetricName == "servicegraph.request.error.count" {
			require.Len(t, stat.BySource, 1)
			require.Equal(t, int64(20), stat.BySource[0].SampleCount)
			return
		}
	}
	t.Fatal("missing query-view catalog coverage")
}

func TestMemoryViewsPreserveNativeSourceAndCounterResetPartitions(t *testing.T) {
	ctx := context.Background()
	end := time.Now().UTC().Truncate(time.Second)
	store := persistence.NewInMemoryMetricStore(persistence.WithMetricQueryViews(sources.DefaultRegistry().QueryViews))
	for i, value := range []float64{0, 2, 5, 1, 4} {
		require.NoError(t, store.Write(ctx, []metrics.MetricSample{{MetricName: "servicegraph.request.count", EntityID: "reset", Timestamp: end.Add(time.Duration(i-4) * 30 * time.Second), Value: value, Unit: metrics.UnitCount, Labels: map[string]string{"source": "mesh.istio", "network.protocol.name": "grpc", "rpc.grpc.status_code": "13", "edge_client_id": "front", "edge_server_id": "checkout"}}}))
	}
	require.NoError(t, store.Write(ctx, []metrics.MetricSample{{MetricName: "servicegraph.request.error.count", EntityID: "native", Timestamp: end, Value: 7, Unit: metrics.UnitCount, Labels: map[string]string{"source": "traces.servicegraph", "failed": "true"}}}))
	q := query.MetricQuery{MetricName: "servicegraph.request.error.count", Source: "mesh.istio", EntityIDs: []entities.EntityID{"reset"}, To: end, Window: time.Minute, Aggregation: metrics.AggregationIncrease, Unit: metrics.UnitCount, MaxSampleAge: time.Minute, EdgeSourceID: "front", EdgeTargetID: "checkout"}
	got, err := store.Query(ctx, q)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, float64(4), got[0].Value)
	q.EdgeTargetID = "other"
	got, err = store.Query(ctx, q)
	require.NoError(t, err)
	require.Empty(t, got)
	q = query.MetricQuery{MetricName: "servicegraph.request.error.count", Source: "traces.servicegraph", EntityIDs: []entities.EntityID{"native"}, To: end, Aggregation: metrics.AggregationLatest, MaxSampleAge: time.Minute}
	got, err = store.Query(ctx, q)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, float64(7), got[0].Value)
	require.Equal(t, "servicegraph.connector-failures", got[0].Derivation.ID)
	q.MetricName = "servicegraph.request.count"
	q.Source = "mesh.istio"
	q.EntityIDs = []entities.EntityID{"reset"}
	q.From = end
	raw, err := store.Query(ctx, q)
	require.NoError(t, err)
	require.Len(t, raw, 1)
	require.Nil(t, raw[0].Derivation)
	require.Equal(t, float64(4), raw[0].Value)
}
