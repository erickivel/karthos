package metrics

import (
	"math"
	"testing"
)

func TestPodPhaseLabel(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  string
	}{
		{name: "pending", value: 1, want: HealthPending},
		{name: "running", value: 2, want: PodPhaseRunning},
		{name: "succeeded", value: 3, want: HealthSucceeded},
		{name: "failed", value: 4, want: HealthFailed},
		{name: "unknown", value: 5, want: HealthUnknown},
		{name: "invalid", value: 99, want: HealthUnknown},
		{name: "not a number", value: math.NaN(), want: HealthUnknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := PodPhaseLabel(test.value); got != test.want {
				t.Fatalf("PodPhaseLabel(%v) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestHealthLabelRecognizesPodPhase(t *testing.T) {
	if !IsHealthStatusMetric("k8s.pod.phase") {
		t.Fatal("k8s.pod.phase must expose an API enum label")
	}
	if got := HealthLabel("k8s.pod.phase", 2); got != PodPhaseRunning {
		t.Fatalf("HealthLabel() = %q, want %q", got, PodPhaseRunning)
	}
}
