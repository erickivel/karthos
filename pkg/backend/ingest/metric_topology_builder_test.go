package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
)

func makeMetricEnvelope(source string, resource map[string]string) schema.RawEnvelope {
	env, _ := schema.NewRawEnvelope(source, schema.SignalMetric, resource, []byte(`{}`), time.Now().UTC())
	return env
}

func TestMetricTopologyBuilder_PodRunsOnNode(t *testing.T) {
	b := NewMetricTopologyBuilder(testCluster)
	env := makeMetricEnvelope("kubeletstats", map[string]string{
		"k8s.pod.name":       "my-pod",
		"k8s.namespace.name": "default",
		"k8s.node.name":      "my-node",
	})

	edges, err := b.Build(context.Background(), env)
	require.NoError(t, err)

	var runsOn *topology.Edge
	for _, e := range edges {
		if e.Relation == topology.RelRunsOn {
			e := e
			runsOn = &e
		}
	}
	require.NotNil(t, runsOn, "expected a runs_on edge")

	expectedPodID := entities.ComputeEntityID(entities.KindPod, testCluster, "default", "my-pod")
	expectedNodeID := entities.ComputeEntityID(entities.KindNode, testCluster, "my-node")
	assert.Equal(t, expectedPodID, runsOn.Source)
	assert.Equal(t, expectedNodeID, runsOn.Target)
	assert.Equal(t, float64(1.0), runsOn.Confidence)
	assert.Equal(t, []string{"kubeletstats"}, runsOn.Provenance)
}

func TestMetricTopologyBuilder_NamespaceContainsPod(t *testing.T) {
	b := NewMetricTopologyBuilder(testCluster)
	env := makeMetricEnvelope("kubeletstats", map[string]string{
		"k8s.pod.name":       "my-pod",
		"k8s.namespace.name": "default",
	})

	edges, err := b.Build(context.Background(), env)
	require.NoError(t, err)

	var containsEdge *topology.Edge
	for _, e := range edges {
		if e.Relation == topology.RelContains {
			e := e
			containsEdge = &e
		}
	}
	require.NotNil(t, containsEdge, "expected a contains edge")

	expectedNsID := entities.ComputeEntityID(entities.KindNamespace, testCluster, "default")
	expectedPodID := entities.ComputeEntityID(entities.KindPod, testCluster, "default", "my-pod")
	assert.Equal(t, expectedNsID, containsEdge.Source)
	assert.Equal(t, expectedPodID, containsEdge.Target)
}

func TestMetricTopologyBuilder_PodContainsContainer(t *testing.T) {
	b := NewMetricTopologyBuilder(testCluster)
	env := makeMetricEnvelope("kubeletstats", map[string]string{
		"k8s.pod.name":       "my-pod",
		"k8s.namespace.name": "default",
		"k8s.container.name": "my-container",
	})

	edges, err := b.Build(context.Background(), env)
	require.NoError(t, err)

	expectedPodID := entities.ComputeEntityID(entities.KindPod, testCluster, "default", "my-pod")
	expectedContainerID := entities.ComputeEntityID(entities.KindContainer, testCluster, "default", "my-pod", "my-container")

	// Find the contains edge whose source is the pod (not the namespace→pod edge)
	var podContains *topology.Edge
	for _, e := range edges {
		if e.Relation == topology.RelContains && e.Source == expectedPodID {
			e := e
			podContains = &e
		}
	}
	require.NotNil(t, podContains, "expected a pod contains container edge")
	assert.Equal(t, expectedPodID, podContains.Source)
	assert.Equal(t, expectedContainerID, podContains.Target)
}

func TestMetricTopologyBuilder_AllAttributes_ProducesAllEdges(t *testing.T) {
	b := NewMetricTopologyBuilder(testCluster)
	env := makeMetricEnvelope("kubeletstats", map[string]string{
		"k8s.pod.name":       "my-pod",
		"k8s.namespace.name": "default",
		"k8s.node.name":      "my-node",
		"k8s.container.name": "my-container",
	})

	edges, err := b.Build(context.Background(), env)
	require.NoError(t, err)

	// Expect: runs_on (pod→node), contains (ns→pod), contains (pod→container)
	assert.Len(t, edges, 3)

	relations := make(map[topology.RelationType]int)
	for _, e := range edges {
		relations[e.Relation]++
	}
	assert.Equal(t, 1, relations[topology.RelRunsOn])
	assert.Equal(t, 2, relations[topology.RelContains])
}

func TestMetricTopologyBuilder_NoStructuralAttributes_ReturnsNil(t *testing.T) {
	b := NewMetricTopologyBuilder(testCluster)
	env := makeMetricEnvelope("kubeletstats", map[string]string{
		"service.name": "some-service",
	})

	edges, err := b.Build(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, edges)
}

func TestMetricTopologyBuilder_ZeroTimestamp_UsesCurrentTime(t *testing.T) {
	b := NewMetricTopologyBuilder(testCluster)
	// Construct envelope directly to bypass validation that rejects zero timestamps.
	env := schema.RawEnvelope{
		Source:     "kubeletstats",
		SignalType: schema.SignalMetric,
		Resource: map[string]string{
			"k8s.pod.name":       "my-pod",
			"k8s.namespace.name": "default",
			"k8s.node.name":      "my-node",
		},
	}

	before := time.Now().UTC()
	edges, err := b.Build(context.Background(), env)
	after := time.Now().UTC()

	require.NoError(t, err)
	require.NotEmpty(t, edges)
	for _, e := range edges {
		assert.False(t, e.LastSeen.IsZero(), "edge timestamp should not be zero")
		assert.True(t, !e.LastSeen.Before(before) && !e.LastSeen.After(after),
			"edge timestamp should be between before and after Build() call")
	}
}

func TestMetricTopologyBuilder_EmptyEnvelope_ReturnsNil(t *testing.T) {
	b := NewMetricTopologyBuilder(testCluster)
	env := makeMetricEnvelope("kubeletstats", nil)

	edges, err := b.Build(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, edges)
}
