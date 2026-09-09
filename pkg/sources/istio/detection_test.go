package istio

import (
	"testing"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/detection"
)

func TestDetectRecognizesNativeSidecarMesh(t *testing.T) {
	capability := Detect(meshInventory(corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "demo", Name: "api", OwnerReferences: []metav1.OwnerReference{{Controller: boolPointer(true)}}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "api"}}, InitContainers: []corev1.Container{{Name: "istio-proxy", RestartPolicy: containerRestartPolicy(corev1.ContainerRestartPolicyAlways)}}}}))
	require.Equal(t, profile.CapabilityAvailable, capability.Status)
	require.Equal(t, []string{"metric"}, capability.Signals)
	require.Equal(t, []string{"demo"}, capability.Evidence.Mesh.InjectedNamespaces)
}

func TestDetectReportsPartialWhenAWorkloadLacksProxy(t *testing.T) {
	capability := Detect(meshInventory(
		corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "demo", Name: "with-proxy", OwnerReferences: []metav1.OwnerReference{{Controller: boolPointer(true)}}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "api"}, {Name: "istio-proxy"}}}},
		corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "demo", Name: "without-proxy", OwnerReferences: []metav1.OwnerReference{{Controller: boolPointer(true)}}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "api"}}}},
	))
	require.Equal(t, profile.CapabilityPartial, capability.Status)
	require.Empty(t, capability.Signals)
}

func TestDetectReportsUnavailableWithoutMeshPrerequisites(t *testing.T) {
	capability := Detect(detection.Inventory{})
	require.Equal(t, profile.CapabilityUnavailable, capability.Status)
}

func meshInventory(pods ...corev1.Pod) detection.Inventory {
	return detection.Inventory{
		Namespaces:                    []corev1.Namespace{{ObjectMeta: metav1.ObjectMeta{Name: "istio-system"}}, {ObjectMeta: metav1.ObjectMeta{Name: "demo", Labels: map[string]string{"istio-injection": "enabled"}}}},
		Pods:                          pods,
		MutatingWebhookConfigurations: []admissionv1.MutatingWebhookConfiguration{{ObjectMeta: metav1.ObjectMeta{Name: "istio-sidecar-injector"}}},
	}
}

func boolPointer(value bool) *bool { return &value }
func containerRestartPolicy(value corev1.ContainerRestartPolicy) *corev1.ContainerRestartPolicy {
	return &value
}
