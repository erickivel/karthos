package metrics

import (
	"time"

	"github.com/erickivel/karthos/pkg/core/entities"
)

// MetricSample is the wire and storage form of a single metric datapoint.
//
// Value carries the numeric form: a regular gauge/counter value for most
// metrics, or an integer-encoded enum (rounded float) for the
// enum metrics such as `karthos.{pod,container}.health.status` and
// `k8s.pod.phase` and `k8s.pod.status_reason`. ValueLabel is the public
// plain-text translation of an integer-enum value and is set ONLY by the
// API/store boundary for metrics where IsHealthStatusMetric reports true.
// Frontends should render ValueLabel for those metrics; the integer encoding
// never leaves the API for consumers.
// Derivation identifies an explicit query interpretation, never a stored label.
type Derivation struct {
	ID             string `json:"id"`
	Version        string `json:"version"`
	InputMetric    string `json:"input_metric"`
	Interpretation string `json:"interpretation"`
}
type MetricSample struct {
	Derivation *Derivation       `json:"derivation,omitempty"`
	MetricName string            `json:"metric_name"`
	EntityID   entities.EntityID `json:"entity_id"`
	Value      float64           `json:"value"`
	ValueLabel string            `json:"value_label,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
	Labels     map[string]string `json:"labels,omitempty"`
	Unit       Unit              `json:"unit,omitempty"`
}
