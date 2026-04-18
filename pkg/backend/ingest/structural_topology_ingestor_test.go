package ingest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/topology"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func TestStructuralTopologyIngestor_WritesSnapshotToGraphStore(t *testing.T) {
	prof := &profile.ClusterProfile{
		ClusterName: "test",
		Services: []entities.Service{
			entities.NewService("test", "demo", "gateway-go", map[string]string{"app": "gateway-go"}, nil),
		},
		Workloads: []entities.Workload{
			workloadWithPodLabels("test", "demo", "gateway-go", map[string]string{"app": "gateway-go"}),
		},
	}

	store := persistence.NewInMemoryGraphStore()
	builder := NewK8sStructuralTopologyBuilder("test")
	ingestor := NewStructuralTopologyIngestor(builder, store, nil, nil)

	err := ingestor.IngestProfile(context.Background(), prof)
	require.NoError(t, err)

	snapshot, err := store.GetLatest(context.Background())
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Len(t, snapshot.Graph.Edges, 1)
	require.Equal(t, topology.RelExposes, snapshot.Graph.Edges[0].Relation)
	require.Len(t, snapshot.Graph.Nodes, 2)
}

func TestStructuralTopologyIngestor_EmptyProfileWritesNothing(t *testing.T) {
	prof := &profile.ClusterProfile{ClusterName: "test"}

	store := persistence.NewInMemoryGraphStore()
	builder := NewK8sStructuralTopologyBuilder("test")
	ingestor := NewStructuralTopologyIngestor(builder, store, nil, nil)

	err := ingestor.IngestProfile(context.Background(), prof)
	require.NoError(t, err)
	snapshot, err := store.GetLatest(context.Background())
	require.NoError(t, err)
	require.Nil(t, snapshot)
}

func TestStructuralTopologyIngestor_UpsertsStructuralEntities(t *testing.T) {
	prof := &profile.ClusterProfile{
		ClusterName: "test",
		Services: []entities.Service{
			entities.NewService("test", "demo", "gateway-go", map[string]string{"app": "gateway-go"}, map[string]string{"name": "gateway-go", "namespace": "demo"}),
		},
		Workloads: []entities.Workload{
			func() entities.Workload {
				w := workloadWithPodLabels("test", "demo", "gateway-go", map[string]string{"app": "gateway-go"})
				w.ConfigMapRefs = []string{"gateway-config"}
				w.SecretRefs = []string{"gateway-secret"}
				w.PVCRefs = []string{"gateway-cache"}
				return w
			}(),
		},
		Ingresses: []entities.Ingress{
			entities.NewIngress("test", "demo", "gateway", "gateway.demo.local", map[string]string{"name": "gateway", "namespace": "demo"}),
		},
	}
	prof.Ingresses[0].BackendServices = []entities.IngressBackend{{ServiceName: "gateway-go", ServicePort: 8080}}

	graphStore := persistence.NewInMemoryGraphStore()
	entityRegistry := persistence.NewInMemoryEntityRegistry()
	builder := NewK8sStructuralTopologyBuilder("test")
	ingestor := NewStructuralTopologyIngestor(builder, graphStore, entityRegistry, nil)

	err := ingestor.IngestProfile(context.Background(), prof)
	require.NoError(t, err)

	assertEntity := func(id entities.EntityID, kind entities.EntityKind, name, namespace string) {
		t.Helper()
		entity, err := entityRegistry.GetEntity(context.Background(), id)
		require.NoError(t, err)
		stored, ok := entity.(persistence.StoredEntity)
		require.True(t, ok)
		require.Equal(t, kind, stored.Kind())
		require.Equal(t, name, stored.Name)
		require.Equal(t, namespace, stored.Namespace)
	}

	workloadID := prof.Workloads[0].ID()
	serviceID := prof.Services[0].ID()
	ingressID := prof.Ingresses[0].ID()
	configMapID := entities.ComputeEntityID(entities.KindConfigMap, "test", "demo", "gateway-config")
	secretID := entities.ComputeEntityID(entities.KindSecret, "test", "demo", "gateway-secret")
	pvcID := entities.ComputeEntityID(entities.KindPVC, "test", "demo", "gateway-cache")

	assertEntity(workloadID, entities.KindWorkload, "gateway-go", "demo")
	assertEntity(serviceID, entities.KindService, "gateway-go", "demo")
	assertEntity(ingressID, entities.KindIngress, "gateway", "demo")
	assertEntity(configMapID, entities.KindConfigMap, "gateway-config", "demo")
	assertEntity(secretID, entities.KindSecret, "gateway-secret", "demo")
	assertEntity(pvcID, entities.KindPVC, "gateway-cache", "demo")
}
