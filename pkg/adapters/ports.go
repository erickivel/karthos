package adapters

import (
	"context"

	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

type DiscoveryAdapter interface {
	Name() string
	Discover(ctx context.Context) (*profile.ClusterProfile, error)
}

type SourceAdapter interface {
	Name() string
	Collect(ctx context.Context) (<-chan schema.RawEnvelope, error)
	Health(ctx context.Context) error
}
