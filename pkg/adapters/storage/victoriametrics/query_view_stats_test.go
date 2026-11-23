package victoriametrics

import (
	"context"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQueryViewCatalogStatsUseClassifiedInputs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expression := r.URL.Query().Get("query")
		if strings.Contains(expression, `rpc\.grpc\.status_code`) {
			value := "9"
			if strings.Contains(expression, "tlast_over_time") {
				value = "1700000000"
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"source":"mesh.istio"},"value":[1700000000,"` + value + `"]}]}}`))
		} else {
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		}
	}))
	defer server.Close()
	stats, err := NewVMMetricStore(server.URL).QueryCatalogStats(context.Background(), time.Hour)
	require.NoError(t, err)
	for _, stat := range stats {
		if stat.MetricName == "servicegraph.request.error.count" {
			require.Len(t, stat.BySource, 1)
			require.Equal(t, int64(9), stat.BySource[0].SampleCount)
			require.Equal(t, int64(1700000000), stat.BySource[0].LastSeenAt.Unix())
			return
		}
	}
	t.Fatal("query-derived view missing from catalog coverage")
}
