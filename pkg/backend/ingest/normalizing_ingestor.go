package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
)

// NormalizingIngestor implements Ingestor by normalizing metric envelopes and writing to MetricStore.
type NormalizingIngestor struct {
	normalizer Normalizer
	store      persistence.MetricStore
	logger     *logging.Logger
}

func NewNormalizingIngestor(normalizer Normalizer, store persistence.MetricStore, logger *logging.Logger) *NormalizingIngestor {
	return &NormalizingIngestor{normalizer: normalizer, store: store, logger: logger}
}

func (i *NormalizingIngestor) Ingest(ctx context.Context, envelope schema.RawEnvelope) error {
	samples, err := i.normalizer.Normalize(ctx, envelope)
	if err != nil {
		// Envelopes whose resource attributes cannot be resolved to a canonical entity
		// are expected (e.g. cluster-level metrics with no k8s.* or service.* attributes).
		// Log a warning and skip rather than propagating a hard error that would cause the
		// upstream OTel collector to drop the entire batch as a permanent failure.
		if errors.Is(err, ErrUnresolvableEntity) {
			if i.logger != nil {
				i.logger.Warn("skipping envelope: unresolvable entity",
					logging.String("source", envelope.Source),
					logging.Any("resource", envelope.Resource),
					logging.Error(err),
				)
			}
			return nil
		}
		return fmt.Errorf("normalize envelope: %w", err)
	}
	if len(samples) == 0 {
		return nil
	}
	if err := i.store.Write(ctx, samples); err != nil {
		return err
	}
	if i.logger != nil {
		i.logger.Info("wrote metric samples",
			logging.String("source", envelope.Source),
			logging.Any("count", len(samples)),
		)
	}
	return nil
}

// TopologyIngestor implements Ingestor by building topology edges from trace envelopes.
type TopologyIngestor struct {
	builder TopologyBuilder
	store   persistence.GraphStore
}

func NewTopologyIngestor(builder TopologyBuilder, store persistence.GraphStore) *TopologyIngestor {
	return &TopologyIngestor{builder: builder, store: store}
}

func (i *TopologyIngestor) Ingest(ctx context.Context, envelope schema.RawEnvelope) error {
	edges, err := i.builder.Build(ctx, envelope)
	if err != nil {
		return fmt.Errorf("build topology: %w", err)
	}
	if len(edges) == 0 {
		return nil
	}
	snapshot := buildSnapshotFromEdges(edges, envelope)
	return i.store.PutSnapshot(ctx, snapshot)
}

// buildSnapshotFromEdges creates a topology.Snapshot from a slice of edges,
// using the min FirstSeen and max LastSeen across edges for the time window.
func buildSnapshotFromEdges(edges []topology.Edge, envelope schema.RawEnvelope) topology.Snapshot {
	from := envelope.Timestamp
	to := envelope.Timestamp

	for _, e := range edges {
		if !e.FirstSeen.IsZero() && e.FirstSeen.Before(from) {
			from = e.FirstSeen
		}
		if e.LastSeen.After(to) {
			to = e.LastSeen
		}
	}

	return topology.Snapshot{
		Version: schema.CurrentVersion,
		Graph: topology.Graph{
			Edges:     edges,
			Timestamp: to,
		},
		From:      from,
		To:        to,
		CreatedAt: time.Now().UTC(),
	}
}
