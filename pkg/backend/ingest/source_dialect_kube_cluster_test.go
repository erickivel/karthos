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

// TestOTLPNormalizer_RawKubeClusterContainerCPURequest verifies that the
// k8s_cluster receiver's underscore wire name is translated to the dotted
// catalog canonical via the KubeClusterDialect.
func TestOTLPNormalizer_RawKubeClusterContainerCPURequest(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC).UnixNano())
	resource := map[string]string{
		"k8s.namespace.name": "default",
		"k8s.pod.name":       "test-pod",
		"k8s.container.name": "app",
	}

	rm := makeGaugeRM("k8s.container.cpu_request", 0.25, tsNano)
	env := makeEnvelope(t, resource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	expectedID := entities.ComputeEntityID(entities.KindContainer, testCluster, "default", "test-pod", "app")
	assert.Equal(t, "k8s.container.cpu.request", samples[0].MetricName)
	assert.Equal(t, expectedID, samples[0].EntityID)
	assert.InDelta(t, 250.0, samples[0].Value, 1e-9)
	assert.Equal(t, metrics.UnitMillicores, samples[0].Unit)
}

// TestOTLPNormalizer_RawKubeClusterPodPhase verifies that k8s_cluster's
// integer-encoded pod phase gauge round-trips to a KindPod entity sample.
func TestOTLPNormalizer_RawKubeClusterPodPhase(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC).UnixNano())
	resource := map[string]string{
		"k8s.namespace.name": "default",
		"k8s.pod.name":       "test-pod",
	}

	rm := makeGaugeRM("k8s.pod.phase", 2, tsNano)
	env := makeEnvelope(t, resource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	expectedID := entities.ComputeEntityID(entities.KindPod, testCluster, "default", "test-pod")
	assert.Equal(t, "k8s.pod.phase", samples[0].MetricName)
	assert.Equal(t, expectedID, samples[0].EntityID)
	assert.InDelta(t, 2.0, samples[0].Value, 1e-9)
	assert.Equal(t, metrics.UnitCount, samples[0].Unit)
}

func TestOTLPNormalizer_RawKubeClusterContainerMemoryLimit(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC).UnixNano())
	resource := map[string]string{
		"k8s.namespace.name": "default",
		"k8s.pod.name":       "test-pod",
		"k8s.container.name": "app",
	}

	rm := makeGaugeRM("k8s.container.memory_limit", 268435456, tsNano)
	env := makeEnvelope(t, resource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	expectedID := entities.ComputeEntityID(entities.KindContainer, testCluster, "default", "test-pod", "app")
	assert.Equal(t, "k8s.container.memory.limit", samples[0].MetricName)
	assert.Equal(t, expectedID, samples[0].EntityID)
	assert.InDelta(t, 268435456.0, samples[0].Value, 1e-9)
	assert.Equal(t, metrics.UnitBytes, samples[0].Unit)
}

func TestOTLPNormalizer_RawKubeClusterWorkloadMetrics(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC).UnixNano())

	cases := []struct {
		name       string
		metricName string
		resource   map[string]string
		expected   string
		entityID   entities.EntityID
		unit       metrics.Unit
	}{
		{
			name:       "job active pods",
			metricName: "k8s.job.active_pods",
			resource: map[string]string{
				"k8s.namespace.name": "batch",
				"k8s.job.name":       "daily-import",
			},
			expected: "k8s.job.active_pods",
			entityID: entities.ComputeEntityID(entities.KindWorkload, testCluster, "batch", "job", "daily-import"),
			unit:     metrics.UnitCount,
		},
		{
			name:       "cronjob active jobs",
			metricName: "k8s.cronjob.active_jobs",
			resource: map[string]string{
				"k8s.namespace.name": "batch",
				"k8s.cronjob.name":   "daily-import",
			},
			expected: "k8s.cronjob.active_jobs",
			entityID: entities.ComputeEntityID(entities.KindWorkload, testCluster, "batch", "cronjob", "daily-import"),
			unit:     metrics.UnitCount,
		},
		{
			name:       "hpa current replicas",
			metricName: "k8s.hpa.current_replicas",
			resource: map[string]string{
				"k8s.namespace.name": "default",
				"k8s.hpa.name":       "gateway",
			},
			expected: "k8s.hpa.current_replicas",
			entityID: entities.ComputeEntityID(entities.KindHPA, testCluster, "default", "gateway"),
			unit:     metrics.UnitCount,
		},
		{
			name:       "resource quota used",
			metricName: "k8s.resource_quota.used",
			resource: map[string]string{
				"k8s.namespace.name":     "default",
				"k8s.resourcequota.name": "compute",
			},
			expected: "k8s.resource_quota.used",
			entityID: entities.ComputeEntityID(entities.KindResourceQuota, testCluster, "default", "compute"),
			unit:     metrics.UnitResource,
		},
		{
			name:       "replication controller available",
			metricName: "k8s.replication_controller.available",
			resource: map[string]string{
				"k8s.namespace.name":             "default",
				"k8s.replicationcontroller.name": "legacy",
			},
			expected: "k8s.replication_controller.available",
			entityID: entities.ComputeEntityID(entities.KindWorkload, testCluster, "default", "replication_controller", "legacy"),
			unit:     metrics.UnitCount,
		},
		{
			name:       "node allocatable pods",
			metricName: "k8s.node.allocatable_pods",
			resource: map[string]string{
				"k8s.node.name": "worker-1",
			},
			expected: "k8s.node.allocatable_pods",
			entityID: entities.ComputeEntityID(entities.KindNode, testCluster, "worker-1"),
			unit:     metrics.UnitCount,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rm := makeGaugeRM(tc.metricName, 7, tsNano)
			env := makeEnvelope(t, tc.resource, rm)

			samples, err := normalizer.Normalize(context.Background(), env)
			require.NoError(t, err)
			require.Len(t, samples, 1)

			assert.Equal(t, tc.expected, samples[0].MetricName)
			assert.Equal(t, tc.entityID, samples[0].EntityID)
			assert.InDelta(t, 7.0, samples[0].Value, 1e-9)
			assert.Equal(t, tc.unit, samples[0].Unit)
		})
	}
}

func TestOTLPNormalizer_ResourceQuotaPreservesResourceAndMapsUnits(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC).UnixNano())
	resourceAttrs := map[string]string{
		"k8s.namespace.name":     "default",
		"k8s.resourcequota.name": "compute",
	}

	cases := []struct {
		name     string
		resource string
		unit     metrics.Unit
	}{
		{name: "cpu requests become millicores", resource: "requests.cpu", unit: metrics.UnitMillicores},
		{name: "memory limits become bytes", resource: "limits.memory", unit: metrics.UnitBytes},
		{name: "ephemeral storage becomes bytes", resource: "requests.ephemeral-storage", unit: metrics.UnitBytes},
		{name: "storage class quota becomes bytes", resource: "fast.storageclass.storage.k8s.io/requests.storage", unit: metrics.UnitBytes},
		{name: "huge pages become bytes", resource: "hugepages-2Mi", unit: metrics.UnitBytes},
		{name: "pod object quota stays count", resource: "pods", unit: metrics.UnitCount},
		{name: "count prefixed resources stay count", resource: "count/deployments.apps", unit: metrics.UnitCount},
		{name: "extended GPU resource stays generic", resource: "requests.nvidia.com/gpu", unit: metrics.UnitResource},
		{name: "unknown resources keep resource unit", resource: "example.com/widgets", unit: metrics.UnitResource},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rm := makeGaugeRMWithAttrs("k8s.resource_quota.used", 7, tsNano, map[string]string{
				"resource": tc.resource,
			})
			env := makeEnvelope(t, resourceAttrs, rm)

			samples, err := normalizer.Normalize(context.Background(), env)
			require.NoError(t, err)
			require.Len(t, samples, 1)

			assert.Equal(t, "k8s.resource_quota.used", samples[0].MetricName)
			assert.Equal(t, tc.resource, samples[0].Labels["resource"])
			assert.Equal(t, tc.unit, samples[0].Unit)
		})
	}
}

func TestOTLPNormalizer_NodeConditionReady(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())
	nodeResource := map[string]string{"k8s.node.name": "test-node"}

	rm := makeGaugeRMWithAttr("k8s.node.condition", 1.0, tsNano, "condition", "Ready")
	env := makeEnvelope(t, nodeResource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, "k8s.node.condition.ready", samples[0].MetricName)
}

func TestOTLPNormalizer_NodeConditionMemoryPressure(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())
	nodeResource := map[string]string{"k8s.node.name": "test-node"}

	rm := makeGaugeRMWithAttr("k8s.node.condition", 0.0, tsNano, "condition", "MemoryPressure")
	env := makeEnvelope(t, nodeResource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, "k8s.node.condition.memory_pressure", samples[0].MetricName)
}

func TestOTLPNormalizer_NodeConditionUnknownSkipped(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())
	nodeResource := map[string]string{"k8s.node.name": "test-node"}

	rm := makeGaugeRMWithAttr("k8s.node.condition", 0.0, tsNano, "condition", "SomeUnknownCondition")
	env := makeEnvelope(t, nodeResource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, samples, "unknown condition type should be skipped")
}

func TestOTLPNormalizer_DeploymentEntityResolution(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())
	deployResource := map[string]string{
		"k8s.deployment.name": "my-deployment",
		"k8s.namespace.name":  "default",
	}

	rm := makeGaugeRM("k8s.deployment.desired", 3.0, tsNano)
	env := makeEnvelope(t, deployResource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, "k8s.deployment.desired", samples[0].MetricName)
	assert.InDelta(t, 3.0, samples[0].Value, 1e-9)
}
