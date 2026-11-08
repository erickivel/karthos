package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/karthos/pkg/backend/persistence"
	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/core/topology"
)

func TestHealthResolverPreservesContainerReadinessPresence(t *testing.T) {
	now := time.Now().UTC()
	store := &recordingMetricStore{samples: map[string][]metrics.MetricSample{
		"k8s.container.ready": {
			{MetricName: "k8s.container.ready", EntityID: "ready", Value: 1, Timestamp: now},
			{MetricName: "k8s.container.ready", EntityID: "not-ready", Value: 0, Timestamp: now},
		},
		"k8s.container.restarts": {
			{MetricName: "k8s.container.restarts", EntityID: "ready", Value: 0, Timestamp: now},
			{MetricName: "k8s.container.restarts", EntityID: "not-ready", Value: 0, Timestamp: now},
			{MetricName: "k8s.container.restarts", EntityID: "restarting", Value: 1, Timestamp: now},
		},
	}}
	resolution := NewHealthResolver(store).Resolve(context.Background(), []topology.Node{
		{ID: "ready", Kind: entities.KindContainer},
		{ID: "not-ready", Kind: entities.KindContainer},
		{ID: "unknown", Kind: entities.KindContainer},
		{ID: "restarting", Kind: entities.KindContainer},
	}, false)

	require.Equal(t, healthQualityComplete, resolution.Containers)
	require.Equal(t, metrics.HealthHealthy, resolution.Statuses["ready"])
	require.Equal(t, metrics.HealthRunningNotReady, resolution.Statuses["not-ready"])
	require.Equal(t, metrics.HealthUnknown, resolution.Statuses["unknown"])
	require.Equal(t, metrics.HealthRestarting, resolution.Statuses["restarting"])
}

func TestHealthResolverUsesGraphInstant(t *testing.T) {
	store := persistence.NewInMemoryMetricStore()
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	id := entities.EntityID("pod-historical")
	require.NoError(t, store.Write(context.Background(), []metrics.MetricSample{
		{MetricName: "k8s.pod.phase", EntityID: id, Value: 2, Timestamp: at},
		{MetricName: "k8s.pod.phase", EntityID: id, Value: 4, Timestamp: at.Add(10 * time.Minute)},
	}))
	resolution := NewHealthResolver(store).ResolveAt(context.Background(), []topology.Node{{ID: id, Kind: entities.KindPod}}, true, at)
	require.Equal(t, metrics.HealthHealthy, resolution.Statuses[id])
	later := NewHealthResolver(store).ResolveAt(context.Background(), []topology.Node{{ID: id, Kind: entities.KindPod}}, true, at.Add(10*time.Minute))
	require.Equal(t, metrics.HealthFailed, later.Statuses[id])
}

func TestCombineHealthQualityPreservesPartialTruth(t *testing.T) {
	tests := []struct {
		name      string
		qualities []healthQuality
		expected  healthQuality
	}{
		{name: "all complete", qualities: []healthQuality{healthQualityComplete, healthQualityComplete}, expected: healthQualityComplete},
		{name: "all unavailable", qualities: []healthQuality{healthQualityUnavailable, healthQualityUnavailable}, expected: healthQualityUnavailable},
		{name: "explicit partial", qualities: []healthQuality{healthQualityComplete, healthQualityPartial}, expected: healthQualityPartial},
		{name: "complete unavailable mix", qualities: []healthQuality{healthQualityComplete, healthQualityUnavailable}, expected: healthQualityPartial},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, combineHealthQuality(tt.qualities...))
		})
	}
}
