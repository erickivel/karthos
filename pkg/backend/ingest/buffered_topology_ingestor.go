package ingest

import (
	"context"
	"sync"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
)

// BufferedTopologyIngestor accumulates topology edges and nodes in memory
// and flushes one consolidated snapshot to GraphStore on a configurable interval.
// It implements the Ingestor interface.
type BufferedTopologyIngestor struct {
	builders map[schema.SignalType]TopologyBuilder
	store    persistence.GraphStore
	logger   *logging.Logger

	mu    sync.Mutex
	edges map[string]topology.Edge
	nodes map[string]topology.Node
	from  time.Time
	to    time.Time
	dirty bool

	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
}

// NewBufferedTopologyIngestor creates a BufferedTopologyIngestor and starts
// the background flush goroutine.
func NewBufferedTopologyIngestor(
	builders map[schema.SignalType]TopologyBuilder,
	store persistence.GraphStore,
	logger *logging.Logger,
	interval time.Duration,
) *BufferedTopologyIngestor {
	b := &BufferedTopologyIngestor{
		builders: builders,
		store:    store,
		logger:   logger,
		edges:    make(map[string]topology.Edge),
		nodes:    make(map[string]topology.Node),
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go b.run()
	return b
}

// Ingest builds edges and nodes from the envelope using the registered builder
// for the envelope's signal type, then merges them into the in-memory buffer.
func (b *BufferedTopologyIngestor) Ingest(ctx context.Context, envelope schema.RawEnvelope) error {
	builder, ok := b.builders[envelope.SignalType]
	if !ok {
		return nil
	}

	edges, err := builder.Build(ctx, envelope)
	if err != nil {
		return err
	}

	var nodes []topology.Node
	if nb, ok := builder.(NodeBuilder); ok {
		nodes = nb.BuildNodes(ctx, envelope)
	}

	if len(edges) == 0 && len(nodes) == 0 {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for _, e := range edges {
		if existing, ok := b.edges[e.ID]; ok {
			if e.LastSeen.After(existing.LastSeen) {
				existing.LastSeen = e.LastSeen
			}
			existing.Confidence = e.Confidence
			existing.Provenance = e.Provenance
			existing.Metadata = e.Metadata
			b.edges[e.ID] = existing
		} else {
			b.edges[e.ID] = e
		}
	}

	for _, n := range nodes {
		b.nodes[string(n.ID)] = n
	}

	ts := envelope.Timestamp
	if b.from.IsZero() || ts.Before(b.from) {
		b.from = ts
	}
	if ts.After(b.to) {
		b.to = ts
	}
	for _, e := range edges {
		if e.LastSeen.After(b.to) {
			b.to = e.LastSeen
		}
	}

	b.dirty = true
	return nil
}

// Flush writes the accumulated buffer as a single consolidated snapshot.
// Safe to call from any goroutine.
func (b *BufferedTopologyIngestor) Flush() {
	b.mu.Lock()
	if !b.dirty {
		b.mu.Unlock()
		return
	}

	edges := b.edges
	nodes := b.nodes
	from := b.from
	to := b.to

	b.edges = make(map[string]topology.Edge)
	b.nodes = make(map[string]topology.Node)
	b.from = time.Time{}
	b.to = time.Time{}
	b.dirty = false
	b.mu.Unlock()

	edgeSlice := make([]topology.Edge, 0, len(edges))
	for _, e := range edges {
		edgeSlice = append(edgeSlice, e)
	}
	nodeSlice := make([]topology.Node, 0, len(nodes))
	for _, n := range nodes {
		nodeSlice = append(nodeSlice, n)
	}

	snapshot := topology.Snapshot{
		Version: schema.CurrentVersion,
		Graph: topology.Graph{
			Nodes:     nodeSlice,
			Edges:     edgeSlice,
			Timestamp: to,
		},
		From:      from,
		To:        to,
		CreatedAt: time.Now().UTC(),
	}

	if err := b.store.PutSnapshot(context.Background(), snapshot); err != nil {
		if b.logger != nil {
			b.logger.Error("flush topology snapshot failed", logging.Error(err))
		}
	} else if b.logger != nil {
		b.logger.Info("flushed topology snapshot",
			logging.Any("edges", len(edgeSlice)),
			logging.Any("nodes", len(nodeSlice)),
		)
	}
}

// Close stops the background flush goroutine and performs a final flush.
func (b *BufferedTopologyIngestor) Close() {
	close(b.stop)
	<-b.done
	b.Flush()
}

func (b *BufferedTopologyIngestor) run() {
	defer close(b.done)
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.Flush()
		case <-b.stop:
			return
		}
	}
}
