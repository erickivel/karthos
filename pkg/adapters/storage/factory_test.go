package storage

import (
	"context"
	"testing"

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
	assert.Nil(t, stores.RawStore, "raw store should be nil when disabled")
}

func TestNewStores_MemoryBackendWithRaw(t *testing.T) {
	stores, err := NewStores(context.Background(), StorageConfig{
		Backend:    BackendMemory,
		RawEnabled: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, stores.Close()) })

	assert.NotNil(t, stores.RawStore, "raw store should be non-nil when enabled")
}

func TestNewStores_EmptyBackendDefaultsToMemory(t *testing.T) {
	stores, err := NewStores(context.Background(), StorageConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, stores.Close()) })

	assert.NotNil(t, stores.MetricStore)
	assert.NotNil(t, stores.GraphStore)
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
		PostgresDSN: "postgres://localhost/oksg",
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
