package ingest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/karthos/pkg/core/schema"
	"github.com/erickivel/karthos/pkg/core/topology"
)

// recordingGraphStore records observed edge batches for test assertions.
type recordingGraphStore struct {
	mu        sync.Mutex
	snapshots []topology.Snapshot
}

type failOnceGraphStore struct {
	recordingGraphStore
	mu       sync.Mutex
	failed   bool
	failWith error
}

type blockingFailOnceGraphStore struct {
	recordingGraphStore
	mu       sync.Mutex
	failed   bool
	started  chan struct{}
	release  chan struct{}
	failWith error
}

func (s *blockingFailOnceGraphStore) PutObservedEdges(ctx context.Context, edges []topology.Edge) error {
	s.mu.Lock()
	if !s.failed {
		s.failed = true
		s.mu.Unlock()
		close(s.started)
		<-s.release
		return s.failWith
	}
	s.mu.Unlock()
	return s.recordingGraphStore.PutObservedEdges(ctx, edges)
}

func (s *failOnceGraphStore) PutObservedEdges(ctx context.Context, edges []topology.Edge) error {
	s.mu.Lock()
	if !s.failed {
		s.failed = true
		err := s.failWith
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return s.recordingGraphStore.PutObservedEdges(ctx, edges)
}

func (r *recordingGraphStore) PutObservedEdges(_ context.Context, edges []topology.Edge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.snapshots = append(r.snapshots, topology.Snapshot{Graph: topology.Graph{Edges: edges}})
	return nil
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

// configuredBuilder implements TopologyBuilder.
type configuredBuilder struct {
	edges      []topology.Edge
	errOnBuild error
}

func (b *configuredBuilder) Build(_ context.Context, _ schema.RawEnvelope) ([]topology.Edge, error) {
	if b.errOnBuild != nil {
		return nil, b.errOnBuild
	}
	return b.edges, nil
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
	assert.Equal(t, 0, store.count(), "no observed edge batch before flush")

	builder.edges = []topology.Edge{edge2}
	env2 := newEnvelope(t, schema.SignalMetric, now.Add(time.Second))
	require.NoError(t, buf.Ingest(context.Background(), env2))
	assert.Equal(t, 0, store.count(), "still no observed edge batch before flush")

	buf.Flush()

	assert.Equal(t, 1, store.count(), "exactly one observed edge batch after flush")
	snap := store.last()
	assert.Len(t, snap.Graph.Edges, 2, "both edges in consolidated snapshot")
}

func TestBufferedTopologyIngestor_RequeuesFailedBatchWithLaterObservations(t *testing.T) {
	store := &failOnceGraphStore{failWith: errors.New("graph store unavailable")}
	first := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	later := first.Add(time.Minute)
	firstEdge := topology.NewEdge("pod-a", "node-1", topology.RelRunsOn, first)
	firstEdge.Metadata = map[string]string{"revision": "old"}
	laterEdge := topology.NewEdge("pod-a", "node-1", topology.RelRunsOn, later)
	laterEdge.Metadata = map[string]string{"revision": "new"}
	additionalEdge := topology.NewEdge("pod-b", "node-1", topology.RelRunsOn, later)
	builder := &configuredBuilder{edges: []topology.Edge{firstEdge}}

	buf := NewBufferedTopologyIngestor(
		map[schema.SignalType]TopologyBuilder{schema.SignalMetric: builder},
		store,
		nil,
		time.Hour,
	)
	defer buf.Close()

	require.NoError(t, buf.Ingest(context.Background(), newEnvelope(t, schema.SignalMetric, first)))
	buf.Flush()
	require.Zero(t, store.count(), "the first store attempt fails")

	builder.edges = []topology.Edge{laterEdge, additionalEdge}
	require.NoError(t, buf.Ingest(context.Background(), newEnvelope(t, schema.SignalMetric, later)))
	buf.Flush()

	require.Equal(t, 1, store.count())
	batch := store.last()
	require.Len(t, batch.Graph.Edges, 2)
	for _, edge := range batch.Graph.Edges {
		if edge.ID != firstEdge.ID {
			continue
		}
		assert.Equal(t, first, edge.FirstSeen)
		assert.Equal(t, later, edge.LastSeen)
		assert.Equal(t, "new", edge.Metadata["revision"])
	}
}

func TestBufferedTopologyIngestor_RequeueDoesNotOverwriteConcurrentNewerObservation(t *testing.T) {
	store := &blockingFailOnceGraphStore{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		failWith: errors.New("graph store unavailable"),
	}
	first := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	later := first.Add(time.Minute)
	firstEdge := topology.NewEdge("pod-a", "node-1", topology.RelRunsOn, first)
	firstEdge.Metadata = map[string]string{"revision": "old"}
	laterEdge := topology.NewEdge("pod-a", "node-1", topology.RelRunsOn, later)
	laterEdge.Metadata = map[string]string{"revision": "new"}
	builder := &configuredBuilder{edges: []topology.Edge{firstEdge}}
	buf := NewBufferedTopologyIngestor(map[schema.SignalType]TopologyBuilder{schema.SignalMetric: builder}, store, nil, time.Hour)
	defer buf.Close()

	require.NoError(t, buf.Ingest(context.Background(), newEnvelope(t, schema.SignalMetric, first)))
	flushed := make(chan struct{})
	go func() {
		buf.Flush()
		close(flushed)
	}()
	<-store.started

	builder.edges = []topology.Edge{laterEdge}
	require.NoError(t, buf.Ingest(context.Background(), newEnvelope(t, schema.SignalMetric, later)))
	close(store.release)
	<-flushed

	buf.Flush()
	require.Equal(t, 1, store.count())
	edge := store.last().Graph.Edges[0]
	assert.Equal(t, first, edge.FirstSeen)
	assert.Equal(t, later, edge.LastSeen)
	assert.Equal(t, "new", edge.Metadata["revision"])
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

func TestBufferedTopologyIngestor_NoDirtyData_FlushIsNoop(t *testing.T) {
	store := &recordingGraphStore{}
	builders := map[schema.SignalType]TopologyBuilder{
		schema.SignalMetric: &configuredBuilder{},
	}

	buf := NewBufferedTopologyIngestor(builders, store, nil, time.Hour)
	defer buf.Close()

	buf.Flush()

	assert.Equal(t, 0, store.count(), "flush with no ingested data should not write an observed edge batch")
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

func TestBufferedTopologyIngestor_Close_FinalFlush(t *testing.T) {
	store := &recordingGraphStore{}
	now := time.Now().UTC()

	builder := &configuredBuilder{
		edges: []topology.Edge{topology.NewEdge("pod-a", "node-1", topology.RelRunsOn, now)},
	}
	builders := map[schema.SignalType]TopologyBuilder{
		schema.SignalMetric: builder,
	}

	buf := NewBufferedTopologyIngestor(builders, store, nil, time.Hour)

	env := newEnvelope(t, schema.SignalMetric, now)
	require.NoError(t, buf.Ingest(context.Background(), env))
	assert.Equal(t, 0, store.count(), "no flush yet")

	buf.Close()

	assert.Equal(t, 1, store.count(), "Close should trigger final flush")
}

func TestBufferedTopologyIngestor_PeriodicFlush(t *testing.T) {
	store := &recordingGraphStore{}
	now := time.Now().UTC()

	builder := &configuredBuilder{
		edges: []topology.Edge{topology.NewEdge("pod-a", "node-1", topology.RelRunsOn, now)},
	}
	builders := map[schema.SignalType]TopologyBuilder{
		schema.SignalMetric: builder,
	}

	buf := NewBufferedTopologyIngestor(builders, store, nil, 50*time.Millisecond)
	defer buf.Close()

	env := newEnvelope(t, schema.SignalMetric, now)
	require.NoError(t, buf.Ingest(context.Background(), env))

	// Wait for at least one tick to fire.
	require.Eventually(t, func() bool {
		return store.count() > 0
	}, time.Second, 10*time.Millisecond, "periodic flush should write an observed edge batch")
}
