package istio

import (
	"sort"
	"strings"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/detection"
)

const (
	istioSystemNamespace         = "istio-system"
	istioSidecarWebhook          = "istio-sidecar-injector"
	istioMetricsPort             = int32(15020)
	istioMetricsPath             = "/stats/prometheus"
	karthosSystemNamespace       = "karthos-system"
	istioProxyContainer          = "istio-proxy"
	istioSidecarInjectAnnotation = "sidecar.istio.io/inject"
)

// Detect determines whether the static Istio module has complete usable mesh
// coverage, partial coverage, or no supported mesh prerequisites.
func Detect(inventory detection.Inventory) profile.SourceCapability {
	injected := make([]string, 0)
	hasSystemNamespace := false
	for _, namespace := range inventory.Namespaces {
		if namespace.Name == istioSystemNamespace {
			hasSystemNamespace = true
		}
		if namespace.Labels["istio-injection"] == "enabled" {
			injected = append(injected, namespace.Name)
		}
	}
	capability := profile.SourceCapability{SourceID: metrics.SourceMeshIstio, Kind: profile.SourceKindDirect, Status: profile.CapabilityUnavailable}
	if !hasSystemNamespace || !hasWebhook(inventory.MutatingWebhookConfigurations, istioSidecarWebhook) || len(injected) == 0 {
		return capability
	}
	sort.Strings(injected)
	capability.Status = profile.CapabilityAvailable
	if partialMesh(inventory.Pods, injected) {
		capability.Status = profile.CapabilityPartial
	} else {
		capability.Signals = []string{"metric"}
	}
	capability.Evidence.Mesh = &profile.MeshEvidence{Flavor: "istio", Mode: "sidecar", InjectedNamespaces: injected, MetricsPort: istioMetricsPort, MetricsPath: istioMetricsPath}
	return capability
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
	return namespace == istioSystemNamespace || namespace == karthosSystemNamespace || strings.HasPrefix(namespace, "kube-")
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
