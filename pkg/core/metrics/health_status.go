package metrics

import "math"

// Health-status labels are the public, plain-text representation of
// integer-encoded health and pod-reason metrics. They are the
// only form that ever leaves the API: storage and PromQL composition keep
// the integer encoding (see metric_store.buildPromQL), and the API boundary
// translates to one of these strings via PodHealthLabel / ContainerHealthLabel.
//
// New consumers (frontends, dashboards, exporters) MUST read the string label
// and never the integer.
const (
	HealthHealthy                           = "healthy"
	HealthRunningNotReady                   = "running_not_ready"
	HealthRestarting                        = "restarting"
	HealthPending                           = "pending"
	HealthSucceeded                         = "succeeded"
	HealthFailed                            = "failed"
	HealthUnknown                           = "unknown"
	PodStatusReasonEvicted                  = "evicted"
	PodStatusReasonNodeAffinity             = "node_affinity"
	PodStatusReasonNodeLost                 = "node_lost"
	PodStatusReasonShutdown                 = "shutdown"
	PodStatusReasonUnexpectedAdmissionError = "unexpected_admission_error"
)

// PodHealthLabel translates the integer-encoded oksg.pod.health.status value
// to its public string label. Values outside the documented set return
// HealthUnknown — the catalog row's Description is the contract.
//
// The pod-scope composition currently emits only {1, 4, 5, 6, 7}; running_not_ready
// and restarting are observable today at container scope only and are tracked
// as a follow-up that requires a pod_id label on container metrics.
func PodHealthLabel(value float64) string {
	switch roundEnum(value) {
	case 1:
		return HealthHealthy
	case 2:
		return HealthRunningNotReady
	case 3:
		return HealthRestarting
	case 4:
		return HealthPending
	case 5:
		return HealthSucceeded
	case 6:
		return HealthFailed
	case 7:
		return HealthUnknown
	default:
		return HealthUnknown
	}
}

// ContainerHealthLabel translates the integer-encoded
// oksg.container.health.status value to its public string label. The composition
// emits {1, 2, 3} on a live container; values outside that set fall through
// to HealthUnknown.
func ContainerHealthLabel(value float64) string {
	switch roundEnum(value) {
	case 1:
		return HealthHealthy
	case 2:
		return HealthRunningNotReady
	case 3:
		return HealthRestarting
	default:
		return HealthUnknown
	}
}

// PodStatusReasonLabel translates the k8s_cluster receiver's integer pod
// status-reason encoding into the public API representation.
func PodStatusReasonLabel(value float64) string {
	switch roundEnum(value) {
	case 1:
		return PodStatusReasonEvicted
	case 2:
		return PodStatusReasonNodeAffinity
	case 3:
		return PodStatusReasonNodeLost
	case 4:
		return PodStatusReasonShutdown
	case 5:
		return PodStatusReasonUnexpectedAdmissionError
	default:
		return HealthUnknown
	}
}

// IsHealthStatusMetric reports whether the metric name is one of the enum
// metrics that carry a string ValueLabel on the wire.
func IsHealthStatusMetric(name string) bool {
	return name == "oksg.pod.health.status" ||
		name == "oksg.container.health.status" ||
		name == "k8s.pod.status_reason"
}

// HealthLabel dispatches to PodHealthLabel or ContainerHealthLabel based on the
// metric name. Used by the API/store boundary to populate MetricSample.ValueLabel
// without callers having to know which enum table applies.
func HealthLabel(metricName string, value float64) string {
	switch metricName {
	case "oksg.pod.health.status":
		return PodHealthLabel(value)
	case "oksg.container.health.status":
		return ContainerHealthLabel(value)
	case "k8s.pod.status_reason":
		return PodStatusReasonLabel(value)
	default:
		return ""
	}
}

// roundEnum rounds a float sample value to its nearest non-negative integer,
// guarding against tiny floating-point drift from PromQL arithmetic. NaN and
// negative values fall through to a sentinel that no enum entry maps to, so
// the caller's `default` arm produces HealthUnknown.
func roundEnum(value float64) int {
	if math.IsNaN(value) || value < 0 {
		return -1
	}
	return int(math.Round(value))
}
