package ingest_test

import (
	"context"

	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
)

func TestOTLPNormalizer_KubeletstatsCP(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC).UnixNano())

	rm := makeGaugeRM("k8s.pod.cpu.usage", 0.5, tsNano)
	env := makeEnvelope(t, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t, "k8s.pod.cpu.usage", s.MetricName)
	assert.InDelta(t, 500.0, s.Value, 1e-9)
	assert.Equal(t, metrics.SourceKubelet, s.Labels["source"])
	assert.Equal(t, time.Unix(0, int64(tsNano)).UTC(), s.Timestamp)
}

func TestOTLPNormalizer_RawKubeletContainerCPUUsage(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC).UnixNano())
	resource := map[string]string{
		"k8s.namespace.name": "default",
		"k8s.pod.name":       "test-pod",
		"k8s.container.name": "app",
	}

	rm := makeGaugeRM("container.cpu.usage", 0.5, tsNano)
	env := makeEnvelope(t, resource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	expectedID := entities.ComputeEntityID(entities.KindContainer, testCluster, "default", "test-pod", "app")
	assert.Equal(t, "k8s.container.cpu.usage", samples[0].MetricName)
	assert.Equal(t, expectedID, samples[0].EntityID)
	assert.InDelta(t, 500.0, samples[0].Value, 1e-9)
	assert.Equal(t, metrics.UnitMillicores, samples[0].Unit)
}

func TestOTLPNormalizer_RawKubeletContainerMemoryUsage(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC).UnixNano())
	resource := map[string]string{
		"k8s.namespace.name": "default",
		"k8s.pod.name":       "test-pod",
		"k8s.container.name": "app",
	}

	rm := makeGaugeRM("container.memory.usage", 1048576, tsNano)
	env := makeEnvelope(t, resource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	expectedID := entities.ComputeEntityID(entities.KindContainer, testCluster, "default", "test-pod", "app")
	assert.Equal(t, "k8s.container.memory.usage", samples[0].MetricName)
	assert.Equal(t, expectedID, samples[0].EntityID)
	assert.InDelta(t, 1048576.0, samples[0].Value, 1e-9)
	assert.Equal(t, metrics.UnitBytes, samples[0].Unit)
}

// TestOTLPNormalizer_RawKubeletContainerFilesystemUsage verifies that the
// kubeletstats raw `container.filesystem.usage` wire name is translated to the
// dotted catalog canonical `k8s.container.filesystem.usage` via KubeletDialect.
func TestOTLPNormalizer_RawKubeletContainerFilesystemUsage(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC).UnixNano())
	resource := map[string]string{
		"k8s.namespace.name": "default",
		"k8s.pod.name":       "test-pod",
		"k8s.container.name": "app",
	}

	rm := makeGaugeRM("container.filesystem.usage", 4096, tsNano)
	env := makeEnvelope(t, resource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	expectedID := entities.ComputeEntityID(entities.KindContainer, testCluster, "default", "test-pod", "app")
	assert.Equal(t, "k8s.container.filesystem.usage", samples[0].MetricName)
	assert.Equal(t, expectedID, samples[0].EntityID)
	assert.InDelta(t, 4096.0, samples[0].Value, 1e-9)
	assert.Equal(t, metrics.UnitBytes, samples[0].Unit)
}

// TestOTLPNormalizer_RawKubeletPodNetworkIO verifies that the kubeletstats
// pod-scope network throughput metric is accepted as the bare canonical
// `k8s.pod.network.io` (direction is preserved as a label, not split into two
// metric names).
func TestOTLPNormalizer_RawKubeletPodNetworkIO(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC).UnixNano())

	rm := makeGaugeRMWithAttrs("k8s.pod.network.io", 1500, tsNano, map[string]string{
		"direction": "receive",
		"interface": "eth0",
	})
	env := makeEnvelope(t, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	assert.Equal(t, "k8s.pod.network.io", samples[0].MetricName)
	assert.InDelta(t, 1500.0, samples[0].Value, 1e-9)
	assert.Equal(t, metrics.UnitBytes, samples[0].Unit)
	assert.Equal(t, "receive", samples[0].Labels["direction"])
	assert.Equal(t, "eth0", samples[0].Labels["network_interface_name"])
}

func TestOTLPNormalizer_VolumeMetricPreservesVolumeLabelsAndResolvesPVC(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	resource := podResource()
	resource["k8s.persistentvolumeclaim.name"] = "bundle-cache"
	resource["k8s.volume.name"] = "data"
	resource["k8s.volume.type"] = "persistentVolumeClaim"
	rm := makeGaugeRM("k8s.volume.capacity", 1024, tsNano)
	env := makeEnvelope(t, resource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t, "k8s.volume.capacity", s.MetricName)
	assert.Equal(t, entities.ComputeEntityID(entities.KindPVC, testCluster, "default", "bundle-cache"), s.EntityID)
	assert.Equal(t, "bundle-cache", s.Labels["persistentvolumeclaim_name"])
	assert.Equal(t, "data", s.Labels["volume_name"])
	assert.Equal(t, "persistentVolumeClaim", s.Labels["volume_type"])
	assert.NotContains(t, s.Labels, "k8s.persistentvolumeclaim.name")
}

func TestOTLPNormalizer_NonPVCVolumeStaysOnPodWithIdentityLabels(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())
	resource := podResource()
	resource["k8s.volume.name"] = "kube-api-access-x7n2p"
	resource["k8s.volume.type"] = "projected"

	samples, err := normalizer.Normalize(context.Background(), makeEnvelope(t, resource, makeGaugeRM("k8s.volume.capacity", 512, tsNano)))
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t, entities.ComputeEntityID(entities.KindPod, testCluster, "default", "test-pod"), s.EntityID)
	assert.Equal(t, "kube-api-access-x7n2p", s.Labels["volume_name"])
	assert.Equal(t, "projected", s.Labels["volume_type"])
	assert.NotContains(t, s.Labels, "persistentvolumeclaim_name")
}
