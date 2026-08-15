package sources

import (
	"testing"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/collector/spec"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources/detection"
)

func TestDefaultRegistryContainsEveryCompiledModuleExactlyOnce(t *testing.T) {
	registry := DefaultRegistry()
	require.Equal(t, []string{
		IDKubernetesStructure,
		IDKubelet,
		IDHostOS,
		IDKubeCluster,
		IDPrometheusApp,
		IDMeshIstio,
		IDTracesServicegraph,
	}, registry.IDs())
	structure, ok := registry.Lookup(IDKubernetesStructure)
	require.True(t, ok)
	require.False(t, structure.MetricSource)
	prometheus, ok := registry.Lookup(IDPrometheusApp)
	require.True(t, ok)
	require.True(t, prometheus.MetricSource)
}

func TestRegistryAppendsTypedRequirementsFromSelectedModules(t *testing.T) {
	capabilityProfile := &profile.ClusterProfile{Capabilities: []profile.SourceCapability{
		{SourceID: IDKubernetesStructure, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"structure"}},
		{SourceID: IDKubelet, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: IDHostOS, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: IDKubeCluster, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: IDPrometheusApp, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}, Evidence: profile.CapabilityEvidence{PrometheusTargets: []profile.PrometheusTarget{{ServiceName: "gateway", Namespace: "demo", Port: 8080, Path: "/metrics"}}}},
		{SourceID: IDMeshIstio, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}, Evidence: profile.CapabilityEvidence{Mesh: &profile.MeshEvidence{Flavor: "istio", InjectedNamespaces: []string{"demo"}}}},
		{SourceID: IDTracesServicegraph, Kind: profile.SourceKindDerived, Status: profile.CapabilityAvailable, Signals: []string{"metric"}, Evidence: profile.CapabilityEvidence{OTLP: &profile.OTLPEvidence{TracesEnabled: true}}},
	}}
	registry := DefaultRegistry()
	sourcePlan, err := registry.ResolveSourcePlan(capabilityProfile)
	require.NoError(t, err)

	plan := spec.CollectionPlan{}
	require.NoError(t, registry.AppendRequirements(&plan, capabilityProfile, sourcePlan))
	require.NotNil(t, plan.Agent.Kubelet)
	require.NotNil(t, plan.Agent.HostOS)
	require.NotNil(t, plan.Agent.Istio)
	require.NotNil(t, plan.Gateway.KubeCluster)
	require.NotNil(t, plan.Gateway.Prometheus)
	require.True(t, plan.Agent.OTLP.Traces)
	require.Nil(t, plan.Gateway.ServiceGraph)
	require.Equal(t, metrics.SourceMeshIstio, sourcePlan.MetricAssignments["servicegraph.request.count"])
}

func TestDefaultRegistryDetectsEveryCompiledModuleFromOneInventory(t *testing.T) {
	inventory := detection.Inventory{
		ClusterName:      "test",
		TargetNamespaces: []string{"demo"},
		Nodes:            []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}},
		Services: []corev1.Service{{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo", Annotations: map[string]string{
				"prometheus.io/scrape": "true",
				"prometheus.io/port":   "8080",
				"oksg.io/otlp-traces":  "true",
			}},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}, {Port: 4317}}},
		}},
		Namespaces: []corev1.Namespace{
			{ObjectMeta: metav1.ObjectMeta{Name: "istio-system"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "demo", Labels: map[string]string{"istio-injection": "enabled"}}},
		},
		Pods: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{Namespace: "demo", Name: "api-0", OwnerReferences: []metav1.OwnerReference{{Controller: pointer(true)}}},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "api"}, {Name: "istio-proxy"}}},
		}},
		MutatingWebhookConfigurations: []admissionv1.MutatingWebhookConfiguration{{ObjectMeta: metav1.ObjectMeta{Name: "istio-sidecar-injector"}}},
	}

	capabilities, err := DefaultRegistry().Detect(inventory)
	require.NoError(t, err)
	require.Len(t, capabilities, 7)
	require.Equal(t, []string{
		IDHostOS,
		IDKubeCluster,
		IDKubelet,
		IDKubernetesStructure,
		IDMeshIstio,
		IDPrometheusApp,
		IDTracesServicegraph,
	}, sourceIDs(capabilities))
	for _, capability := range capabilities {
		require.Equal(t, profile.CapabilityAvailable, capability.Status)
	}
}

func TestRegistryRejectsDetectorThatClaimsAnotherModuleIdentity(t *testing.T) {
	registry := Registry{modules: []Module{{
		ID:   "expected",
		Kind: profile.SourceKindDirect,
		Detect: func(detection.Inventory) profile.SourceCapability {
			return profile.SourceCapability{SourceID: "other", Kind: profile.SourceKindDirect}
		},
	}}}

	_, err := registry.Detect(detection.Inventory{})
	require.ErrorContains(t, err, `compiled source module "expected" returned capability for "other"`)
}

func TestRegistryRejectsDetectorThatChangesItsSourceKind(t *testing.T) {
	registry := Registry{modules: []Module{{
		ID:   "expected",
		Kind: profile.SourceKindDirect,
		Detect: func(detection.Inventory) profile.SourceCapability {
			return profile.SourceCapability{SourceID: "expected", Kind: profile.SourceKindDerived}
		},
	}}}

	_, err := registry.Detect(detection.Inventory{})
	require.ErrorContains(t, err, `compiled source module "expected" returned kind "derived", expected "direct"`)
}

func pointer(value bool) *bool { return &value }

func sourceIDs(capabilities []profile.SourceCapability) []string {
	ids := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		ids = append(ids, capability.SourceID)
	}
	return ids
}
