package persistence

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/schema"
)

func makeEnvelope(source string, signalType schema.SignalType, ts time.Time) schema.RawEnvelope {
	return schema.RawEnvelope{
		ID:         "test-id",
		Version:    schema.CurrentVersion,
		Source:     source,
		SignalType: signalType,
		Timestamp:  ts,
		ReceivedAt: ts,
		Payload:    json.RawMessage(`{}`),
	}
}

func TestRawStore_WriteAndQueryAll(t *testing.T) {
	store := NewInMemoryRawStore()
	ctx := context.Background()

	now := time.Now()
	envelopes := []schema.RawEnvelope{
		makeEnvelope("otlp-grpc", schema.SignalMetric, now),
		makeEnvelope("prometheus", schema.SignalMetric, now),
		makeEnvelope("otlp-grpc", schema.SignalTrace, now),
	}

	require.NoError(t, store.WriteRaw(ctx, envelopes))

	got, err := store.QueryRaw(ctx, RawQuery{})
	require.NoError(t, err)
	assert.Len(t, got, 3)
}

func TestRawStore_FilterBySignalType(t *testing.T) {
	store := NewInMemoryRawStore()
	ctx := context.Background()

	now := time.Now()
	require.NoError(t, store.WriteRaw(ctx, []schema.RawEnvelope{
		makeEnvelope("otlp-grpc", schema.SignalMetric, now),
		makeEnvelope("otlp-grpc", schema.SignalTrace, now),
		makeEnvelope("otlp-grpc", schema.SignalMetric, now),
	}))

	got, err := store.QueryRaw(ctx, RawQuery{SignalType: schema.SignalTrace})
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, schema.SignalTrace, got[0].SignalType)
}

func TestRawStore_FilterBySource(t *testing.T) {
	store := NewInMemoryRawStore()
	ctx := context.Background()

	now := time.Now()
	require.NoError(t, store.WriteRaw(ctx, []schema.RawEnvelope{
		makeEnvelope("otlp-grpc", schema.SignalMetric, now),
		makeEnvelope("prometheus", schema.SignalMetric, now),
		makeEnvelope("otlp-grpc", schema.SignalMetric, now),
	}))

	got, err := store.QueryRaw(ctx, RawQuery{Source: "prometheus"})
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, "prometheus", got[0].Source)
}

func TestRawStore_FilterByTimeRange(t *testing.T) {
	store := NewInMemoryRawStore()
	ctx := context.Background()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, store.WriteRaw(ctx, []schema.RawEnvelope{
		makeEnvelope("otlp", schema.SignalMetric, base),
		makeEnvelope("otlp", schema.SignalMetric, base.Add(time.Minute)),
		makeEnvelope("otlp", schema.SignalMetric, base.Add(2*time.Minute)),
	}))

	got, err := store.QueryRaw(ctx, RawQuery{
		From: base.Add(30 * time.Second),
		To:   base.Add(90 * time.Second),
	})
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestRawStore_EmptyReturnsNil(t *testing.T) {
	store := NewInMemoryRawStore()
	ctx := context.Background()

	got, err := store.QueryRaw(ctx, RawQuery{})
	assert.NoError(t, err)
	assert.Nil(t, got)
}
