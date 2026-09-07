package ingest

import (
	"context"
	"time"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/core/topology"
)

// MetricTopologyBuilder extracts the observed runs_on relation from OTLP metric
// envelope resource attributes. Direct Kubernetes structure refresh owns all
// structural nodes and contains relations.
//
// It produces a pod runs_on node edge when the corresponding attributes are
// present.
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

	return edges, nil
}
