package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
)

// stubBuilder implements TopologyBuilder and optionally NodeBuilder.
type stubBuilder struct {
	edges []topology.Edge
	nodes []topology.Node
}

func (s *stubBuilder) Build(_ context.Context, _ schema.RawEnvelope) ([]topology.Edge, error) {
	return s.edges, nil
}

func (s *stubBuilder) BuildNodes(_ context.Context, _ schema.RawEnvelope) []topology.Node {
	return s.nodes
}

func TestTopologyIngestor_NodesOnly_StoresSnapshot(t *testing.T) {
	store := persistence.NewInMemoryGraphStore()
	builder := &stubBuilder{
		edges: nil,
		nodes: []topology.Node{
			{ID: "node/test-node", Kind: "Node"},
			{ID: "namespace/default", Kind: "Namespace"},
		},
	}

	ingestor := NewTopologyIngestor(builder, store)
	env, err := schema.NewRawEnvelope("kubeletstats", schema.SignalMetric, map[string]string{
		"k8s.node.name": "test-node",
	}, []byte(`{}`), time.Now().UTC())
	require.NoError(t, err)

	err = ingestor.Ingest(context.Background(), env)
	require.NoError(t, err)

	snap, err := store.GetLatest(context.Background())
	require.NoError(t, err)
	require.NotNil(t, snap, "expected snapshot to be stored for node-only batch")
	assert.Len(t, snap.Graph.Nodes, 2)
	assert.Empty(t, snap.Graph.Edges)
}

func TestTopologyIngestor_NoEdgesNoNodes_Skips(t *testing.T) {
	store := persistence.NewInMemoryGraphStore()
	builder := &stubBuilder{edges: nil, nodes: nil}

	ingestor := NewTopologyIngestor(builder, store)
	env, err := schema.NewRawEnvelope("kubeletstats", schema.SignalMetric, map[string]string{}, []byte(`{}`), time.Now().UTC())
	require.NoError(t, err)

	err = ingestor.Ingest(context.Background(), env)
	require.NoError(t, err)

	snap, err := store.GetLatest(context.Background())
	require.NoError(t, err)
	assert.Nil(t, snap, "expected no snapshot when both edges and nodes are empty")
}
