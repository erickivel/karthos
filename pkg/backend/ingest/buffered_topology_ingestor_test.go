package ingest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
)

// recordingGraphStore records PutSnapshot calls for test assertions.
type recordingGraphStore struct {
	mu        sync.Mutex
	snapshots []topology.Snapshot
}

func (r *recordingGraphStore) PutSnapshot(_ context.Context, snapshot topology.Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.snapshots = append(r.snapshots, snapshot)
	return nil
}

func (r *recordingGraphStore) GetLatest(_ context.Context) (*topology.Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.snapshots) == 0 {
		return nil, nil
	}
	s := r.snapshots[len(r.snapshots)-1]
	return &s, nil
}

func (r *recordingGraphStore) QueryEdges(_ context.Context, _ query.EdgeQuery) ([]topology.Edge, error) {
	return nil, nil
}

func (r *recordingGraphStore) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.snapshots)
}

func (r *recordingGraphStore) last() topology.Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshots[len(r.snapshots)-1]
}

// configuredBuilder implements TopologyBuilder and optionally NodeBuilder.
type configuredBuilder struct {
	edges      []topology.Edge
	nodes      []topology.Node
	errOnBuild error
}

func (b *configuredBuilder) Build(_ context.Context, _ schema.RawEnvelope) ([]topology.Edge, error) {
	if b.errOnBuild != nil {
		return nil, b.errOnBuild
	}
	return b.edges, nil
}

func (b *configuredBuilder) BuildNodes(_ context.Context, _ schema.RawEnvelope) []topology.Node {
	return b.nodes
}

func newEnvelope(t *testing.T, signal schema.SignalType, ts time.Time) schema.RawEnvelope {
	t.Helper()
	env, err := schema.NewRawEnvelope("test", signal, map[string]string{"k8s.pod.name": "pod-1"}, []byte(`{}`), ts)
	require.NoError(t, err)
	return env
}

func TestBufferedTopologyIngestor_AccumulatesEdges(t *testing.T) {
	store := &recordingGraphStore{}
	now := time.Now().UTC()

	edge1 := topology.NewEdge("pod-a", "node-1", topology.RelRunsOn, now)
	edge2 := topology.NewEdge("pod-b", "node-1", topology.RelRunsOn, now)

	builder := &configuredBuilder{edges: []topology.Edge{edge1}}
	builders := map[schema.SignalType]TopologyBuilder{
		schema.SignalMetric: builder,
	}

	buf := NewBufferedTopologyIngestor(builders, store, nil, time.Hour)
	defer buf.Close()

	env1 := newEnvelope(t, schema.SignalMetric, now)
	require.NoError(t, buf.Ingest(context.Background(), env1))
	assert.Equal(t, 0, store.count(), "no PutSnapshot before flush")

	builder.edges = []topology.Edge{edge2}
	env2 := newEnvelope(t, schema.SignalMetric, now.Add(time.Second))
	require.NoError(t, buf.Ingest(context.Background(), env2))
	assert.Equal(t, 0, store.count(), "still no PutSnapshot before flush")

	buf.Flush()

	assert.Equal(t, 1, store.count(), "exactly one PutSnapshot after flush")
	snap := store.last()
	assert.Len(t, snap.Graph.Edges, 2, "both edges in consolidated snapshot")
}

func TestBufferedTopologyIngestor_EdgeMerging_LastSeenUpdated(t *testing.T) {
	store := &recordingGraphStore{}
	earlier := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)

	edge := topology.NewEdge("pod-a", "node-1", topology.RelRunsOn, earlier)

	builder := &configuredBuilder{edges: []topology.Edge{edge}}
	builders := map[schema.SignalType]TopologyBuilder{
		schema.SignalMetric: builder,
	}

	buf := NewBufferedTopologyIngestor(builders, store, nil, time.Hour)
	defer buf.Close()

	env1 := newEnvelope(t, schema.SignalMetric, earlier)
	require.NoError(t, buf.Ingest(context.Background(), env1))

	laterEdge := topology.NewEdge("pod-a", "node-1", topology.RelRunsOn, later)
	builder.edges = []topology.Edge{laterEdge}
	env2 := newEnvelope(t, schema.SignalMetric, later)
	require.NoError(t, buf.Ingest(context.Background(), env2))

	buf.Flush()

	require.Equal(t, 1, store.count())
	snap := store.last()
	require.Len(t, snap.Graph.Edges, 1, "same edge ID should merge")
	assert.Equal(t, later, snap.Graph.Edges[0].LastSeen, "LastSeen should be the later value")
	assert.Equal(t, earlier, snap.Graph.Edges[0].FirstSeen, "FirstSeen should be the earlier value")
}

func TestBufferedTopologyIngestor_NodeDeduplication(t *testing.T) {
	store := &recordingGraphStore{}
	now := time.Now().UTC()

	nodeA := topology.Node{ID: entities.EntityID("node-1"), Kind: "Node"}
	nodeB := topology.Node{ID: entities.EntityID("pod-1"), Kind: "Pod"}

	builder := &configuredBuilder{
		edges: []topology.Edge{topology.NewEdge("pod-1", "node-1", topology.RelRunsOn, now)},
		nodes: []topology.Node{nodeA, nodeB},
	}
	builders := map[schema.SignalType]TopologyBuilder{
		schema.SignalMetric: builder,
	}

	buf := NewBufferedTopologyIngestor(builders, store, nil, time.Hour)
	defer buf.Close()

	env1 := newEnvelope(t, schema.SignalMetric, now)
	require.NoError(t, buf.Ingest(context.Background(), env1))
	env2 := newEnvelope(t, schema.SignalMetric, now.Add(time.Second))
	require.NoError(t, buf.Ingest(context.Background(), env2))

	buf.Flush()

	require.Equal(t, 1, store.count())
	snap := store.last()
	assert.Len(t, snap.Graph.Nodes, 2, "nodes should be deduplicated by ID")
}

func TestBufferedTopologyIngestor_NoDirtyData_FlushIsNoop(t *testing.T) {
	store := &recordingGraphStore{}
	builders := map[schema.SignalType]TopologyBuilder{
		schema.SignalMetric: &configuredBuilder{},
	}

	buf := NewBufferedTopologyIngestor(builders, store, nil, time.Hour)
	defer buf.Close()

	buf.Flush()

	assert.Equal(t, 0, store.count(), "flush with no ingested data should not call PutSnapshot")
}

func TestBufferedTopologyIngestor_SignalTypeRouting(t *testing.T) {
	store := &recordingGraphStore{}
	now := time.Now().UTC()

	metricEdge := topology.NewEdge("pod-a", "node-1", topology.RelRunsOn, now)
	traceEdge := topology.NewEdge("svc-a", "svc-b", topology.RelCalls, now)

	metricBuilder := &configuredBuilder{edges: []topology.Edge{metricEdge}}
	traceBuilder := &configuredBuilder{edges: []topology.Edge{traceEdge}}

	builders := map[schema.SignalType]TopologyBuilder{
		schema.SignalMetric: metricBuilder,
		schema.SignalTrace:  traceBuilder,
	}

	buf := NewBufferedTopologyIngestor(builders, store, nil, time.Hour)
	defer buf.Close()

	metricEnv := newEnvelope(t, schema.SignalMetric, now)
	require.NoError(t, buf.Ingest(context.Background(), metricEnv))

	traceEnv := newEnvelope(t, schema.SignalTrace, now)
	require.NoError(t, buf.Ingest(context.Background(), traceEnv))

	buf.Flush()

	require.Equal(t, 1, store.count())
	snap := store.last()
	assert.Len(t, snap.Graph.Edges, 2, "edges from both signal types in one snapshot")

	relations := map[topology.RelationType]bool{}
	for _, e := range snap.Graph.Edges {
		relations[e.Relation] = true
	}
	assert.True(t, relations[topology.RelRunsOn], "metric edge present")
	assert.True(t, relations[topology.RelCalls], "trace edge present")
}

func TestBufferedTopologyIngestor_UnknownSignalType_Skipped(t *testing.T) {
	store := &recordingGraphStore{}
	builders := map[schema.SignalType]TopologyBuilder{
		schema.SignalMetric: &configuredBuilder{
			edges: []topology.Edge{topology.NewEdge("a", "b", topology.RelRunsOn, time.Now().UTC())},
		},
	}

	buf := NewBufferedTopologyIngestor(builders, store, nil, time.Hour)
	defer buf.Close()

	env := newEnvelope(t, schema.SignalTrace, time.Now().UTC())
	err := buf.Ingest(context.Background(), env)
	require.NoError(t, err, "unknown signal should not error")

	buf.Flush()
	assert.Equal(t, 0, store.count(), "no data buffered for unknown signal type")
}

func TestBufferedTopologyIngestor_BuilderError_ReturnsError(t *testing.T) {
	store := &recordingGraphStore{}
	builder := &configuredBuilder{errOnBuild: assert.AnError}
	builders := map[schema.SignalType]TopologyBuilder{
		schema.SignalMetric: builder,
	}

	buf := NewBufferedTopologyIngestor(builders, store, nil, time.Hour)
	defer buf.Close()

	env := newEnvelope(t, schema.SignalMetric, time.Now().UTC())
	err := buf.Ingest(context.Background(), env)
	require.Error(t, err, "builder error should propagate")

	buf.Flush()
	assert.Equal(t, 0, store.count(), "nothing buffered when builder errors")
}
