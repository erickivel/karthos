package prometheusapp

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/detection"
)

func TestDetectSelectsValidAnnotatedTargetsInConfiguredNamespaces(t *testing.T) {
	capability := Detect(detection.Inventory{
		TargetNamespaces: []string{"demo"},
		Services: []corev1.Service{
			{ObjectMeta: metav1.ObjectMeta{Name: "default-path", Namespace: "demo", Annotations: map[string]string{"prometheus.io/scrape": "true", "prometheus.io/port": "8080"}}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "custom-path", Namespace: "demo", Annotations: map[string]string{"prometheus.io/scrape": "true", "prometheus.io/port": "9090", "prometheus.io/path": "/custom"}}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 9090}}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "outside", Namespace: "other", Annotations: map[string]string{"prometheus.io/scrape": "true", "prometheus.io/port": "8080"}}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "invalid-port", Namespace: "demo", Annotations: map[string]string{"prometheus.io/scrape": "true", "prometheus.io/port": "8081"}}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}}},
		},
	})

	require.Equal(t, profile.CapabilityAvailable, capability.Status)
	require.Equal(t, []profile.PrometheusTarget{
		{ServiceName: "custom-path", Namespace: "demo", Port: 9090, Path: "/custom"},
		{ServiceName: "default-path", Namespace: "demo", Port: 8080, Path: "/metrics"},
	}, capability.Evidence.PrometheusTargets)
}

func TestDetectReportsUnavailableWithoutValidAnnotatedTargets(t *testing.T) {
	capability := Detect(detection.Inventory{Services: []corev1.Service{{ObjectMeta: metav1.ObjectMeta{Name: "bad", Annotations: map[string]string{"prometheus.io/scrape": "true", "prometheus.io/port": "not-a-port"}}}}})
	require.Equal(t, profile.CapabilityUnavailable, capability.Status)
	require.Empty(t, capability.Signals)
}
