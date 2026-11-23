package storage

import (
	"context"
	"github.com/erickivel/karthos/pkg/backend/query"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStores_MemoryBackend(t *testing.T) {
	stores, err := NewStores(context.Background(), StorageConfig{
		Backend: BackendMemory,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, stores.Close()) })

	assert.NotNil(t, stores.MetricStore)
	assert.NotNil(t, stores.GraphStore)
	assert.NotNil(t, stores.MetadataStore)
	assert.NotNil(t, stores.EntityRegistry)
}

func TestNewStoresRequiresExplicitBackend(t *testing.T) {
	_, err := NewStores(context.Background(), StorageConfig{})
	require.ErrorContains(t, err, "unsupported storage backend")
}

func TestNewStores_ExternalRequiresPostgresDSN(t *testing.T) {
	_, err := NewStores(context.Background(), StorageConfig{
		Backend: BackendExternal,
		VMURL:   "http://localhost:8428",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "postgres-dsn")
}

func TestNewStores_ExternalRequiresVMURL(t *testing.T) {
	_, err := NewStores(context.Background(), StorageConfig{
		Backend:     BackendExternal,
		PostgresDSN: "postgres://localhost/karthos",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "victoriametrics-url")
}

func TestNewStores_UnknownBackend(t *testing.T) {
	_, err := NewStores(context.Background(), StorageConfig{
		Backend: "redis",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestStores_CloseIsIdempotent(t *testing.T) {
	stores, err := NewStores(context.Background(), StorageConfig{
		Backend: BackendMemory,
	})
	require.NoError(t, err)

	assert.NoError(t, stores.Close())
	assert.NoError(t, stores.Close())
}

func TestMemoryFactoryWiresQueryViews(t *testing.T) {
	stores, err := NewStores(context.Background(), StorageConfig{Backend: BackendMemory})
	require.NoError(t, err)
	at := time.Now().UTC()
	labels := map[string]string{"source": "mesh.istio", "network.protocol.name": "grpc", "rpc.grpc.status_code": "13", "http.response.status_code": "200"}
	require.NoError(t, stores.MetricStore.Write(context.Background(), []metrics.MetricSample{{MetricName: "servicegraph.request.count", EntityID: "checkout", Value: 7, Timestamp: at, Unit: metrics.UnitCount, Labels: labels}}))
	result, err := stores.MetricStore.Query(context.Background(), query.MetricQuery{MetricName: "servicegraph.request.error.count", Source: "mesh.istio", From: at, To: at, Step: time.Second, Aggregation: metrics.AggregationLatest})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, float64(7), result[0].Value)
	require.Equal(t, "istio.request-errors", result[0].Derivation.ID)
}
