package persistence

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/entities"
)

func TestEntityRegistry_UpsertAndGet(t *testing.T) {
	reg := NewInMemoryEntityRegistry()
	ctx := context.Background()

	pod := entities.NewPod("cluster", "default", "my-pod", map[string]string{
		"cluster":   "cluster",
		"namespace": "default",
		"name":      "my-pod",
	})

	require.NoError(t, reg.UpsertEntity(ctx, pod))

	got, err := reg.GetEntity(ctx, pod.ID())
	require.NoError(t, err)
	assert.Equal(t, pod.ID(), got.ID())
	assert.Equal(t, entities.KindPod, got.Kind())
}

func TestEntityRegistry_GetNotFound(t *testing.T) {
	reg := NewInMemoryEntityRegistry()
	ctx := context.Background()

	_, err := reg.GetEntity(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestEntityRegistry_UpsertOverwrites(t *testing.T) {
	reg := NewInMemoryEntityRegistry()
	ctx := context.Background()

	svc1 := entities.NewService("cluster", "default", "api", nil, map[string]string{
		"version": "v1",
	})
	require.NoError(t, reg.UpsertEntity(ctx, svc1))

	svc2 := entities.NewService("cluster", "default", "api", nil, map[string]string{
		"version": "v2",
	})
	require.NoError(t, reg.UpsertEntity(ctx, svc2))

	got, err := reg.GetEntity(ctx, svc1.ID())
	require.NoError(t, err)
	assert.Equal(t, "v2", got.Labels()["version"])
}

func TestEntityRegistry_ListByKind(t *testing.T) {
	reg := NewInMemoryEntityRegistry()
	ctx := context.Background()

	pod := entities.NewPod("cluster", "default", "pod-1", map[string]string{
		"cluster":   "cluster",
		"namespace": "default",
		"name":      "pod-1",
	})
	svc := entities.NewService("cluster", "default", "svc-1", nil, map[string]string{
		"cluster":   "cluster",
		"namespace": "default",
		"name":      "svc-1",
	})
	node := entities.NewNode("cluster", "node-1", map[string]string{
		"cluster": "cluster",
		"name":    "node-1",
	})

	require.NoError(t, reg.UpsertEntity(ctx, pod))
	require.NoError(t, reg.UpsertEntity(ctx, svc))
	require.NoError(t, reg.UpsertEntity(ctx, node))

	got, err := reg.ListEntities(ctx, EntityQuery{Kind: entities.KindPod})
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, entities.KindPod, got[0].Kind())
}

func TestEntityRegistry_ListByNamespace(t *testing.T) {
	reg := NewInMemoryEntityRegistry()
	ctx := context.Background()

	pod1 := entities.NewPod("cluster", "ns-a", "pod-1", map[string]string{
		"namespace": "ns-a",
	})
	pod2 := entities.NewPod("cluster", "ns-b", "pod-2", map[string]string{
		"namespace": "ns-b",
	})

	require.NoError(t, reg.UpsertEntity(ctx, pod1))
	require.NoError(t, reg.UpsertEntity(ctx, pod2))

	got, err := reg.ListEntities(ctx, EntityQuery{Namespace: "ns-a"})
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestEntityRegistry_UpsertStoredEntityPreservesFields(t *testing.T) {
	reg := NewInMemoryEntityRegistry()
	ctx := context.Background()

	// Simulate what entity_resolver.go does: passes a StoredEntity with
	// Name/Namespace/Cluster already resolved, and Metadata containing OTLP keys.
	se := StoredEntity{
		EntityID:   "test-pod-id",
		EntityKind: entities.KindPod,
		Cluster:    "my-cluster",
		Namespace:  "default",
		Name:       "my-pod",
		Metadata: map[string]string{
			"k8s.pod.name":       "my-pod",
			"k8s.namespace.name": "default",
			"k8s.node.name":      "node-1",
		},
	}

	require.NoError(t, reg.UpsertEntity(ctx, se))

	got, err := reg.GetEntity(ctx, "test-pod-id")
	require.NoError(t, err)

	stored := got.(StoredEntity)
	assert.Equal(t, "my-pod", stored.Name)
	assert.Equal(t, "default", stored.Namespace)
	assert.Equal(t, "my-cluster", stored.Cluster)
}

func TestEntityRegistry_UpsertOTLPLabelsDerivesFields(t *testing.T) {
	reg := NewInMemoryEntityRegistry()
	ctx := context.Background()

	// Simulate a non-StoredEntity (e.g., entities.NewPod) with OTLP-style labels
	// where the "name"/"namespace" short keys are absent.
	pod := entities.NewPod("", "default", "my-pod", map[string]string{
		"k8s.pod.name":       "my-pod",
		"k8s.namespace.name": "default",
	})

	require.NoError(t, reg.UpsertEntity(ctx, pod))

	got, err := reg.GetEntity(ctx, pod.ID())
	require.NoError(t, err)

	stored := got.(StoredEntity)
	assert.Equal(t, "my-pod", stored.Name)
	assert.Equal(t, "default", stored.Namespace)
}

func TestMaterializeEntityClonesLabelsAndUsesCanonicalFallbacks(t *testing.T) {
	labels := map[string]string{
		"k8s.cluster.name":   "cluster-a",
		"k8s.namespace.name": "payments",
		"service.name":       "checkout",
	}

	got := MaterializeEntity(entities.NewService("", "", "", nil, labels))
	labels["service.name"] = "mutated"

	assert.Equal(t, "cluster-a", got.Cluster)
	assert.Equal(t, "payments", got.Namespace)
	assert.Equal(t, "checkout", got.Name)
	assert.Equal(t, "checkout", got.Metadata["service.name"])
}

func TestStoredEntityLabelsReturnsCopy(t *testing.T) {
	entity := StoredEntity{Metadata: map[string]string{"name": "api"}}

	labels := entity.Labels()
	labels["name"] = "mutated"

	assert.Equal(t, "api", entity.Metadata["name"])
}

func TestEntityRegistry_ListAll(t *testing.T) {
	reg := NewInMemoryEntityRegistry()
	ctx := context.Background()

	pod := entities.NewPod("cluster", "default", "pod-1", nil)
	svc := entities.NewService("cluster", "default", "svc-1", nil, nil)

	require.NoError(t, reg.UpsertEntity(ctx, pod))
	require.NoError(t, reg.UpsertEntity(ctx, svc))

	got, err := reg.ListEntities(ctx, EntityQuery{})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}
