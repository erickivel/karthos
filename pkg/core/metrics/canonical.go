package metrics

import "github.com/erickivel/oksg/pkg/core/entities"

type MetricCategory string
type MetricScope string
type MetricInstrument string

const (
	CategoryResource MetricCategory = "resource"
	CategoryTraffic  MetricCategory = "traffic"
	CategoryLatency  MetricCategory = "latency"
	CategoryError    MetricCategory = "error"
	CategoryCapacity MetricCategory = "capacity"
	CategoryHealth   MetricCategory = "health"

	ScopeEntity MetricScope = "entity"
	ScopeEdge   MetricScope = "edge"

	InstrumentGauge     MetricInstrument = "gauge"
	InstrumentCounter   MetricInstrument = "counter"
	InstrumentHistogram MetricInstrument = "histogram"
)

type CanonicalMetric struct {
	Name        string                `json:"name"`
	DisplayName string                `json:"display_name"`
	Unit        Unit                  `json:"unit"`
	Category    MetricCategory        `json:"category"`
	Scopes      []MetricScope         `json:"scopes,omitempty"`
	AppliesTo   []entities.EntityKind `json:"applies_to,omitempty"`
	Dimensions  []string              `json:"dimensions,omitempty"`
	Description string                `json:"description"`
	Sources     []SourceRef           `json:"sources,omitempty"`
	Aggregation *AggregationSpec      `json:"aggregation,omitempty"`
	Instrument  MetricInstrument      `json:"-"`
	StoredUnit  Unit                  `json:"-"`
}

// SourceRef binds a canonical metric to a logical source that can produce it.
// Smaller priority values win source arbitration.
type SourceRef struct {
	ID       string `json:"id"`
	Priority int    `json:"priority"`
}

// Logical source IDs. Each ID is the contract between the catalog (which metrics
// declare a producer), the source registry (which detects availability and owns
// source parsing), and the planner (which tags wire output with the
// matching oksg.source attribute).
const (
	// SourceKubernetesStructure identifies the non-metric Kubernetes API module.
	// It is included in capability snapshots but never in metric arbitration.
	SourceKubernetesStructure = "kubernetes.structure"
	SourceKubelet             = "kubelet"
	SourceHostOS              = "host-os"
	SourceKubeCluster         = "kube-cluster"
	SourcePrometheusApp       = "prometheus.app"
	SourceMeshIstio           = "mesh.istio"
	SourceTracesServicegraph  = "traces.servicegraph"
)
