package ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

// StructuralTopologyIngestor ingests K8s structural topology from a ClusterProfile
// into the graph store.
type StructuralTopologyIngestor struct {
	builder StructuralTopologyBuilder
	store   persistence.GraphStore
}

func NewStructuralTopologyIngestor(builder StructuralTopologyBuilder, store persistence.GraphStore) *StructuralTopologyIngestor {
	return &StructuralTopologyIngestor{builder: builder, store: store}
}

func (i *StructuralTopologyIngestor) IngestProfile(ctx context.Context, prof *profile.ClusterProfile) error {
	edges, nodes, err := i.builder.BuildStructural(ctx, prof)
	if err != nil {
		return fmt.Errorf("build structural topology: %w", err)
	}
	if len(edges) == 0 && len(nodes) == 0 {
		return nil
	}

	now := time.Now().UTC()
	snapshot := topology.Snapshot{
		Version: schema.CurrentVersion,
		Graph: topology.Graph{
			Nodes:     nodes,
			Edges:     edges,
			Timestamp: now,
		},
		From:      prof.DiscoveredAt,
		To:        now,
		CreatedAt: now,
	}
	return i.store.PutSnapshot(ctx, snapshot)
}
