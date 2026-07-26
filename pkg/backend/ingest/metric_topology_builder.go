package ingest

import (
	"context"
	"time"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/core/topology"
)

// MetricTopologyBuilder extracts structural topology edges from OTLP metric
// envelope resource attributes. It does not parse the metric payload — the
// resource attribute map carries enough information to derive runs_on and
// contains relations from kubeletstats and similar receivers.
//
// It produces the following edges when the corresponding attributes are present:
//   - pod runs_on node     (k8s.pod.name + k8s.namespace.name + k8s.node.name)
//   - namespace contains pod   (k8s.namespace.name + k8s.pod.name)
//   - pod contains container   (k8s.pod.name + k8s.namespace.name + k8s.container.name)
type MetricTopologyBuilder struct {
	clusterName  string
	podLifecycle PodLifecycle
}

// PodLifecycle decides whether telemetry may refresh a pod's runs_on edge.
// Implementations must fail closed while Kubernetes lifecycle state is unknown.
type PodLifecycle interface {
	AllowsRunsOn(namespace, podName, podUID, nodeName string) bool
}

// NewMetricTopologyBuilder creates a MetricTopologyBuilder scoped to the given cluster.
func NewMetricTopologyBuilder(clusterName string) *MetricTopologyBuilder {
	return &MetricTopologyBuilder{clusterName: clusterName}
}

// NewMetricTopologyBuilderWithLifecycle creates a builder that validates
// runs_on edges against live Kubernetes pod lifecycle state.
func NewMetricTopologyBuilderWithLifecycle(clusterName string, podLifecycle PodLifecycle) *MetricTopologyBuilder {
	return &MetricTopologyBuilder{clusterName: clusterName, podLifecycle: podLifecycle}
}

// Build derives structural topology edges from the envelope's resource attribute map.
// It satisfies the TopologyBuilder interface.
func (b *MetricTopologyBuilder) Build(_ context.Context, envelope schema.RawEnvelope) ([]topology.Edge, error) {
	res := envelope.Resource
	podName := res[semconv.ResourceK8sPodName]
	podUID := res[semconv.ResourceK8sPodUID]
	nodeName := res[semconv.ResourceK8sNodeName]
	namespace := res[semconv.ResourceK8sNamespaceName]
	containerName := res[semconv.ResourceK8sContainerName]

	t := envelope.Timestamp
	if t.IsZero() {
		t = time.Now().UTC()
	}

	var edges []topology.Edge

	// pod runs_on node
	if podName != "" && namespace != "" && nodeName != "" &&
		(b.podLifecycle == nil || b.podLifecycle.AllowsRunsOn(namespace, podName, podUID, nodeName)) {
		podID := entities.ComputeEntityID(entities.KindPod, b.clusterName, namespace, podName)
		nodeID := entities.ComputeEntityID(entities.KindNode, b.clusterName, nodeName)
		e := topology.NewEdge(podID, nodeID, topology.RelRunsOn, t)
		if podUID != "" {
			e.ID = topology.ComputeEdgeIDWithDiscriminator(podID, nodeID, topology.RelRunsOn, podUID)
			e.Metadata = map[string]string{
				"k8s.cluster.name": b.clusterName,
				"k8s.pod.uid":      podUID,
			}
		}
		e.Provenance = []string{envelope.Source}
		edges = append(edges, e)
	}

	// namespace contains pod
	if namespace != "" && podName != "" {
		nsID := entities.ComputeEntityID(entities.KindNamespace, b.clusterName, namespace)
		podID := entities.ComputeEntityID(entities.KindPod, b.clusterName, namespace, podName)
		e := topology.NewEdge(nsID, podID, topology.RelContains, t)
		e.Provenance = []string{envelope.Source}
		edges = append(edges, e)
	}

	// pod contains container
	if podName != "" && namespace != "" && containerName != "" {
		podID := entities.ComputeEntityID(entities.KindPod, b.clusterName, namespace, podName)
		containerID := entities.ComputeEntityID(entities.KindContainer, b.clusterName, namespace, podName, containerName)
		e := topology.NewEdge(podID, containerID, topology.RelContains, t)
		e.Provenance = []string{envelope.Source}
		edges = append(edges, e)
	}

	return edges, nil
}

// BuildNodes derives topology nodes from the envelope's resource attributes.
// It is called in addition to Build when both structural edges and nodes are needed.
func (b *MetricTopologyBuilder) BuildNodes(_ context.Context, envelope schema.RawEnvelope) []topology.Node {
	res := envelope.Resource
	podName := res[semconv.ResourceK8sPodName]
	nodeName := res[semconv.ResourceK8sNodeName]
	namespace := res[semconv.ResourceK8sNamespaceName]
	containerName := res[semconv.ResourceK8sContainerName]

	seen := make(map[entities.EntityID]topology.Node)

	if namespace != "" {
		id := entities.ComputeEntityID(entities.KindNamespace, b.clusterName, namespace)
		seen[id] = topology.Node{
			ID:   id,
			Kind: entities.KindNamespace,
			Labels: map[string]string{
				"name": namespace,
			},
		}
	}
	if nodeName != "" {
		id := entities.ComputeEntityID(entities.KindNode, b.clusterName, nodeName)
		seen[id] = topology.Node{
			ID:   id,
			Kind: entities.KindNode,
			Labels: map[string]string{
				"name": nodeName,
			},
		}
	}
	if podName != "" && namespace != "" {
		id := entities.ComputeEntityID(entities.KindPod, b.clusterName, namespace, podName)
		seen[id] = topology.Node{
			ID:   id,
			Kind: entities.KindPod,
			Labels: map[string]string{
				"name":      podName,
				"namespace": namespace,
			},
		}
	}
	if containerName != "" && podName != "" && namespace != "" {
		id := entities.ComputeEntityID(entities.KindContainer, b.clusterName, namespace, podName, containerName)
		seen[id] = topology.Node{
			ID:   id,
			Kind: entities.KindContainer,
			Labels: map[string]string{
				"name":      containerName,
				"namespace": namespace,
				"pod":       podName,
			},
		}
	}

	if len(seen) == 0 {
		return nil
	}
	nodes := make([]topology.Node, 0, len(seen))
	for _, n := range seen {
		nodes = append(nodes, n)
	}
	return nodes
}
