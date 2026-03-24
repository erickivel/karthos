package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func TestMetadataStore_EmptyReturnsNil(t *testing.T) {
	store := NewInMemoryMetadataStore()
	ctx := context.Background()

	got, err := store.GetDiscovery(ctx)
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestMetadataStore_PutAndGet(t *testing.T) {
	store := NewInMemoryMetadataStore()
	ctx := context.Background()

	prof := profile.ClusterProfile{
		ClusterID:    entities.ComputeEntityID(entities.KindCluster, "test"),
		ClusterName:  "test-cluster",
		DiscoveredAt: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}

	require.NoError(t, store.PutDiscovery(ctx, prof))

	got, err := store.GetDiscovery(ctx)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "test-cluster", got.ClusterName)
	assert.Equal(t, prof.ClusterID, got.ClusterID)
}

func TestMetadataStore_OverwritesPrevious(t *testing.T) {
	store := NewInMemoryMetadataStore()
	ctx := context.Background()

	prof1 := profile.ClusterProfile{ClusterName: "first"}
	prof2 := profile.ClusterProfile{ClusterName: "second"}

	require.NoError(t, store.PutDiscovery(ctx, prof1))
	require.NoError(t, store.PutDiscovery(ctx, prof2))

	got, err := store.GetDiscovery(ctx)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "second", got.ClusterName)
}

func TestMetadataStore_ReturnsCopy(t *testing.T) {
	store := NewInMemoryMetadataStore()
	ctx := context.Background()

	prof := profile.ClusterProfile{ClusterName: "original"}
	require.NoError(t, store.PutDiscovery(ctx, prof))

	got1, _ := store.GetDiscovery(ctx)
	got1.ClusterName = "mutated"

	got2, _ := store.GetDiscovery(ctx)
	assert.Equal(t, "original", got2.ClusterName)
}
