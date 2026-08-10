package kubernetes

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/erickivel/oksg/pkg/adapters"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

const (
	discoveryAdapterName         = "k8s-capability-discovery-adapter"
	prometheusAnnotationScrape   = "prometheus.io/scrape"
	prometheusAnnotationPath     = "prometheus.io/path"
	prometheusAnnotationPort     = "prometheus.io/port"
	defaultPrometheusPath        = "/metrics"
	otlpAnnotationTraces         = "oksg.io/otlp-traces"
	otlpGRPCPort                 = int32(4317)
	otlpHTTPPort                 = int32(4318)
	istioSystemNamespace         = "istio-system"
	istioSidecarWebhook          = "istio-sidecar-injector"
	istioMetricsPort             = int32(15020)
	istioMetricsPath             = "/stats/prometheus"
	oksgSystemNamespace          = "oksg-system"
	istioProxyContainer          = "istio-proxy"
	istioSidecarInjectAnnotation = "sidecar.istio.io/inject"
)

type HTTPProbe func(ctx context.Context, url string) bool

// DiscoveryAdapter produces a capability-only profile. Structural inventory
// intentionally belongs to kubernetes.structure at backend runtime.
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
	return func(d *DiscoveryAdapter) { d.targetNamespaces = append([]string(nil), namespaces...) }
}

func NewDiscoveryAdapter(client kubernetes.Interface, clusterName string, probe HTTPProbe, opts ...DiscoveryOption) *DiscoveryAdapter {
	if probe == nil {
		probe = func(context.Context, string) bool { return false }
	}
	adapter := &DiscoveryAdapter{client: client, clusterName: clusterName, probe: probe, now: time.Now}
	for _, option := range opts {
		option(adapter)
	}
	return adapter
}

func (d *DiscoveryAdapter) Name() string { return discoveryAdapterName }

// Discover runs each supported source probe directly. A failed Kubernetes API
// operation is an error, never an implicit "unavailable" capability.
func (d *DiscoveryAdapter) Discover(ctx context.Context) (*profile.ClusterProfile, error) {
	if d.client == nil {
		return nil, fmt.Errorf("Kubernetes client is required")
	}
	if _, err := d.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{}); err != nil {
		return nil, fmt.Errorf("detect kubernetes.structure: list nodes: %w", err)
	}
	services, err := d.client.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("detect prometheus.app/traces.servicegraph: list services: %w", err)
	}
	namespaces, err := d.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("detect mesh.istio: list namespaces: %w", err)
	}
	pods, err := d.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("detect mesh.istio: list pods: %w", err)
	}
	webhooks, err := d.client.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("detect mesh.istio: list mutating webhook configurations: %w", err)
	}

	targets := make([]profile.PrometheusTarget, 0)
	otlp := make([]profile.OTLPEvidence, 0)
	for _, service := range services.Items {
		if target, ok := d.prometheusTarget(ctx, service); ok {
			targets = append(targets, target)
		}
		if evidence, ok := otlpEvidence(service); ok {
			otlp = append(otlp, evidence)
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Namespace != targets[j].Namespace {
			return targets[i].Namespace < targets[j].Namespace
		}
		return targets[i].ServiceName < targets[j].ServiceName
	})

	capabilities := []profile.SourceCapability{
		availableCapability(metrics.SourceKubernetesStructure, profile.SourceKindDirect, []string{"structure"}),
		availableCapability(metrics.SourceKubelet, profile.SourceKindDirect, []string{"metric"}),
		availableCapability(metrics.SourceHostOS, profile.SourceKindDirect, []string{"metric"}),
		availableCapability(metrics.SourceKubeCluster, profile.SourceKindDirect, []string{"metric"}),
		{SourceID: metrics.SourcePrometheusApp, Kind: profile.SourceKindDirect, Status: statusFor(len(targets) > 0), Signals: signalsFor(len(targets) > 0, "metric"), Evidence: profile.CapabilityEvidence{PrometheusTargets: targets}},
		d.istioCapability(namespaces.Items, pods.Items, webhooks.Items),
		servicegraphCapability(otlp),
	}
	result := &profile.ClusterProfile{ClusterID: entities.NewCluster(d.clusterName, nil).ID(), ClusterName: d.clusterName, DiscoveredAt: d.now().UTC(), Capabilities: capabilities}
	result.Sort()
	return result, nil
}

func availableCapability(id string, kind profile.SourceKind, signals []string) profile.SourceCapability {
	return profile.SourceCapability{SourceID: id, Kind: kind, Status: profile.CapabilityAvailable, Signals: signals}
}
func statusFor(available bool) profile.CapabilityStatus {
	if available {
		return profile.CapabilityAvailable
	}
	return profile.CapabilityUnavailable
}
func signalsFor(available bool, signal string) []string {
	if available {
		return []string{signal}
	}
	return nil
}

func (d *DiscoveryAdapter) istioCapability(namespaces []corev1.Namespace, pods []corev1.Pod, webhooks []admissionv1.MutatingWebhookConfiguration) profile.SourceCapability {
	injected := make([]string, 0)
	hasSystemNamespace := false
	for _, namespace := range namespaces {
		if namespace.Name == istioSystemNamespace {
			hasSystemNamespace = true
		}
		if namespace.Labels["istio-injection"] == "enabled" {
			injected = append(injected, namespace.Name)
		}
	}
	if !hasSystemNamespace || !hasWebhook(webhooks, istioSidecarWebhook) || len(injected) == 0 {
		return profile.SourceCapability{SourceID: metrics.SourceMeshIstio, Kind: profile.SourceKindDirect, Status: profile.CapabilityUnavailable}
	}
	sort.Strings(injected)
	status := profile.CapabilityAvailable
	if partialMesh(pods, injected) {
		status = profile.CapabilityPartial
	}
	return profile.SourceCapability{SourceID: metrics.SourceMeshIstio, Kind: profile.SourceKindDirect, Status: status, Signals: signalsFor(status == profile.CapabilityAvailable, "metric"), Evidence: profile.CapabilityEvidence{Mesh: &profile.MeshEvidence{Flavor: "istio", Mode: "sidecar", InjectedNamespaces: injected, MetricsPort: istioMetricsPort, MetricsPath: istioMetricsPath}}}
}

func servicegraphCapability(evidence []profile.OTLPEvidence) profile.SourceCapability {
	capability := profile.SourceCapability{SourceID: metrics.SourceTracesServicegraph, Kind: profile.SourceKindDerived, Status: profile.CapabilityUnavailable}
	if len(evidence) == 0 {
		return capability
	}
	// The derived connector needs traces; its bounded evidence records one
	// deterministic trace producer rather than an unbounded service inventory.
	for _, source := range evidence {
		if source.TracesEnabled {
			capability.Status = profile.CapabilityAvailable
			capability.Signals = []string{"metric"}
			capability.Evidence.OTLP = &source
			return capability
		}
	}
	return capability
}

func (d *DiscoveryAdapter) prometheusTarget(ctx context.Context, service corev1.Service) (profile.PrometheusTarget, bool) {
	if !strings.EqualFold(service.Annotations[prometheusAnnotationScrape], "true") || !d.targetNamespace(service.Namespace) {
		return profile.PrometheusTarget{}, false
	}
	port, err := strconv.Atoi(service.Annotations[prometheusAnnotationPort])
	if err != nil || port < 1 || port > 65535 || !declaresPort(service, int32(port)) {
		return profile.PrometheusTarget{}, false
	}
	path := service.Annotations[prometheusAnnotationPath]
	if path == "" {
		path = defaultPrometheusPath
	}
	target := profile.PrometheusTarget{ServiceName: service.Name, Namespace: service.Namespace, Port: int32(port), Path: path}
	if service.Spec.ClusterIP != "" && service.Spec.ClusterIP != corev1.ClusterIPNone {
		target.Reachable = d.probe(ctx, "http://"+service.Spec.ClusterIP+":"+strconv.Itoa(port)+path)
	}
	return target, true
}

func otlpEvidence(service corev1.Service) (profile.OTLPEvidence, bool) {
	annotated := strings.EqualFold(service.Annotations[otlpAnnotationTraces], "true")
	grpc, http := false, false
	for _, port := range service.Spec.Ports {
		grpc = grpc || port.Port == otlpGRPCPort
		http = http || port.Port == otlpHTTPPort
	}
	if !annotated && !grpc && !http {
		return profile.OTLPEvidence{}, false
	}
	endpoint := service.Name + "." + service.Namespace + ".svc.cluster.local"
	return profile.OTLPEvidence{TracesEnabled: true, GRPCEndpoint: endpoint + ":4317", HTTPEndpoint: endpoint + ":4318"}, true
}

func hasWebhook(items []admissionv1.MutatingWebhookConfiguration, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
func partialMesh(pods []corev1.Pod, injectedNamespaces []string) bool {
	injected := map[string]struct{}{}
	for _, namespace := range injectedNamespaces {
		injected[namespace] = struct{}{}
	}
	for _, pod := range pods {
		if terminatingPod(pod) || systemNamespace(pod.Namespace) || !controllerOwned(pod) {
			continue
		}
		if _, ok := injected[pod.Namespace]; !ok || strings.EqualFold(strings.TrimSpace(pod.Annotations[istioSidecarInjectAnnotation]), "false") || !hasProxy(pod) {
			return true
		}
	}
	return false
}
func terminatingPod(pod corev1.Pod) bool {
	return pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}
func systemNamespace(namespace string) bool {
	return namespace == istioSystemNamespace || namespace == oksgSystemNamespace || strings.HasPrefix(namespace, "kube-")
}
func controllerOwned(pod corev1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Controller != nil && *owner.Controller {
			return true
		}
	}
	return false
}
func hasProxy(pod corev1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		if container.Name == istioProxyContainer {
			return true
		}
	}
	for _, container := range pod.Spec.InitContainers {
		if container.Name == istioProxyContainer && container.RestartPolicy != nil && *container.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			return true
		}
	}
	return false
}
func declaresPort(service corev1.Service, port int32) bool {
	for _, declared := range service.Spec.Ports {
		if declared.Port == port {
			return true
		}
	}
	return false
}
func (d *DiscoveryAdapter) targetNamespace(namespace string) bool {
	if len(d.targetNamespaces) == 0 {
		return true
	}
	for _, target := range d.targetNamespaces {
		if target == namespace {
			return true
		}
	}
	return false
}
