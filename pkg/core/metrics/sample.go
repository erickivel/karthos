package metrics

import (
	"time"

	"github.com/erickivel/oksg/pkg/core/entities"
)

// MetricSample is the wire and storage form of a single metric datapoint.
//
// Value carries the numeric form: a regular gauge/counter value for most
// metrics, or an integer-encoded enum (rounded float) for the
// `oksg.{pod,container}.health.status` metrics. ValueLabel is the public
// plain-text translation of an integer-enum value and is set ONLY by the
// API/store boundary for metrics where IsHealthStatusMetric reports true.
// Frontends should render ValueLabel for those metrics; the integer encoding
// never leaves the API for consumers.
type MetricSample struct {
	MetricName string            `json:"metric_name"`
	EntityID   entities.EntityID `json:"entity_id"`
	Value      float64           `json:"value"`
	ValueLabel string            `json:"value_label,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
	Labels     map[string]string `json:"labels,omitempty"`
	Unit       Unit              `json:"unit,omitempty"`
}
