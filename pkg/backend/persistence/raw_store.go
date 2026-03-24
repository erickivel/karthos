package persistence

import (
	"context"
	"time"

	"github.com/erickivel/oksg/pkg/core/schema"
)

// RawQuery defines filters for raw envelope retrieval.
type RawQuery struct {
	SignalType schema.SignalType
	Source     string
	From       time.Time
	To         time.Time
}

// RawStore persists and queries raw telemetry envelopes for debug and replay.
// Implementations should support time-based retention policies.
type RawStore interface {
	WriteRaw(ctx context.Context, envelopes []schema.RawEnvelope) error
	QueryRaw(ctx context.Context, q RawQuery) ([]schema.RawEnvelope, error)
}
