package kubernetes

import (
	"context"
	"testing"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/entities"
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

func TestDiscoverIgnoresIstioControlPlaneWithoutInjectedNamespaces(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "istio-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "oksg-demo"}},
		&admissionv1.MutatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "istio-sidecar-injector"}},
	)

	adapter := NewDiscoveryAdapter(client, "kind-test", nil)
	profile, err := adapter.Discover(context.Background())
	require.NoError(t, err)

	for _, source := range profile.Sources {
		require.NotEqual(t, "istio", source.Type)
	}
	require.NotContains(t, profile.RecommendedReceivers, "prometheus")
	for _, path := range profile.CollectionPaths {
		require.NotEqual(t, "prometheus/mesh-istio", path.ReceiverType)
	}
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
				Selector:  map[string]string{"app": "gateway-go"},
				Ports: []corev1.ServicePort{
					{Port: 8080, TargetPort: intstr.FromInt32(9090)},
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
	target := profile.Sources[0].PrometheusTargets[0]
	require.True(t, target.Reachable)
	require.Equal(t, int32(8080), target.Port, "prometheus.io/port identifies the Service port, not targetPort")
}

func TestDiscoverRejectsPrometheusTargetWithoutDeclaredServicePort(t *testing.T) {
	tests := []struct {
		name string
		port string
	}{
		{name: "missing annotation"},
		{name: "malformed annotation", port: "metrics"},
		{name: "undeclared numeric port", port: "9090"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotations := map[string]string{"prometheus.io/scrape": "true"}
			if tt.port != "" {
				annotations["prometheus.io/port"] = tt.port
			}
			client := fake.NewClientset(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "api", Namespace: "oksg-demo", Annotations: annotations,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.0.20",
					Ports:     []corev1.ServicePort{{Port: 8080, TargetPort: intstr.FromInt32(9090)}},
				},
			})

			adapter := NewDiscoveryAdapter(client, "kind-test", func(context.Context, string) bool {
				t.Fatal("invalid target must be rejected before reachability probing")
				return false
			})
			prof, err := adapter.Discover(context.Background())
			require.NoError(t, err)
			require.Len(t, prof.Sources, 1)
			require.False(t, prof.Sources[0].Present)
			require.Empty(t, prof.Sources[0].PrometheusTargets)
		})
	}
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

func TestDiscoverExtractsWorkloadControllerHPAsAndResourceQuotas(t *testing.T) {
	client := fake.NewClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "oksg-demo"},
			Spec: appsv1.StatefulSetSpec{
				Replicas: int32Ptr(2),
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "db"}},
				},
			},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "oksg-system"},
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "agent"}},
				},
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: "api-abc123", Namespace: "oksg-demo"},
			Spec: appsv1.ReplicaSetSpec{
				Replicas: int32Ptr(3),
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
				},
			},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "import-once", Namespace: "batch"},
			Spec: batchv1.JobSpec{
				Completions: int32Ptr(1),
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"job": "import-once"}},
				},
			},
		},
		&batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "batch"},
			Spec: batchv1.CronJobSpec{
				JobTemplate: batchv1.JobTemplateSpec{
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"cron": "nightly"}},
						},
					},
				},
			},
		},
		&corev1.ReplicationController{
			ObjectMeta: metav1.ObjectMeta{Name: "legacy", Namespace: "default"},
			Spec: corev1.ReplicationControllerSpec{
				Replicas: int32Ptr(2),
				Template: &corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "legacy"}},
				},
			},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "oksg-demo"},
		},
		&corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "compute", Namespace: "oksg-demo"},
		},
	)

	adapter := NewDiscoveryAdapter(client, "kind-test", nil)
	prof, err := adapter.Discover(context.Background())
	require.NoError(t, err)

	workloadsByID := make(map[entities.EntityID]entities.Workload)
	for _, workload := range prof.Workloads {
		workloadsByID[workload.ID()] = workload
	}

	expectedWorkloads := map[entities.EntityID]entities.WorkloadType{
		entities.ComputeEntityID(entities.KindWorkload, "kind-test", "oksg-demo", "statefulset", "db"):              entities.WorkloadStatefulSet,
		entities.ComputeEntityID(entities.KindWorkload, "kind-test", "oksg-system", "daemonset", "agent"):           entities.WorkloadDaemonSet,
		entities.ComputeEntityID(entities.KindWorkload, "kind-test", "oksg-demo", "replicaset", "api-abc123"):       entities.WorkloadReplicaSet,
		entities.ComputeEntityID(entities.KindWorkload, "kind-test", "batch", "job", "import-once"):                 entities.WorkloadJob,
		entities.ComputeEntityID(entities.KindWorkload, "kind-test", "batch", "cronjob", "nightly"):                 entities.WorkloadCronJob,
		entities.ComputeEntityID(entities.KindWorkload, "kind-test", "default", "replication_controller", "legacy"): entities.WorkloadReplicationController,
	}
	require.Len(t, prof.Workloads, len(expectedWorkloads))
	for id, workloadType := range expectedWorkloads {
		workload, ok := workloadsByID[id]
		require.True(t, ok, "missing workload %s", id)
		require.Equal(t, workloadType, workload.Type)
		require.Equal(t, string(workloadType), workload.Labels()[entities.WorkloadTypeLabel])
	}

	require.Len(t, prof.HPAs, 1)
	require.Equal(t, entities.ComputeEntityID(entities.KindHPA, "kind-test", "oksg-demo", "api"), prof.HPAs[0].ID())
	require.Len(t, prof.ResourceQuotas, 1)
	require.Equal(t, entities.ComputeEntityID(entities.KindResourceQuota, "kind-test", "oksg-demo", "compute"), prof.ResourceQuotas[0].ID())
}

func TestDiscoverSkipsTerminatingPods(t *testing.T) {
	now := metav1.Now()
	client := fake.NewClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "service-b-node", Namespace: "oksg-demo"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "service-b-node"}},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "service-b-node-old",
				Namespace:         "oksg-demo",
				Labels:            map[string]string{"app": "service-b-node"},
				DeletionTimestamp: &now,
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-b-node-current",
				Namespace: "oksg-demo",
				Labels:    map[string]string{"app": "service-b-node"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	adapter := NewDiscoveryAdapter(client, "kind-test", nil)
	profile, err := adapter.Discover(context.Background())
	require.NoError(t, err)
	require.Len(t, profile.Pods, 1)
	require.Equal(t, "service-b-node-current", profile.Pods[0].Name)
	require.NotEmpty(t, profile.Pods[0].WorkloadID)
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
