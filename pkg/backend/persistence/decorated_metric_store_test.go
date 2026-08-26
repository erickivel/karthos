package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/metrics"
)

func TestDecorateMetricStoreAddsEnumLabelsWithoutChangingStoredValues(t *testing.T) {
	base := &recordingMetricStore{samples: []metrics.MetricSample{{
		MetricName: "k8s.pod.status_reason", Value: 6, Timestamp: time.Now().UTC(),
	}}}
	store := DecorateMetricStore(base)

	got, err := store.Query(context.Background(), query.MetricQuery{MetricName: "k8s.pod.status_reason"})

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "unknown", got[0].ValueLabel)
	assert.Empty(t, base.samples[0].ValueLabel)
}

type recordingMetricStore struct{ samples []metrics.MetricSample }

func (s *recordingMetricStore) Write(_ context.Context, samples []metrics.MetricSample) error {
	s.samples = samples
	return nil
}

func (s *recordingMetricStore) Query(_ context.Context, _ query.MetricQuery) ([]metrics.MetricSample, error) {
	return s.samples, nil
}

func (s *recordingMetricStore) QueryCatalogStats(_ context.Context, _ time.Duration) ([]CatalogStat, error) {
	return nil, nil
}
