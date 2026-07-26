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

type testPodLifecycle struct {
	allows bool
}

func (l testPodLifecycle) AllowsRunsOn(_, _, _, _ string) bool {
	return l.allows
}

type togglePodLifecycle struct {
	allows bool
}

func (l *togglePodLifecycle) AllowsRunsOn(_, _, _, _ string) bool {
	return l.allows
}

func TestMetricTopologyBuilder_RequiresActiveMatchingPodLifecycleForRunsOn(t *testing.T) {
	env := makeMetricEnvelope("kubeletstats", map[string]string{
		"k8s.pod.name":       "my-pod",
		"k8s.pod.uid":        "pod-uid-a",
		"k8s.namespace.name": "default",
		"k8s.node.name":      "my-node",
	})

	active := NewMetricTopologyBuilderWithLifecycle(testCluster, testPodLifecycle{allows: true})
	activeEdges, err := active.Build(context.Background(), env)
	require.NoError(t, err)
	assert.Len(t, activeEdges, 2)
	assert.Equal(t, topology.RelRunsOn, activeEdges[0].Relation)

	retired := NewMetricTopologyBuilderWithLifecycle(testCluster, testPodLifecycle{allows: false})
	retiredEdges, err := retired.Build(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, retiredEdges, 1)
	assert.Equal(t, topology.RelContains, retiredEdges[0].Relation)
}

func TestMetricTopologyBuilderVersionsRunsOnIdentityByPodUID(t *testing.T) {
	builder := NewMetricTopologyBuilderWithLifecycle(testCluster, testPodLifecycle{allows: true})
	resource := map[string]string{
		"k8s.pod.name":       "job-0",
		"k8s.namespace.name": "default",
		"k8s.node.name":      "node-a",
	}

	old := makeMetricEnvelope("kubeletstats", cloneStringMap(resource))
	old.Resource["k8s.pod.uid"] = "uid-old"
	new := makeMetricEnvelope("kubeletstats", cloneStringMap(resource))
	new.Resource["k8s.pod.uid"] = "uid-new"

	oldEdges, err := builder.Build(context.Background(), old)
	require.NoError(t, err)
	newEdges, err := builder.Build(context.Background(), new)
	require.NoError(t, err)

	oldRunsOn := findRunsOnEdge(t, oldEdges)
	newRunsOn := findRunsOnEdge(t, newEdges)
	assert.Equal(t, oldRunsOn.Source, newRunsOn.Source)
	assert.Equal(t, oldRunsOn.Target, newRunsOn.Target)
	assert.NotEqual(t, oldRunsOn.ID, newRunsOn.ID)
	assert.Equal(t, testCluster, oldRunsOn.Metadata["k8s.cluster.name"])
	assert.Equal(t, testCluster, newRunsOn.Metadata["k8s.cluster.name"])
	assert.Equal(t, "uid-old", oldRunsOn.Metadata["k8s.pod.uid"])
	assert.Equal(t, "uid-new", newRunsOn.Metadata["k8s.pod.uid"])
}

func TestBufferedTopologyIngestor_RetiredUIDCannotAdvanceRunsOnLastSeen(t *testing.T) {
	baseline := time.Date(2026, 7, 20, 12, 9, 11, 0, time.UTC)
	later := baseline.Add(time.Minute)
	lifecycle := &togglePodLifecycle{allows: true}
	builder := NewMetricTopologyBuilderWithLifecycle(testCluster, lifecycle)
	store := &recordingGraphStore{}
	buffer := NewBufferedTopologyIngestor(
		map[schema.SignalType]TopologyBuilder{schema.SignalMetric: builder},
		store,
		nil,
		time.Hour,
	)
	defer buffer.Close()

	resource := map[string]string{
		"k8s.pod.name":       "my-pod",
		"k8s.pod.uid":        "retired-uid",
		"k8s.namespace.name": "default",
		"k8s.node.name":      "my-node",
	}
	first, err := schema.NewRawEnvelope("kubeletstats", schema.SignalMetric, resource, []byte(`{}`), baseline)
	require.NoError(t, err)
	require.NoError(t, buffer.Ingest(context.Background(), first))

	lifecycle.allows = false
	stale, err := schema.NewRawEnvelope("kubeletstats", schema.SignalMetric, resource, []byte(`{}`), later)
	require.NoError(t, err)
	require.NoError(t, buffer.Ingest(context.Background(), stale))
	buffer.Flush()

	for _, edge := range store.last().Graph.Edges {
		if edge.Relation == topology.RelRunsOn {
			assert.Equal(t, baseline, edge.LastSeen)
			return
		}
	}
	t.Fatal("expected retained runs_on edge")
}

func findRunsOnEdge(t *testing.T, edges []topology.Edge) topology.Edge {
	t.Helper()
	for _, edge := range edges {
		if edge.Relation == topology.RelRunsOn {
			return edge
		}
	}
	t.Fatal("expected runs_on edge")
	return topology.Edge{}
}

func cloneStringMap(input map[string]string) map[string]string {
	clone := make(map[string]string, len(input))
	for key, value := range input {
		clone[key] = value
	}
	return clone
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

func TestMetricTopologyBuilder_BuildNodesIncludesFilterLabels(t *testing.T) {
	b := NewMetricTopologyBuilder(testCluster)
	env := makeMetricEnvelope("kubeletstats", map[string]string{
		"k8s.pod.name":       "my-pod",
		"k8s.namespace.name": "default",
		"k8s.node.name":      "my-node",
		"k8s.container.name": "my-container",
	})

	nodes := b.BuildNodes(context.Background(), env)
	require.Len(t, nodes, 4)

	byID := map[entities.EntityID]topology.Node{}
	for _, node := range nodes {
		byID[node.ID] = node
	}

	podID := entities.ComputeEntityID(entities.KindPod, testCluster, "default", "my-pod")
	containerID := entities.ComputeEntityID(entities.KindContainer, testCluster, "default", "my-pod", "my-container")
	namespaceID := entities.ComputeEntityID(entities.KindNamespace, testCluster, "default")

	assert.Equal(t, "default", byID[podID].Labels["namespace"])
	assert.Equal(t, "my-pod", byID[podID].Labels["name"])
	assert.Equal(t, "default", byID[containerID].Labels["namespace"])
	assert.Equal(t, "my-container", byID[containerID].Labels["name"])
	assert.Equal(t, "default", byID[namespaceID].Labels["name"])
}
