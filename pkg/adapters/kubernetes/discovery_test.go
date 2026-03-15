package kubernetes

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/require"
)

func TestDiscoverBuildsClusterProfileWithPrometheusTargets(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "kind-control-plane"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "oksg-demo"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway-go", Namespace: "oksg-demo"},
			Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(1)},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-go",
				Namespace: "oksg-demo",
				Annotations: map[string]string{
					"prometheus.io/scrape": "true",
					"prometheus.io/path":   "/metrics",
					"prometheus.io/port":   "8080",
				},
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.96.0.10",
				Ports: []corev1.ServicePort{
					{Port: 8080},
				},
			},
		},
	)

	adapter := NewDiscoveryAdapter(client, "kind-test", func(_ context.Context, url string) bool {
		return url == "http://10.96.0.10:8080/metrics"
	})

	profile, err := adapter.Discover(context.Background())
	require.NoError(t, err)
	require.Equal(t, "kind-test", profile.ClusterName)
	require.Len(t, profile.Nodes, 1)
	require.Len(t, profile.Namespaces, 1)
	require.Len(t, profile.Workloads, 1)
	require.Len(t, profile.Services, 1)
	require.Equal(t, []string{"kubeletstats", "prometheus"}, profile.RecommendedReceivers)
	require.Len(t, profile.CollectionPaths, 2)
	require.Len(t, profile.Sources, 1)
	require.True(t, profile.Sources[0].Present)
	require.Len(t, profile.Sources[0].PrometheusTargets, 1)
	require.True(t, profile.Sources[0].PrometheusTargets[0].Reachable)
}

func TestDiscoverFallsBackToKubeletstatsWhenPrometheusAbsent(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "oksg-demo"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "service-a-python", Namespace: "oksg-demo"},
			Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(1)},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "service-a-python", Namespace: "oksg-demo"},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.96.0.11",
				Ports: []corev1.ServicePort{
					{Port: 8081},
				},
			},
		},
	)

	adapter := NewDiscoveryAdapter(client, "kind-test", func(_ context.Context, _ string) bool {
		return false
	})

	profile, err := adapter.Discover(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"kubeletstats"}, profile.RecommendedReceivers)
	require.Len(t, profile.CollectionPaths, 1)
	require.Len(t, profile.Sources, 1)
	require.Equal(t, "prometheus", profile.Sources[0].Type)
	require.False(t, profile.Sources[0].Present)
	require.Empty(t, profile.Sources[0].PrometheusTargets)
}

func int32Ptr(value int32) *int32 {
	return &value
}
