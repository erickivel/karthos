package metrics

import "math"

// Health-status labels are the public representation used by derived graph and
// status views. Canonical metric samples remain numeric at storage boundaries.
const (
	HealthHealthy                           = "healthy"
	HealthRunningNotReady                   = "running_not_ready"
	HealthRestarting                        = "restarting"
	HealthPending                           = "pending"
	HealthSucceeded                         = "succeeded"
	HealthFailed                            = "failed"
	HealthUnknown                           = "unknown"
	PodPhaseRunning                         = "running"
	PodStatusReasonEvicted                  = "evicted"
	PodStatusReasonNodeAffinity             = "node_affinity"
	PodStatusReasonNodeLost                 = "node_lost"
	PodStatusReasonShutdown                 = "shutdown"
	PodStatusReasonUnexpectedAdmissionError = "unexpected_admission_error"
)

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

// PodPhaseLabel translates the k8s_cluster receiver's integer pod-phase
// encoding into the public API representation. It deliberately remains
// separate from derived pod health: a running Pod phase contributes to the
// health resolver's healthy bucket, while this metric reports the Kubernetes
// phase exactly as observed.
func PodPhaseLabel(value float64) string {
	switch roundEnum(value) {
	case 1:
		return HealthPending
	case 2:
		return PodPhaseRunning
	case 3:
		return HealthSucceeded
	case 4:
		return HealthFailed
	case 5:
		return HealthUnknown
	default:
		return HealthUnknown
	}
}

// IsHealthStatusMetric reports whether a canonical metric has a public enum
// translation at the API boundary.
func IsHealthStatusMetric(name string) bool {
	return name == "k8s.pod.phase" || name == "k8s.pod.status_reason"
}

// HealthLabel dispatches to the metric's public enum translation. It is used by
// the API/store boundary to populate MetricSample.ValueLabel without callers
// having to know which receiver-specific enum table applies.
func HealthLabel(metricName string, value float64) string {
	switch metricName {
	case "k8s.pod.phase":
		return PodPhaseLabel(value)
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
