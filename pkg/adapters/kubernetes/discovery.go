package kubernetes

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/erickivel/oksg/pkg/adapters"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

const (
	discoveryAdapterName       = "k8s-discovery-adapter"
	prometheusAnnotationScrape = "prometheus.io/scrape"
	prometheusAnnotationPath   = "prometheus.io/path"
	prometheusAnnotationPort   = "prometheus.io/port"
	defaultPrometheusPath      = "/metrics"
	otlpAnnotationTraces       = "oksg.io/otlp-traces"
	otlpGRPCPort               = int32(4317)
	otlpHTTPPort               = int32(4318)
	istioSystemNamespace       = "istio-system"
	istioSidecarWebhook        = "istio-sidecar-injector"
	istioMetricsPort           = int32(15020)
	istioMetricsPath           = "/stats/prometheus"
)

type HTTPProbe func(ctx context.Context, url string) bool

type DiscoveryAdapter struct {
	client           kubernetes.Interface
	clusterName      string
	probe            HTTPProbe
	now              func() time.Time
	targetNamespaces []string
}

var _ adapters.DiscoveryAdapter = (*DiscoveryAdapter)(nil)

type DiscoveryOption func(*DiscoveryAdapter)

func WithTargetNamespaces(namespaces ...string) DiscoveryOption {
	return func(d *DiscoveryAdapter) {
		d.targetNamespaces = namespaces
	}
}

func NewDiscoveryAdapter(client kubernetes.Interface, clusterName string, probe HTTPProbe, opts ...DiscoveryOption) *DiscoveryAdapter {
	if probe == nil {
		probe = func(context.Context, string) bool { return false }
	}

	d := &DiscoveryAdapter{
		client:      client,
		clusterName: clusterName,
		probe:       probe,
		now:         time.Now,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

func (d *DiscoveryAdapter) Name() string {
	return discoveryAdapterName
}

func (d *DiscoveryAdapter) Discover(ctx context.Context) (*profile.ClusterProfile, error) {
	nodes, err := d.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	namespaces, err := d.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	deployments, err := d.client.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	statefulSets, err := d.client.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list statefulsets: %w", err)
	}
	daemonSets, err := d.client.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list daemonsets: %w", err)
	}
	replicaSets, err := d.client.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list replicasets: %w", err)
	}
	jobs, err := d.client.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	cronJobs, err := d.client.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list cronjobs: %w", err)
	}
	replicationControllers, err := d.client.CoreV1().ReplicationControllers("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list replicationcontrollers: %w", err)
	}
	resourceQuotas, err := d.client.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list resourcequotas: %w", err)
	}
	hpas, err := d.client.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list hpas: %w", err)
	}

	pods, err := d.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	services, err := d.client.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	ingresses, err := d.client.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list ingresses: %w", err)
	}

	profileResult := &profile.ClusterProfile{
		ClusterID:    entities.NewCluster(d.clusterName, nil).ID(),
		ClusterName:  d.clusterName,
		DiscoveredAt: d.now().UTC(),
		Nodes:        make([]entities.Node, 0, len(nodes.Items)),
		Namespaces:   make([]entities.Namespace, 0, len(namespaces.Items)),
		Workloads: make([]entities.Workload, 0,
			len(deployments.Items)+len(statefulSets.Items)+len(daemonSets.Items)+
				len(replicaSets.Items)+len(jobs.Items)+len(cronJobs.Items)+len(replicationControllers.Items)),
		HPAs:           make([]entities.HPA, 0, len(hpas.Items)),
		ResourceQuotas: make([]entities.ResourceQuota, 0, len(resourceQuotas.Items)),
		Services:       make([]entities.Service, 0, len(services.Items)),
	}

	for _, node := range sortNodes(nodes.Items) {
		profileResult.Nodes = append(profileResult.Nodes, entities.NewNode(d.clusterName, node.Name, node.Labels))
	}

	for _, namespace := range sortNamespaces(namespaces.Items) {
		profileResult.Namespaces = append(profileResult.Namespaces, entities.NewNamespace(d.clusterName, namespace.Name, namespace.Labels))
	}

	for _, deployment := range sortDeployments(deployments.Items) {
		workload := entities.NewWorkload(d.clusterName, deployment.Namespace, deployment.Name, entities.WorkloadDeployment, deployment.Labels)
		if deployment.Spec.Replicas != nil {
			workload.Replicas = *deployment.Spec.Replicas
		}
		workload.PodTemplateLabels = cloneStringMap(deployment.Spec.Template.Labels)
		extractWorkloadRefs(&workload, deployment.Spec.Template.Spec)
		profileResult.Workloads = append(profileResult.Workloads, workload)
	}
	for _, statefulSet := range sortStatefulSets(statefulSets.Items) {
		workload := entities.NewWorkload(d.clusterName, statefulSet.Namespace, statefulSet.Name, entities.WorkloadStatefulSet, statefulSet.Labels)
		if statefulSet.Spec.Replicas != nil {
			workload.Replicas = *statefulSet.Spec.Replicas
		}
		workload.PodTemplateLabels = cloneStringMap(statefulSet.Spec.Template.Labels)
		extractWorkloadRefs(&workload, statefulSet.Spec.Template.Spec)
		profileResult.Workloads = append(profileResult.Workloads, workload)
	}
	for _, daemonSet := range sortDaemonSets(daemonSets.Items) {
		workload := entities.NewWorkload(d.clusterName, daemonSet.Namespace, daemonSet.Name, entities.WorkloadDaemonSet, daemonSet.Labels)
		workload.PodTemplateLabels = cloneStringMap(daemonSet.Spec.Template.Labels)
		extractWorkloadRefs(&workload, daemonSet.Spec.Template.Spec)
		profileResult.Workloads = append(profileResult.Workloads, workload)
	}
	for _, replicaSet := range sortReplicaSets(replicaSets.Items) {
		workload := entities.NewWorkload(d.clusterName, replicaSet.Namespace, replicaSet.Name, entities.WorkloadReplicaSet, replicaSet.Labels)
		if replicaSet.Spec.Replicas != nil {
			workload.Replicas = *replicaSet.Spec.Replicas
		}
		workload.PodTemplateLabels = cloneStringMap(replicaSet.Spec.Template.Labels)
		extractWorkloadRefs(&workload, replicaSet.Spec.Template.Spec)
		profileResult.Workloads = append(profileResult.Workloads, workload)
	}
	for _, job := range sortJobs(jobs.Items) {
		workload := entities.NewWorkload(d.clusterName, job.Namespace, job.Name, entities.WorkloadJob, job.Labels)
		if job.Spec.Completions != nil {
			workload.Replicas = *job.Spec.Completions
		}
		workload.PodTemplateLabels = cloneStringMap(job.Spec.Template.Labels)
		extractWorkloadRefs(&workload, job.Spec.Template.Spec)
		profileResult.Workloads = append(profileResult.Workloads, workload)
	}
	for _, cronJob := range sortCronJobs(cronJobs.Items) {
		workload := entities.NewWorkload(d.clusterName, cronJob.Namespace, cronJob.Name, entities.WorkloadCronJob, cronJob.Labels)
		workload.PodTemplateLabels = cloneStringMap(cronJob.Spec.JobTemplate.Spec.Template.Labels)
		extractWorkloadRefs(&workload, cronJob.Spec.JobTemplate.Spec.Template.Spec)
		profileResult.Workloads = append(profileResult.Workloads, workload)
	}
	for _, rc := range sortReplicationControllers(replicationControllers.Items) {
		workload := entities.NewWorkload(d.clusterName, rc.Namespace, rc.Name, entities.WorkloadReplicationController, rc.Labels)
		if rc.Spec.Replicas != nil {
			workload.Replicas = *rc.Spec.Replicas
		}
		if rc.Spec.Template != nil {
			workload.PodTemplateLabels = cloneStringMap(rc.Spec.Template.Labels)
			extractWorkloadRefs(&workload, rc.Spec.Template.Spec)
		}
		profileResult.Workloads = append(profileResult.Workloads, workload)
	}
	for _, hpa := range sortHPAs(hpas.Items) {
		profileResult.HPAs = append(profileResult.HPAs, entities.NewHPA(d.clusterName, hpa.Namespace, hpa.Name, hpa.Labels))
	}
	for _, quota := range sortResourceQuotas(resourceQuotas.Items) {
		profileResult.ResourceQuotas = append(profileResult.ResourceQuotas, entities.NewResourceQuota(d.clusterName, quota.Namespace, quota.Name, quota.Labels))
	}

	for _, pod := range sortPods(pods.Items) {
		if len(d.targetNamespaces) > 0 && !d.isTargetNamespace(pod.Namespace) {
			continue
		}
		if isTerminatingPod(pod) {
			continue
		}
		entityPod := entities.NewPod(d.clusterName, pod.Namespace, pod.Name, pod.Labels)
		entityPod.WorkloadID = matchPodWorkloadID(entityPod, profileResult.Workloads)
		profileResult.Pods = append(profileResult.Pods, entityPod)
	}

	for _, ing := range sortIngresses(ingresses.Items) {
		ingress := entities.NewIngress(d.clusterName, ing.Namespace, ing.Name, "", ing.Labels)
		ingress.BackendServices = extractIngressBackends(ing)
		profileResult.Ingresses = append(profileResult.Ingresses, ingress)
	}

	var prometheusTargets []profile.PrometheusTarget
	var otlpSources []profile.ObservabilitySource
	for _, service := range sortServices(services.Items) {
		profileResult.Services = append(
			profileResult.Services,
			entities.NewService(d.clusterName, service.Namespace, service.Name, service.Spec.Selector, service.Labels),
		)

		target, ok := d.prometheusTarget(ctx, service)
		if ok {
			prometheusTargets = append(prometheusTargets, target)
		}

		if src, ok := otlpSource(service); ok {
			otlpSources = append(otlpSources, src)
		}
	}

	profileResult.Sources = []profile.ObservabilitySource{
		{
			Name:              "application-prometheus",
			Type:              "prometheus",
			Present:           len(prometheusTargets) > 0,
			PrometheusTargets: prometheusTargets,
		},
	}
	profileResult.Sources = append(profileResult.Sources, otlpSources...)
	if meshSource, ok := d.istioSource(ctx, namespaces.Items); ok {
		profileResult.Sources = append(profileResult.Sources, meshSource)
	}

	profileResult.CollectionPaths = []profile.OTelCollectionPath{
		{ReceiverType: "kubeletstats", Enabled: true},
	}
	profileResult.RecommendedReceivers = []string{"kubeletstats"}

	if len(prometheusTargets) > 0 {
		targetIDs := make([]string, 0, len(prometheusTargets))
		for _, target := range prometheusTargets {
			targetIDs = append(targetIDs, fmt.Sprintf("%s/%s:%d%s", target.Namespace, target.ServiceName, target.Port, target.Path))
		}

		profileResult.CollectionPaths = append(profileResult.CollectionPaths, profile.OTelCollectionPath{
			ReceiverType: "prometheus",
			Enabled:      true,
			Targets:      targetIDs,
		})
		profileResult.RecommendedReceivers = append(profileResult.RecommendedReceivers, "prometheus")
	}
	if hasIstioSource(profileResult.Sources) {
		profileResult.CollectionPaths = append(profileResult.CollectionPaths, profile.OTelCollectionPath{
			ReceiverType: "prometheus/mesh-istio",
			Enabled:      true,
			Targets:      []string{fmt.Sprintf("pod/istio-proxy:%d%s", istioMetricsPort, istioMetricsPath)},
		})
		if !containsString(profileResult.RecommendedReceivers, "prometheus") {
			profileResult.RecommendedReceivers = append(profileResult.RecommendedReceivers, "prometheus")
		}
	}

	return profileResult, nil
}

func (d *DiscoveryAdapter) istioSource(ctx context.Context, namespaces []corev1.Namespace) (profile.ObservabilitySource, bool) {
	hasSystemNamespace := false
	var injected []string
	for _, namespace := range namespaces {
		if namespace.Name == istioSystemNamespace {
			hasSystemNamespace = true
		}
		if namespace.Labels["istio-injection"] == "enabled" {
			injected = append(injected, namespace.Name)
		}
	}
	if !hasSystemNamespace {
		return profile.ObservabilitySource{}, false
	}

	webhooks, err := d.client.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return profile.ObservabilitySource{}, false
	}
	if !hasMutatingWebhook(webhooks.Items, istioSidecarWebhook) {
		return profile.ObservabilitySource{}, false
	}
	sort.Strings(injected)

	return profile.ObservabilitySource{
		Name:    "istio",
		Type:    "istio",
		Present: true,
		Mesh: &profile.MeshSource{
			Flavor:             "istio",
			Mode:               "sidecar",
			InjectedNamespaces: injected,
			MetricsPort:        istioMetricsPort,
			MetricsPath:        istioMetricsPath,
		},
	}, true
}

func hasMutatingWebhook(items []admissionv1.MutatingWebhookConfiguration, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func hasIstioSource(sources []profile.ObservabilitySource) bool {
	for _, source := range sources {
		if source.Type == "istio" && source.Present && source.Mesh != nil && source.Mesh.Flavor == "istio" {
			return true
		}
	}
	return false
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func (d *DiscoveryAdapter) prometheusTarget(ctx context.Context, service corev1.Service) (profile.PrometheusTarget, bool) {
	if !strings.EqualFold(service.Annotations[prometheusAnnotationScrape], "true") {
		return profile.PrometheusTarget{}, false
	}

	if len(d.targetNamespaces) > 0 && !d.isTargetNamespace(service.Namespace) {
		return profile.PrometheusTarget{}, false
	}

	port := firstServicePort(service)
	if rawPort := service.Annotations[prometheusAnnotationPort]; rawPort != "" {
		if parsed, err := strconv.Atoi(rawPort); err == nil && parsed >= 1 && parsed <= 65535 {
			port = int32(parsed) //#nosec G115 G109 -- range validated: 1-65535 fits safely in int32
		}
	}

	path := service.Annotations[prometheusAnnotationPath]
	if path == "" {
		path = defaultPrometheusPath
	}

	target := profile.PrometheusTarget{
		ServiceName: service.Name,
		Namespace:   service.Namespace,
		Port:        port,
		Path:        path,
	}

	if service.Spec.ClusterIP != "" && service.Spec.ClusterIP != corev1.ClusterIPNone {
		target.Reachable = d.probe(ctx, fmt.Sprintf("http://%s:%d%s", service.Spec.ClusterIP, port, path))
	}

	return target, true
}

func otlpSource(service corev1.Service) (profile.ObservabilitySource, bool) {
	hasAnnotation := strings.EqualFold(service.Annotations[otlpAnnotationTraces], "true")

	hasOTLPPort := false
	for _, p := range service.Spec.Ports {
		if p.Port == otlpGRPCPort || p.Port == otlpHTTPPort {
			hasOTLPPort = true
			break
		}
	}

	if !hasAnnotation && !hasOTLPPort {
		return profile.ObservabilitySource{}, false
	}

	return profile.ObservabilitySource{
		Name:    fmt.Sprintf("%s/%s-otlp", service.Namespace, service.Name),
		Type:    "otlp",
		Present: true,
		OTLP: &profile.OTLPSource{
			MetricsEnabled: false,
			TracesEnabled:  true,
			GRPCEndpoint:   fmt.Sprintf("%s.%s.svc.cluster.local:4317", service.Name, service.Namespace),
			HTTPEndpoint:   fmt.Sprintf("%s.%s.svc.cluster.local:4318", service.Name, service.Namespace),
		},
	}, true
}

func firstServicePort(service corev1.Service) int32 {
	if len(service.Spec.Ports) == 0 {
		return 80
	}
	return service.Spec.Ports[0].Port
}

func sortNodes(items []corev1.Node) []corev1.Node {
	sorted := append([]corev1.Node(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	return sorted
}

func sortNamespaces(items []corev1.Namespace) []corev1.Namespace {
	sorted := append([]corev1.Namespace(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	return sorted
}

func sortDeployments(items []appsv1.Deployment) []appsv1.Deployment {
	sorted := append([]appsv1.Deployment(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace == sorted[j].Namespace {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Namespace < sorted[j].Namespace
	})
	return sorted
}

func sortStatefulSets(items []appsv1.StatefulSet) []appsv1.StatefulSet {
	sorted := append([]appsv1.StatefulSet(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace == sorted[j].Namespace {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Namespace < sorted[j].Namespace
	})
	return sorted
}

func sortDaemonSets(items []appsv1.DaemonSet) []appsv1.DaemonSet {
	sorted := append([]appsv1.DaemonSet(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace == sorted[j].Namespace {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Namespace < sorted[j].Namespace
	})
	return sorted
}

func sortReplicaSets(items []appsv1.ReplicaSet) []appsv1.ReplicaSet {
	sorted := append([]appsv1.ReplicaSet(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace == sorted[j].Namespace {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Namespace < sorted[j].Namespace
	})
	return sorted
}

func sortJobs(items []batchv1.Job) []batchv1.Job {
	sorted := append([]batchv1.Job(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace == sorted[j].Namespace {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Namespace < sorted[j].Namespace
	})
	return sorted
}

func sortCronJobs(items []batchv1.CronJob) []batchv1.CronJob {
	sorted := append([]batchv1.CronJob(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace == sorted[j].Namespace {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Namespace < sorted[j].Namespace
	})
	return sorted
}

func sortReplicationControllers(items []corev1.ReplicationController) []corev1.ReplicationController {
	sorted := append([]corev1.ReplicationController(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace == sorted[j].Namespace {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Namespace < sorted[j].Namespace
	})
	return sorted
}

func sortResourceQuotas(items []corev1.ResourceQuota) []corev1.ResourceQuota {
	sorted := append([]corev1.ResourceQuota(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace == sorted[j].Namespace {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Namespace < sorted[j].Namespace
	})
	return sorted
}

func sortHPAs(items []autoscalingv2.HorizontalPodAutoscaler) []autoscalingv2.HorizontalPodAutoscaler {
	sorted := append([]autoscalingv2.HorizontalPodAutoscaler(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace == sorted[j].Namespace {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Namespace < sorted[j].Namespace
	})
	return sorted
}

func sortServices(items []corev1.Service) []corev1.Service {
	sorted := append([]corev1.Service(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace == sorted[j].Namespace {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Namespace < sorted[j].Namespace
	})
	return sorted
}

func sortPods(items []corev1.Pod) []corev1.Pod {
	sorted := append([]corev1.Pod(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace == sorted[j].Namespace {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Namespace < sorted[j].Namespace
	})
	return sorted
}

func isTerminatingPod(pod corev1.Pod) bool {
	return pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}

func matchPodWorkloadID(pod entities.Pod, workloads []entities.Workload) entities.EntityID {
	for _, workload := range workloads {
		if pod.Namespace != workload.Namespace {
			continue
		}
		if selectorMatches(workload.PodTemplateLabels, pod.Labels()) {
			return workload.ID()
		}
	}
	return ""
}

func selectorMatches(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

func (d *DiscoveryAdapter) isTargetNamespace(ns string) bool {
	for _, target := range d.targetNamespaces {
		if target == ns {
			return true
		}
	}
	return false
}

func extractWorkloadRefs(w *entities.Workload, podSpec corev1.PodSpec) {
	cmSet := make(map[string]struct{})
	secretSet := make(map[string]struct{})
	pvcSet := make(map[string]struct{})

	for _, vol := range podSpec.Volumes {
		if vol.ConfigMap != nil {
			cmSet[vol.ConfigMap.Name] = struct{}{}
		}
		if vol.Secret != nil {
			secretSet[vol.Secret.SecretName] = struct{}{}
		}
		if vol.PersistentVolumeClaim != nil {
			pvcSet[vol.PersistentVolumeClaim.ClaimName] = struct{}{}
		}
	}

	for _, c := range podSpec.Containers {
		for _, ef := range c.EnvFrom {
			if ef.ConfigMapRef != nil {
				cmSet[ef.ConfigMapRef.Name] = struct{}{}
			}
			if ef.SecretRef != nil {
				secretSet[ef.SecretRef.Name] = struct{}{}
			}
		}
		for _, e := range c.Env {
			if e.ValueFrom == nil {
				continue
			}
			if e.ValueFrom.ConfigMapKeyRef != nil {
				cmSet[e.ValueFrom.ConfigMapKeyRef.Name] = struct{}{}
			}
			if e.ValueFrom.SecretKeyRef != nil {
				secretSet[e.ValueFrom.SecretKeyRef.Name] = struct{}{}
			}
		}
	}

	w.ConfigMapRefs = sortedKeys(cmSet)
	w.SecretRefs = sortedKeys(secretSet)
	w.PVCRefs = sortedKeys(pvcSet)
}

func extractIngressBackends(ing networkingv1.Ingress) []entities.IngressBackend {
	var backends []entities.IngressBackend
	seen := make(map[string]struct{})

	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
		svc := ing.Spec.DefaultBackend.Service
		key := fmt.Sprintf("%s:%d", svc.Name, svc.Port.Number)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			backends = append(backends, entities.IngressBackend{
				ServiceName: svc.Name,
				ServicePort: svc.Port.Number,
			})
		}
	}

	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil {
				continue
			}
			svc := path.Backend.Service
			key := fmt.Sprintf("%s:%d", svc.Name, svc.Port.Number)
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				backends = append(backends, entities.IngressBackend{
					ServiceName: svc.Name,
					ServicePort: svc.Port.Number,
				})
			}
		}
	}

	return backends
}

func sortIngresses(items []networkingv1.Ingress) []networkingv1.Ingress {
	sorted := append([]networkingv1.Ingress(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace == sorted[j].Namespace {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Namespace < sorted[j].Namespace
	})
	return sorted
}

func sortedKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cloneStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	clone := make(map[string]string, len(m))
	for k, v := range m {
		clone[k] = v
	}
	return clone
}
