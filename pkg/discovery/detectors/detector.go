package detectors

import (
	"context"

	"github.com/erickivel/oksg/pkg/discovery/profile"
)

type Detector interface {
	Name() string
	Detect(ctx context.Context) (profile.ObservabilitySource, error)
}
