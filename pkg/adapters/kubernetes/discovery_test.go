package kubernetes

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
)

func TestDiscoverProducesDeterministicCapabilityProfile(t *testing.T) {
	client := fake.NewClientset(&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo", Annotations: map[string]string{"prometheus.io/scrape": "true", "prometheus.io/port": "8080", "karthos.io/otlp-traces": "true"}}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}, {Port: 4317}}}})
	adapter := NewDiscoveryAdapter(client, "test")

	result, err := adapter.Discover(context.Background())
	require.NoError(t, err)
	require.Len(t, result.Capabilities, 7)
	structure, ok := result.Capability(metrics.SourceKubernetesStructure)
	require.True(t, ok)
	require.Equal(t, profile.CapabilityAvailable, structure.Status)
	prometheus, ok := result.Capability(metrics.SourcePrometheusApp)
	require.True(t, ok)
	require.Equal(t, profile.CapabilityAvailable, prometheus.Status)
	require.Equal(t, []profile.PrometheusTarget{{ServiceName: "api", Namespace: "demo", Port: 8080, Path: "/metrics"}}, prometheus.Evidence.PrometheusTargets)
	servicegraph, ok := result.Capability(metrics.SourceTracesServicegraph)
	require.True(t, ok)
	require.Equal(t, profile.SourceKindDerived, servicegraph.Kind)
	require.Equal(t, profile.CapabilityAvailable, servicegraph.Status)
}

func TestDiscoverExcludesInstallationNamespaceFromCapabilityEvidence(t *testing.T) {
	annotation := map[string]string{"prometheus.io/scrape": "true", "prometheus.io/port": "8080", "karthos.io/otlp-traces": "true"}
	client := fake.NewClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "backend", Namespace: "karthos-system", Annotations: annotation}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}, {Port: 4317}}}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "demo", Annotations: annotation}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}, {Port: 4317}}}},
	)
	result, err := NewDiscoveryAdapter(client, "test", WithExcludedNamespace("karthos-system")).Discover(context.Background())
	require.NoError(t, err)
	prometheus, ok := result.Capability(metrics.SourcePrometheusApp)
	require.True(t, ok)
	require.Equal(t, []profile.PrometheusTarget{{ServiceName: "app", Namespace: "demo", Port: 8080, Path: "/metrics"}}, prometheus.Evidence.PrometheusTargets)
}

func TestDiscoverFailsWhenARequiredKubernetesListIsDenied(t *testing.T) {
	tests := []struct {
		name     string
		resource string
	}{
		{name: "nodes", resource: "nodes"},
		{name: "services", resource: "services"},
		{name: "namespaces", resource: "namespaces"},
		{name: "pods", resource: "pods"},
		{name: "webhooks", resource: "mutatingwebhookconfigurations"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientset()
			client.PrependReactor("list", tt.resource, func(k8stesting.Action) (bool, runtime.Object, error) { return true, nil, errors.New("denied") })
			adapter := NewDiscoveryAdapter(client, "test")

			_, err := adapter.Discover(context.Background())
			require.ErrorContains(t, err, "list "+tt.resource+": denied")
		})
	}
}
