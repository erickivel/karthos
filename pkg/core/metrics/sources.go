package metrics

import (
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

// LogicalSource declares a producer of canonical metrics. Detect inspects the
// ClusterProfile to decide whether the source is available; Dialect translates
// raw on-the-wire metric names emitted by the source into canonical names.
//
// Sources whose Dialect is nil emit canonical-shaped names directly.
type LogicalSource struct {
	ID          string
	DisplayName string
	Description string
	Detect      func(*profile.ClusterProfile) bool
	Dialect     Dialect
}

var defaultLogicalSources = []LogicalSource{
	{
		ID:          SourceKubelet,
		DisplayName: "Kubelet stats",
		Description: "Per-node kubelet /stats/summary scraped by the OTel kubeletstats receiver.",
		Detect:      alwaysAvailable,
		Dialect:     KubeletDialect,
	},
	{
		ID:          SourceHostOS,
		DisplayName: "Host OS metrics",
		Description: "Node-level CPU, memory, disk, filesystem, network and load metrics scraped by the OTel hostmetrics receiver.",
		Detect:      alwaysAvailable,
	},
	{
		ID:          SourceKubeCluster,
		DisplayName: "Kubernetes API",
		Description: "Cluster-level workload, capacity and condition metrics produced by the OTel k8s_cluster receiver.",
		Detect:      alwaysAvailable,
	},
	{
		ID:          SourcePrometheusApp,
		DisplayName: "Application Prometheus endpoints",
		Description: "Application metrics scraped from declared Prometheus targets.",
		Detect:      hasPrometheusTargets,
		Dialect:     PrometheusDialect,
	},
	{
		ID:          SourceMeshIstio,
		DisplayName: "Istio service mesh",
		Description: "Istio sidecar telemetry scraped from istio-proxy /stats/prometheus.",
		Detect:      hasIstioMesh,
		Dialect:     IstioDialect,
	},
	{
		ID:          SourceTracesSpanmetrics,
		DisplayName: "Trace-derived span metrics",
		Description: "Span-derived RPC request rate and duration produced by the OTel spanmetrics connector.",
		Detect:      hasTraces,
		Dialect:     SpanmetricsDialect,
	},
	{
		ID:          SourceTracesServicegraph,
		DisplayName: "Trace-derived service graph",
		Description: "Edge metrics produced by the OTel servicegraph connector from incoming traces.",
		Detect:      hasTraces,
		Dialect:     ServicegraphDialect,
	},
}

// DefaultLogicalSources returns a copy of the built-in source registry.
func DefaultLogicalSources() []LogicalSource {
	out := make([]LogicalSource, len(defaultLogicalSources))
	copy(out, defaultLogicalSources)
	return out
}

func alwaysAvailable(*profile.ClusterProfile) bool { return true }

func hasPrometheusTargets(p *profile.ClusterProfile) bool {
	if p == nil {
		return false
	}
	for _, source := range p.Sources {
		if source.Type == "prometheus" && source.Present && len(source.PrometheusTargets) > 0 {
			return true
		}
	}
	return false
}

func hasIstioMesh(p *profile.ClusterProfile) bool {
	if p == nil {
		return false
	}
	for _, source := range p.Sources {
		if source.Type == "istio" && source.Present && source.Mesh != nil {
			return true
		}
	}
	return false
}

func hasTraces(p *profile.ClusterProfile) bool {
	if p == nil {
		return false
	}
	for _, source := range p.Sources {
		if source.Type != "otlp" || !source.Present || source.OTLP == nil {
			continue
		}
		if source.OTLP.TracesEnabled {
			return true
		}
	}
	return false
}
