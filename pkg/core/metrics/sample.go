package metrics

import (
	"time"

	"github.com/erickivel/oksg/pkg/core/entities"
)

type MetricSample struct {
	MetricName string            `json:"metric_name"`
	EntityID   entities.EntityID `json:"entity_id"`
	Value      float64           `json:"value"`
	Timestamp  time.Time         `json:"timestamp"`
	Labels     map[string]string `json:"labels,omitempty"`
}
