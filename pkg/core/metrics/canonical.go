package metrics

import "github.com/erickivel/oksg/pkg/core/entities"

type MetricCategory string
type MetricScope string

const (
	CategoryResource MetricCategory = "resource"
	CategoryTraffic  MetricCategory = "traffic"
	CategoryLatency  MetricCategory = "latency"
	CategoryError    MetricCategory = "error"
	CategoryCapacity MetricCategory = "capacity"
	CategoryHealth   MetricCategory = "health"

	ScopeEntity MetricScope = "entity"
	ScopeEdge   MetricScope = "edge"
)

type CanonicalMetric struct {
	Name            string                `json:"name"`
	DisplayName     string                `json:"display_name"`
	Unit            Unit                  `json:"unit"`
	Category        MetricCategory        `json:"category"`
	Scopes          []MetricScope         `json:"scopes,omitempty"`
	AppliesTo       []entities.EntityKind `json:"applies_to,omitempty"`
	Dimensions      []string              `json:"dimensions,omitempty"`
	Description     string                `json:"description"`
	Sources         []SourceRef           `json:"sources,omitempty"`
	SelectionPolicy SelectionPolicy       `json:"selection_policy,omitempty"`
	Aggregation     *AggregationSpec      `json:"aggregation,omitempty"`
}

// SourceRef binds a canonical metric to a logical source that can produce it.
// Priority is interpreted by SelectionPolicy: smaller wins for PolicyPriority.
type SourceRef struct {
	ID       string `json:"id"`
	Priority int    `json:"priority"`
}

// Logical source IDs. Each ID is the contract between the catalog (which metrics
// declare a producer), the source registry (which knows how to detect availability
// and which dialect to apply), and the planner (which tags wire output with the
// matching oksg.source attribute).
const (
	SourceKubelet            = "kubelet"
	SourceHostOS             = "host-os"
	SourceKubeCluster        = "kube-cluster"
	SourcePrometheusApp      = "prometheus.app"
	SourceMeshIstio          = "mesh.istio"
	SourceTracesSpanmetrics  = "traces.spanmetrics"
	SourceTracesServicegraph = "traces.servicegraph"
)

// SelectionPolicy decides how Sources is consumed when more than one source is
// available in a given cluster. Only PolicyPriority is implemented today; other
// values are reserved so future work doesn't require a catalog migration.
type SelectionPolicy string

const (
	// PolicyPriority picks the highest-priority (lowest Priority value) source
	// whose Detect returns true. Default when SelectionPolicy is empty.
	PolicyPriority SelectionPolicy = "priority"
	// PolicyDisjoint reserved for per-scope routing (e.g. istio for mesh-injected
	// namespaces, traces.servicegraph elsewhere). Resolver returns an error if used.
	PolicyDisjoint SelectionPolicy = "disjoint"
	// PolicyAll reserved for multi-source aggregation. Resolver returns an error if used.
	PolicyAll SelectionPolicy = "all"
)
