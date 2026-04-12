package metrics

type MetricCategory string

const (
	CategoryResource MetricCategory = "resource"
	CategoryTraffic  MetricCategory = "traffic"
	CategoryLatency  MetricCategory = "latency"
	CategoryError    MetricCategory = "error"
	CategoryCapacity MetricCategory = "capacity"
	CategoryHealth   MetricCategory = "health"
)

type CanonicalMetric struct {
	Name        string         `json:"name"`
	Unit        Unit           `json:"unit"`
	Category    MetricCategory `json:"category"`
	Dimensions  []string       `json:"dimensions,omitempty"`
	Description string         `json:"description"`
}
