package kubernetes

import (
	"context"
	"testing"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/require"
)

func TestDiscoverDetectsIstioMeshSource(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "istio-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   "oksg-demo",
			Labels: map[string]string{"istio-injection": "enabled"},
		}},
		&admissionv1.MutatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "istio-sidecar-injector"}},
	)

	adapter := NewDiscoveryAdapter(client, "kind-test", nil)
	profile, err := adapter.Discover(context.Background())
	require.NoError(t, err)

	require.Len(t, profile.Sources, 2)
	mesh := profile.Sources[1]
	require.Equal(t, "istio", mesh.Name)
	require.Equal(t, "istio", mesh.Type)
	require.True(t, mesh.Present)
	require.NotNil(t, mesh.Mesh)
	require.Equal(t, "istio", mesh.Mesh.Flavor)
	require.Equal(t, "sidecar", mesh.Mesh.Mode)
	require.Equal(t, int32(15020), mesh.Mesh.MetricsPort)
	require.Equal(t, "/stats/prometheus", mesh.Mesh.MetricsPath)
	require.Equal(t, []string{"oksg-demo"}, mesh.Mesh.InjectedNamespaces)
	require.Contains(t, profile.RecommendedReceivers, "prometheus")
}

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

func TestDiscoverDetectsOTLPSourceFromAnnotation(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "order-service",
				Namespace: "oksg-demo",
				Annotations: map[string]string{
					"oksg.io/otlp-traces": "true",
				},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Port: 8080},
				},
			},
		},
	)

	adapter := NewDiscoveryAdapter(client, "kind-test", nil)
	profile, err := adapter.Discover(context.Background())
	require.NoError(t, err)

	// prometheus source + one otlp source
	require.Len(t, profile.Sources, 2)
	otlp := profile.Sources[1]
	require.Equal(t, "oksg-demo/order-service-otlp", otlp.Name)
	require.Equal(t, "otlp", otlp.Type)
	require.True(t, otlp.Present)
	require.NotNil(t, otlp.OTLP)
	require.True(t, otlp.OTLP.TracesEnabled)
	require.False(t, otlp.OTLP.MetricsEnabled)
	require.Equal(t, "order-service.oksg-demo.svc.cluster.local:4317", otlp.OTLP.GRPCEndpoint)
	require.Equal(t, "order-service.oksg-demo.svc.cluster.local:4318", otlp.OTLP.HTTPEndpoint)
}

func TestDiscoverDetectsOTLPSourceFromGRPCPort(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "trace-collector",
				Namespace: "oksg-demo",
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Port: 4317},
				},
			},
		},
	)

	adapter := NewDiscoveryAdapter(client, "kind-test", nil)
	profile, err := adapter.Discover(context.Background())
	require.NoError(t, err)

	require.Len(t, profile.Sources, 2)
	otlp := profile.Sources[1]
	require.Equal(t, "oksg-demo/trace-collector-otlp", otlp.Name)
	require.Equal(t, "otlp", otlp.Type)
	require.True(t, otlp.Present)
	require.NotNil(t, otlp.OTLP)
	require.True(t, otlp.OTLP.TracesEnabled)
}

func TestDiscoverDetectsOTLPSourceFromHTTPPort(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "trace-collector-http",
				Namespace: "oksg-demo",
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Port: 4318},
				},
			},
		},
	)

	adapter := NewDiscoveryAdapter(client, "kind-test", nil)
	profile, err := adapter.Discover(context.Background())
	require.NoError(t, err)

	require.Len(t, profile.Sources, 2)
	otlp := profile.Sources[1]
	require.Equal(t, "oksg-demo/trace-collector-http-otlp", otlp.Name)
	require.Equal(t, "otlp", otlp.Type)
	require.True(t, otlp.Present)
	require.NotNil(t, otlp.OTLP)
	require.True(t, otlp.OTLP.TracesEnabled)
}

func TestDiscoverNoOTLPSourceWhenNoAnnotationOrPort(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "plain-service",
				Namespace: "oksg-demo",
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Port: 8080},
				},
			},
		},
	)

	adapter := NewDiscoveryAdapter(client, "kind-test", nil)
	profile, err := adapter.Discover(context.Background())
	require.NoError(t, err)

	// only the prometheus source, no otlp source
	require.Len(t, profile.Sources, 1)
	require.Equal(t, "prometheus", profile.Sources[0].Type)
}

func TestDiscoverExtractsPodTemplateLabelsAndRefs(t *testing.T) {
	client := fake.NewClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway-go", Namespace: "oksg-demo", Labels: map[string]string{"team": "platform"}},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "gateway-go"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "gateway",
								EnvFrom: []corev1.EnvFromSource{
									{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "gateway-config"}}},
									{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "gateway-secret"}}},
								},
							},
						},
						Volumes: []corev1.Volume{
							{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data-pvc"}}},
							{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "gateway-config"}}}},
						},
					},
				},
			},
		},
	)

	adapter := NewDiscoveryAdapter(client, "kind-test", nil)
	profile, err := adapter.Discover(context.Background())
	require.NoError(t, err)
	require.Len(t, profile.Workloads, 1)

	w := profile.Workloads[0]
	require.Equal(t, map[string]string{"app": "gateway-go"}, w.PodTemplateLabels)
	require.Equal(t, []string{"gateway-config"}, w.ConfigMapRefs) // deduped
	require.Equal(t, []string{"gateway-secret"}, w.SecretRefs)
	require.Equal(t, []string{"data-pvc"}, w.PVCRefs)
}

func TestDiscoverExtractsIngresses(t *testing.T) {
	client := fake.NewClientset(
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway-ingress", Namespace: "oksg-demo"},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{
						Host: "gateway.local",
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										Path: "/",
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{
												Name: "gateway-go",
												Port: networkingv1.ServiceBackendPort{Number: 8080},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	)

	adapter := NewDiscoveryAdapter(client, "kind-test", nil)
	profile, err := adapter.Discover(context.Background())
	require.NoError(t, err)
	require.Len(t, profile.Ingresses, 1)

	ing := profile.Ingresses[0]
	require.Equal(t, "gateway-ingress", ing.Name)
	require.Len(t, ing.BackendServices, 1)
	require.Equal(t, "gateway-go", ing.BackendServices[0].ServiceName)
	require.Equal(t, int32(8080), ing.BackendServices[0].ServicePort)
}

func TestDiscoverExtractsEnvValueFromRefs(t *testing.T) {
	client := fake.NewClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-a", Namespace: "oksg-demo"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "main",
								Env: []corev1.EnvVar{
									{Name: "DB_PASS", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "db-creds"}}}},
									{Name: "APP_MODE", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "app-settings"}}}},
									{Name: "PLAIN", Value: "hello"},
								},
							},
						},
					},
				},
			},
		},
	)

	adapter := NewDiscoveryAdapter(client, "kind-test", nil)
	profile, err := adapter.Discover(context.Background())
	require.NoError(t, err)
	require.Len(t, profile.Workloads, 1)

	w := profile.Workloads[0]
	require.Equal(t, []string{"app-settings"}, w.ConfigMapRefs)
	require.Equal(t, []string{"db-creds"}, w.SecretRefs)
}

func int32Ptr(value int32) *int32 {
	return &value
}
