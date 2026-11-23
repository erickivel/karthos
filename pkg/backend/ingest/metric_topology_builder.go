package ingest

import (
	"context"
	"time"

	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/schema"
	"github.com/erickivel/karthos/pkg/core/semconv"
	"github.com/erickivel/karthos/pkg/core/topology"
	"github.com/erickivel/karthos/pkg/sources"
	"github.com/erickivel/karthos/pkg/sources/contract"
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
	sourcePolicy *MetricSourcePolicy
	modules      sources.Registry
}

// PodLifecycle decides whether telemetry may refresh a pod's runs_on edge.
// Implementations must fail closed while Kubernetes lifecycle state is unknown.
type PodLifecycle interface {
	AllowsRunsOn(namespace, podName, podUID, nodeName string) bool
}

// NewMetricTopologyBuilder creates a MetricTopologyBuilder scoped to the given cluster.
func NewMetricTopologyBuilder(clusterName string, policy ...*MetricSourcePolicy) *MetricTopologyBuilder {
	return newMetricTopologyBuilder(clusterName, nil, policy...)
}

// NewMetricTopologyBuilderWithLifecycle creates a builder that validates
// runs_on edges against live Kubernetes pod lifecycle state.
func NewMetricTopologyBuilderWithLifecycle(clusterName string, podLifecycle PodLifecycle, policy ...*MetricSourcePolicy) *MetricTopologyBuilder {
	return newMetricTopologyBuilder(clusterName, podLifecycle, policy...)
}

func newMetricTopologyBuilder(clusterName string, lifecycle PodLifecycle, policies ...*MetricSourcePolicy) *MetricTopologyBuilder {
	modules := sources.DefaultRegistry()
	selected := NewMetricSourcePolicy(modules.MetricIDs(), nil)
	if len(policies) > 0 && policies[0] != nil {
		selected = policies[0]
	}
	return &MetricTopologyBuilder{clusterName: clusterName, podLifecycle: lifecycle, sourcePolicy: selected, modules: modules}
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
		// Resource stamps are accepted for direct producers. Collector-generated
		// stamps live on datapoints, so inspect those independently below.
		if source := res[semconv.AttrKarthosSource]; source != "" {
			if module, ok := b.modules.Lookup(source); ok && module.MetricSource {
				e.Provenance = []string{source}
				edges = append(edges, e)
			}
		}
		var rm metricspb.ResourceMetrics
		if len(envelope.Payload) > 0 {
			if err := protojson.Unmarshal(envelope.Payload, &rm); err != nil {
				return nil, err
			}
		}
		bySource := map[string]topology.Edge{}
		for _, scope := range rm.ScopeMetrics {
			for _, metric := range scope.Metrics {
				for _, dp := range metricAttributeSets(metric) {
					if dp.TimeUnixNano == 0 {
						continue
					}
					attrs := dp.Attributes
					parsed, ok := b.modules.ParseMetric(sourceFromAttrs(attrs), metric.Name, contract.AttributeStrings(attrs))
					if !ok || !b.sourcePolicy.Validate(parsed.CanonicalName, attrs).Accepted() {
						continue
					}
					source := sourceFromAttrs(attrs)
					observed := time.Unix(0, int64(dp.TimeUnixNano)).UTC() // #nosec G115 -- OTLP nanoseconds fit until year 2262
					current, exists := bySource[source]
					if !exists {
						current = e
						current.Provenance = []string{source}
						current.FirstSeen = observed
						current.LastSeen = observed
					}
					if observed.Before(current.FirstSeen) {
						current.FirstSeen = observed
					}
					if observed.After(current.LastSeen) {
						current.LastSeen = observed
					}
					bySource[source] = current
				}
			}
		}
		for _, observed := range bySource {
			edges = append(edges, observed)
		}
	}

	return edges, nil
}
