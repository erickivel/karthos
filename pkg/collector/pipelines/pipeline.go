package pipelines

import (
	"context"

	"github.com/erickivel/oksg/pkg/core/schema"
)

type Processor interface {
	Process(ctx context.Context, in schema.RawEnvelope) ([]schema.RawEnvelope, error)
}

type Pipeline interface {
	Run(ctx context.Context, input <-chan schema.RawEnvelope) error
}
