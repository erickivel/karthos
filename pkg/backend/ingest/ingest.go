package ingest

import (
	"context"

	"github.com/erickivel/oksg/pkg/core/schema"
)

type Ingestor interface {
	Ingest(ctx context.Context, envelope schema.RawEnvelope) error
}
