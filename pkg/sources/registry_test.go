package sources

import (
	"testing"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/karthos/pkg/collector/spec"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/core/semconv"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/detection"
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
	capabilityProfile := completeCapabilityProfile()
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

func TestRegistryRejectsInvalidModuleEvidenceBeforeSourceResolution(t *testing.T) {
	capabilityProfile := completeCapabilityProfile()
	prometheus, ok := capabilityProfile.Capability(IDPrometheusApp)
	require.True(t, ok)
	prometheus.Evidence.PrometheusTargets = nil
	for index := range capabilityProfile.Capabilities {
		if capabilityProfile.Capabilities[index].SourceID == IDPrometheusApp {
			capabilityProfile.Capabilities[index] = prometheus
		}
	}

	_, err := DefaultRegistry().ResolveSourcePlan(capabilityProfile)
	require.ErrorContains(t, err, "prometheus.app available capability requires at least one target")
}

func TestRegistryRejectsSourcePlanBoundToAnotherProfile(t *testing.T) {
	capabilityProfile := completeCapabilityProfile()
	registry := DefaultRegistry()
	sourcePlan, err := registry.ResolveSourcePlan(capabilityProfile)
	require.NoError(t, err)

	capabilityProfile.ClusterName = "other-cluster"
	plan := spec.CollectionPlan{}
	err = registry.AppendRequirements(&plan, capabilityProfile, sourcePlan)
	require.ErrorContains(t, err, "source plan profile fingerprint does not match capability profile")
}

func TestRegistryDelegatesCallsEdgeClassificationToSourceModule(t *testing.T) {
	registry := DefaultRegistry()
	metadata, ok := registry.CallsEdgeMetadata(IDMeshIstio, "istio_requests_total", "servicegraph.request.count", nil)
	require.True(t, ok)
	require.Empty(t, metadata)

	_, ok = registry.CallsEdgeMetadata(IDKubelet, "k8s.pod.cpu.usage", "k8s.pod.cpu.usage", nil)
	require.False(t, ok)
}

func TestRegistryParsesStampedSourceMetricsBeforeGenericIngest(t *testing.T) {
	registry := DefaultRegistry()

	parsed, ok := registry.ParseMetric(IDHostOS, "system.disk.io", map[string]string{
		semconv.AttrDirection: "write",
	})
	require.True(t, ok)
	require.Equal(t, "system.disk.io.write", parsed.CanonicalName)
	require.Equal(t, 1.0, parsed.ConversionFactor)

	parsed, ok = registry.ParseMetric(IDKubeCluster, "k8s.node.condition", map[string]string{
		semconv.AttrCondition: "Ready",
	})
	require.True(t, ok)
	require.Equal(t, "k8s.node.condition.ready", parsed.CanonicalName)

	_, ok = registry.ParseMetric(IDKubeCluster, "k8s.node.condition", map[string]string{
		semconv.AttrCondition: "PIDPressure",
	})
	require.False(t, ok)
}

func TestRegistryParsesSupportedMetricsThroughEachStampedTelemetryModule(t *testing.T) {
	registry := DefaultRegistry()
	for _, tt := range []struct {
		source     string
		raw        string
		attributes map[string]string
		canonical  string
	}{
		{source: IDKubelet, raw: "container.cpu.usage", canonical: "container.cpu.usage"},
		{source: IDHostOS, raw: "system.network.io", attributes: map[string]string{semconv.AttrDirection: "receive"}, canonical: "system.network.io.receive"},
		{source: IDKubeCluster, raw: "k8s.container.cpu_request", canonical: "k8s.container.cpu.request"},
		{source: IDPrometheusApp, raw: "http_requests_total", canonical: "http.server.request.count"},
		{source: IDMeshIstio, raw: "istio_requests_total", canonical: "servicegraph.request.count"},
		{source: IDTracesServicegraph, raw: "traces_service_graph_request_total", canonical: "servicegraph.request.count"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			parsed, ok := registry.ParseMetric(tt.source, tt.raw, tt.attributes)
			require.True(t, ok)
			require.Equal(t, tt.canonical, parsed.CanonicalName)
		})
	}
}

func TestDefaultRegistryDetectsEveryCompiledModuleFromOneInventory(t *testing.T) {
	inventory := detection.Inventory{
		ClusterName:      "test",
		TargetNamespaces: []string{"demo"},
		Nodes:            []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}},
		Services: []corev1.Service{{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo", Annotations: map[string]string{
				"prometheus.io/scrape":   "true",
				"prometheus.io/port":     "8080",
				"karthos.io/otlp-traces": "true",
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

func completeCapabilityProfile() *profile.ClusterProfile {
	return &profile.ClusterProfile{ClusterName: "test", Capabilities: []profile.SourceCapability{
		{SourceID: IDKubernetesStructure, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"structure"}},
		{SourceID: IDKubelet, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: IDHostOS, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: IDKubeCluster, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: IDPrometheusApp, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}, Evidence: profile.CapabilityEvidence{PrometheusTargets: []profile.PrometheusTarget{{ServiceName: "gateway", Namespace: "demo", Port: 8080, Path: "/metrics"}}}},
		{SourceID: IDMeshIstio, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}, Evidence: profile.CapabilityEvidence{Mesh: &profile.MeshEvidence{Flavor: "istio", Mode: "sidecar", InjectedNamespaces: []string{"demo"}, MetricsPort: 15020, MetricsPath: "/stats/prometheus"}}},
		{SourceID: IDTracesServicegraph, Kind: profile.SourceKindDerived, Status: profile.CapabilityAvailable, Signals: []string{"metric"}, Evidence: profile.CapabilityEvidence{OTLP: &profile.OTLPEvidence{TracesEnabled: true}}},
	}}
}
