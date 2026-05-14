package ingest

import (
	"context"
	"testing"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCluster = "test-cluster"

func TestOTLPEntityResolver_Container(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.container.name": "my-container",
		"k8s.pod.name":       "my-pod",
		"k8s.namespace.name": "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindContainer, testCluster, "my-namespace", "my-pod", "my-container")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_Container_Deterministic(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.container.name": "my-container",
		"k8s.pod.name":       "my-pod",
		"k8s.namespace.name": "my-namespace",
	}

	id1, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)

	id2, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)

	assert.Equal(t, id1, id2)
}

func TestOTLPEntityResolver_Pod(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.pod.name":       "my-pod",
		"k8s.namespace.name": "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindPod, testCluster, "my-namespace", "my-pod")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_Pod_Deterministic(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.pod.name":       "my-pod",
		"k8s.namespace.name": "my-namespace",
	}

	id1, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)

	id2, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)

	assert.Equal(t, id1, id2)
}

func TestOTLPEntityResolver_Node(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.node.name": "my-node",
	}

	expected := entities.ComputeEntityID(entities.KindNode, testCluster, "my-node")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_Node_Deterministic(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.node.name": "my-node",
	}

	id1, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)

	id2, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)

	assert.Equal(t, id1, id2)
}

func TestOTLPEntityResolver_ServiceWithNamespace(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"service.name":       "my-service",
		"k8s.namespace.name": "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindService, testCluster, "my-namespace", "my-service")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_ServiceWithNamespace_Deterministic(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"service.name":       "my-service",
		"k8s.namespace.name": "my-namespace",
	}

	id1, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)

	id2, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)

	assert.Equal(t, id1, id2)
}

func TestOTLPEntityResolver_ServiceBestEffort(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"service.name": "my-service",
	}

	expected := entities.ComputeEntityID(entities.KindService, testCluster, "my-service")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_ServiceBestEffort_Deterministic(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"service.name": "my-service",
	}

	id1, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)

	id2, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)

	assert.Equal(t, id1, id2)
}

func TestOTLPEntityResolver_ServiceBestEffortPrefersCanonicalNamespacedRegistryEntity(t *testing.T) {
	registry := persistence.NewInMemoryEntityRegistry()
	require.NoError(t, registry.UpsertEntity(context.Background(), persistence.StoredEntity{
		EntityID:   entities.ComputeEntityID(entities.KindService, testCluster, "oksg-demo", "my-service"),
		EntityKind: entities.KindService,
		Cluster:    testCluster,
		Namespace:  "oksg-demo",
		Name:       "my-service",
		Metadata: map[string]string{
			"service.name":       "my-service",
			"k8s.namespace.name": "oksg-demo",
		},
	}))

	resolver := NewOTLPEntityResolverWithRegistry(testCluster, registry)

	id, err := resolver.Resolve(context.Background(), map[string]string{
		"service.name": "my-service",
	})
	require.NoError(t, err)
	assert.Equal(t, entities.ComputeEntityID(entities.KindService, testCluster, "oksg-demo", "my-service"), id)
}

func TestOTLPEntityResolver_Deployment(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.deployment.name": "my-deployment",
		"k8s.namespace.name":  "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindWorkload, testCluster, "my-namespace", "deployment", "my-deployment")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_Deployment_Deterministic(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.deployment.name": "my-deployment",
		"k8s.namespace.name":  "my-namespace",
	}

	id1, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)

	id2, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)

	assert.Equal(t, id1, id2)
}

func TestOTLPEntityResolver_StatefulSet(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.statefulset.name": "my-statefulset",
		"k8s.namespace.name":   "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindWorkload, testCluster, "my-namespace", "statefulset", "my-statefulset")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_ReplicaSet(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.replicaset.name": "my-replicaset",
		"k8s.namespace.name":  "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindWorkload, testCluster, "my-namespace", "replicaset", "my-replicaset")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_DaemonSet(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.daemonset.name": "my-daemonset",
		"k8s.namespace.name": "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindWorkload, testCluster, "my-namespace", "daemonset", "my-daemonset")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_Job(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.job.name":       "my-job",
		"k8s.namespace.name": "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindWorkload, testCluster, "my-namespace", "job", "my-job")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_CronJob(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.cronjob.name":   "my-cronjob",
		"k8s.namespace.name": "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindWorkload, testCluster, "my-namespace", "cronjob", "my-cronjob")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_HPA(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.hpa.name":       "my-hpa",
		"k8s.namespace.name": "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindHPA, testCluster, "my-namespace", "my-hpa")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_ReplicationController(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.replicationcontroller.name": "my-rc",
		"k8s.namespace.name":             "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindWorkload, testCluster, "my-namespace", "replication_controller", "my-rc")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_ResourceQuota(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.resourcequota.name": "my-quota",
		"k8s.namespace.name":     "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindResourceQuota, testCluster, "my-namespace", "my-quota")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_NamespaceOnly(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.namespace.name": "my-namespace",
		"k8s.namespace.uid":  "abc-123",
	}

	expected := entities.ComputeEntityID(entities.KindNamespace, testCluster, "my-namespace")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_DeploymentWinsOverReplicaSet(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.deployment.name": "my-deployment",
		"k8s.replicaset.name": "my-deployment-abc123",
		"k8s.namespace.name":  "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindWorkload, testCluster, "my-namespace", "deployment", "my-deployment")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_PodWinsOverDeployment(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"k8s.pod.name":        "my-pod-abc123",
		"k8s.deployment.name": "my-deployment",
		"k8s.namespace.name":  "my-namespace",
	}

	expected := entities.ComputeEntityID(entities.KindPod, testCluster, "my-namespace", "my-pod-abc123")

	id, err := resolver.Resolve(context.Background(), resource)
	require.NoError(t, err)
	assert.Equal(t, expected, id)
}

func TestOTLPEntityResolver_EmptyResource(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{}

	_, err := resolver.Resolve(context.Background(), resource)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot resolve entity")
}

func TestOTLPEntityResolver_UnrecognizedResource(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	resource := map[string]string{
		"some.unknown.key":    "value",
		"another.unknown.key": "other-value",
	}

	_, err := resolver.Resolve(context.Background(), resource)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot resolve entity")
}
