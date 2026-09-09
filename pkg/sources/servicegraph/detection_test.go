package servicegraph

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/detection"
)

func TestDetectUsesDeterministicAnnotatedTraceProducer(t *testing.T) {
	capability := Detect(detection.Inventory{Services: []corev1.Service{
		{ObjectMeta: metav1.ObjectMeta{Name: "zeta", Namespace: "demo", Annotations: map[string]string{"karthos.io/otlp-traces": "true"}}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 4317}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "demo"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 4318}}}},
	}})

	require.Equal(t, profile.CapabilityAvailable, capability.Status)
	require.NotNil(t, capability.Evidence.OTLP)
	require.True(t, capability.Evidence.OTLP.TracesEnabled)
}

func TestDetectReportsUnavailableWithoutTraceProducer(t *testing.T) {
	capability := Detect(detection.Inventory{Services: []corev1.Service{{ObjectMeta: metav1.ObjectMeta{Name: "api"}}}})
	require.Equal(t, profile.CapabilityUnavailable, capability.Status)
}
