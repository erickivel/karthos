package ingest

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/schema"
)

type trackingIngestor struct {
	name   string
	calls  *[]string
	retErr error
}

func (r *trackingIngestor) Ingest(_ context.Context, _ schema.RawEnvelope) error {
	*r.calls = append(*r.calls, r.name)
	return r.retErr
}

func TestMultiIngestor_CallsAllInOrder(t *testing.T) {
	var calls []string
	a := &trackingIngestor{name: "a", calls: &calls}
	b := &trackingIngestor{name: "b", calls: &calls}
	m := NewMultiIngestor(a, b)

	err := m.Ingest(context.Background(), schema.RawEnvelope{})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, calls)
}

func TestMultiIngestor_CallsAllEvenOnError(t *testing.T) {
	var calls []string
	errA := errors.New("ingestor-a error")
	a := &trackingIngestor{name: "a", calls: &calls, retErr: errA}
	b := &trackingIngestor{name: "b", calls: &calls}
	m := NewMultiIngestor(a, b)

	err := m.Ingest(context.Background(), schema.RawEnvelope{})
	// Both were called
	assert.Equal(t, []string{"a", "b"}, calls)
	// First error is returned
	assert.ErrorIs(t, err, errA)
}

func TestMultiIngestor_NoIngestors_ReturnsNil(t *testing.T) {
	m := NewMultiIngestor()
	err := m.Ingest(context.Background(), schema.RawEnvelope{})
	require.NoError(t, err)
}
