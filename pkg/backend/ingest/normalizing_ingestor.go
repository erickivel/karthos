package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/schema"
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

// MultiIngestor fans out an envelope to multiple Ingestor implementations.
// All ingestors are called even when earlier ones return errors; the first
// non-nil error encountered is returned after all ingestors have run.
type MultiIngestor struct {
	ingestors []Ingestor
}

// NewMultiIngestor wraps the provided ingestors into a single fan-out Ingestor.
func NewMultiIngestor(ingestors ...Ingestor) *MultiIngestor {
	return &MultiIngestor{ingestors: ingestors}
}

func (m *MultiIngestor) Ingest(ctx context.Context, envelope schema.RawEnvelope) error {
	var firstErr error
	for _, ing := range m.ingestors {
		if err := ing.Ingest(ctx, envelope); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
