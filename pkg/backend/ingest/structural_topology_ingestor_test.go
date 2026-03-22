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
	ingestor := NewStructuralTopologyIngestor(builder, store)

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
	ingestor := NewStructuralTopologyIngestor(builder, store)

	err := ingestor.IngestProfile(context.Background(), prof)
	require.NoError(t, err)
	snapshot, err := store.GetLatest(context.Background())
	require.NoError(t, err)
	require.Nil(t, snapshot)
}
