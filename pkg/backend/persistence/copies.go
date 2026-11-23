package persistence

import (
	"sort"

	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/core/topology"
	"github.com/erickivel/karthos/pkg/discovery/profile"
)

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneMetricSample(sample metrics.MetricSample) metrics.MetricSample {
	cloned := sample
	cloned.Labels = cloneStringMap(sample.Labels)
	return cloned
}

func cloneMetricSamples(samples []metrics.MetricSample) []metrics.MetricSample {
	cloned := make([]metrics.MetricSample, len(samples))
	for i := range samples {
		cloned[i] = cloneMetricSample(samples[i])
	}
	return cloned
}

func cloneNode(node topology.Node) topology.Node {
	cloned := node
	cloned.Labels = cloneStringMap(node.Labels)
	cloned.Provenance = append([]string(nil), node.Provenance...)
	return cloned
}

func cloneEdge(edge topology.Edge) topology.Edge {
	cloned := edge
	cloned.Metadata = cloneStringMap(edge.Metadata)
	cloned.Provenance = append([]string(nil), edge.Provenance...)
	return cloned
}

func cloneSnapshot(snapshot topology.Snapshot) topology.Snapshot {
	cloned := snapshot
	cloned.Graph.Nodes = make([]topology.Node, len(snapshot.Graph.Nodes))
	for i := range snapshot.Graph.Nodes {
		cloned.Graph.Nodes[i] = cloneNode(snapshot.Graph.Nodes[i])
	}
	cloned.Graph.Edges = make([]topology.Edge, len(snapshot.Graph.Edges))
	for i := range snapshot.Graph.Edges {
		cloned.Graph.Edges[i] = cloneEdge(snapshot.Graph.Edges[i])
	}
	return cloned
}

// MergeObservedEdge keeps the temporal history of an observation edge while
// letting the newest observation own its descriptive payload. runs_on
// lifecycle edges have separate retirement semantics and are not merged here.
func MergeObservedEdge(existing, incoming topology.Edge) topology.Edge {
	merged := cloneEdge(existing)
	if incoming.FirstSeen.Before(merged.FirstSeen) {
		merged.FirstSeen = incoming.FirstSeen
	}
	if !incoming.LastSeen.Before(merged.LastSeen) {
		merged.LastSeen = incoming.LastSeen
		merged.Confidence = incoming.Confidence
		merged.Metadata = cloneStringMap(incoming.Metadata)
	}
	seen := make(map[string]struct{}, len(existing.Provenance)+len(incoming.Provenance))
	merged.Provenance = make([]string, 0, len(existing.Provenance)+len(incoming.Provenance))
	for _, source := range append(append([]string(nil), existing.Provenance...), incoming.Provenance...) {
		if _, exists := seen[source]; exists {
			continue
		}
		seen[source] = struct{}{}
		merged.Provenance = append(merged.Provenance, source)
	}
	sort.Strings(merged.Provenance)
	return merged
}

func cloneClusterProfile(snapshot profile.ClusterProfile) profile.ClusterProfile {
	cloned := snapshot
	cloned.Capabilities = append([]profile.SourceCapability(nil), snapshot.Capabilities...)
	for i := range cloned.Capabilities {
		evidence := &cloned.Capabilities[i].Evidence
		evidence.PrometheusTargets = append([]profile.PrometheusTarget(nil), evidence.PrometheusTargets...)
		cloned.Capabilities[i].Signals = append([]string(nil), cloned.Capabilities[i].Signals...)
		if evidence.OTLP != nil {
			otlp := *evidence.OTLP
			evidence.OTLP = &otlp
		}
		if evidence.Mesh != nil {
			mesh := *evidence.Mesh
			mesh.InjectedNamespaces = append([]string(nil), evidence.Mesh.InjectedNamespaces...)
			evidence.Mesh = &mesh
		}
	}
	return cloned
}
