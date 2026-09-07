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

// BufferedTopologyIngestor accumulates observed topology edges in memory and
// flushes them to current observation state on a configurable interval.
// It implements the Ingestor interface.
type BufferedTopologyIngestor struct {
	builders map[schema.SignalType]TopologyBuilder
	store    persistence.ObservationEdgeStore
	logger   *logging.Logger

	mu    sync.Mutex
	edges map[string]topology.Edge
	dirty bool

	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
}

// NewBufferedTopologyIngestor creates a BufferedTopologyIngestor and starts
// the background flush goroutine.
func NewBufferedTopologyIngestor(
	builders map[schema.SignalType]TopologyBuilder,
	store persistence.ObservationEdgeStore,
	logger *logging.Logger,
	interval time.Duration,
) *BufferedTopologyIngestor {
	b := &BufferedTopologyIngestor{
		builders: builders,
		store:    store,
		logger:   logger,
		edges:    make(map[string]topology.Edge),
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go b.run()
	return b
}

// Ingest builds observed edges from the envelope using the registered builder
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

	if len(edges) == 0 {
		return nil
	}

	b.mu.Lock()
	b.mergeLocked(edges)
	b.mu.Unlock()
	return nil
}

// Flush writes the accumulated observed edges as one consolidated batch.
// Safe to call from any goroutine.
func (b *BufferedTopologyIngestor) Flush() {
	b.mu.Lock()
	if !b.dirty {
		b.mu.Unlock()
		return
	}

	edges := b.edges
	b.edges = make(map[string]topology.Edge)
	b.dirty = false
	b.mu.Unlock()

	edgeSlice := make([]topology.Edge, 0, len(edges))
	for _, e := range edges {
		edgeSlice = append(edgeSlice, e)
	}
	if err := b.store.PutObservedEdges(context.Background(), edgeSlice); err != nil {
		b.mu.Lock()
		b.mergeLocked(edgeSlice)
		b.mu.Unlock()
		if b.logger != nil {
			b.logger.Error("flush observed topology edges failed", logging.Error(err))
		}
	} else if b.logger != nil {
		b.logger.Info("flushed observed topology edges",
			logging.Any("edges", len(edgeSlice)),
		)
	}
}

func (b *BufferedTopologyIngestor) mergeLocked(edges []topology.Edge) {
	for _, edge := range edges {
		if existing, ok := b.edges[edge.ID]; ok {
			if existing.FirstSeen.IsZero() || (!edge.FirstSeen.IsZero() && edge.FirstSeen.Before(existing.FirstSeen)) {
				existing.FirstSeen = edge.FirstSeen
			}
			if !edge.LastSeen.Before(existing.LastSeen) {
				existing.LastSeen = edge.LastSeen
				existing.Confidence = edge.Confidence
				existing.Metadata = mergeStringMaps(existing.Metadata, edge.Metadata)
			} else {
				existing.Metadata = mergeStringMaps(edge.Metadata, existing.Metadata)
			}
			existing.Provenance = mergeStringSlices(existing.Provenance, edge.Provenance)
			b.edges[edge.ID] = existing
			continue
		}
		b.edges[edge.ID] = edge
	}
	b.dirty = true
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
