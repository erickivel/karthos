package sources

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/sources/contract"
)

func TestCompiledTelemetryModulesOwnNormalization(t *testing.T) {
	registry := DefaultRegistry()
	for _, sourceID := range registry.MetricIDs() {
		module, ok := registry.Lookup(sourceID)
		require.Truef(t, ok, "module %s", sourceID)
		require.NotNilf(t, module.Normalize, "module %s must own normalization", sourceID)
	}
}

func TestCompiledModulesNormalizeTheirCurrentSourceContracts(t *testing.T) {
	resolver := normalizationResolver{}
	tests := []struct {
		source string
		plan   contract.Plan
		check  func(*testing.T, contract.Plan)
	}{
		{IDHostOS, plan(IDHostOS, "system.disk.io.write", attrs(semconv.AttrDirection, "write")), func(t *testing.T, got contract.Plan) { require.Equal(t, "system.disk.io.write", got.MetricName) }},
		{IDKubeCluster, plan(IDKubeCluster, "k8s.node.condition.ready", attrs(semconv.AttrCondition, "Ready")), func(t *testing.T, got contract.Plan) { require.Equal(t, "k8s.node.condition.ready", got.MetricName) }},
		{IDKubelet, plan(IDKubelet, "k8s.volume.usage", attrs(semconv.AttrK8sPVCName, "data")), func(t *testing.T, got contract.Plan) { require.NotEmpty(t, got.EntityID) }},
		{IDPrometheusApp, plan(IDPrometheusApp, "http.server.request.count", attrs("service", "api", "namespace", "demo")), func(t *testing.T, got contract.Plan) {
			require.Equal(t, entities.EntityID("service:demo:api"), got.EntityID)
		}},
		{IDTracesServicegraph, plan(IDTracesServicegraph, "servicegraph.request.count", attrs(semconv.AttrClient, "client", semconv.AttrServer, "server", semconv.AttrClientNamespace, "demo", semconv.AttrServerNamespace, "demo")), func(t *testing.T, got contract.Plan) { require.Equal(t, "client", got.Labels[semconv.LabelEdgeClient]) }},
		{IDMeshIstio, istioPlan(), func(t *testing.T, got contract.Plan) {
			require.Len(t, got.Derived, 1)
			require.Equal(t, "servicegraph.request.error.count", got.Derived[0].MetricName)
		}},
	}
	registry := DefaultRegistry()
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			require.NoError(t, registry.Normalize(context.Background(), resolver, &tt.plan))
			tt.check(t, tt.plan)
		})
	}
}

func plan(source, metric string, values []*commonpb.KeyValue) contract.Plan {
	return contract.NewPlan(metric, metric, source, map[string]string{semconv.ResourceK8sNamespaceName: "demo"}, values, contract.ParsedMetric{CanonicalName: metric, ConversionFactor: 1})
}
func istioPlan() contract.Plan {
	result := plan(IDMeshIstio, "servicegraph.request.count", attrs(semconv.AttrClient, "client", semconv.AttrServer, "server", semconv.AttrClientNamespace, "demo", semconv.AttrServerNamespace, "demo", semconv.AttrHTTPStatusCode, "500"))
	result.RawMetricName = "istio_requests_total"
	return result
}
func attrs(values ...string) []*commonpb.KeyValue {
	out := make([]*commonpb.KeyValue, 0, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		out = append(out, &commonpb.KeyValue{Key: values[i], Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: values[i+1]}}})
	}
	return out
}

type normalizationResolver struct{}

func (normalizationResolver) Resolve(context.Context, map[string]string) (entities.EntityID, error) {
	return "default", nil
}
func (normalizationResolver) ClusterName() string { return "test" }
func (normalizationResolver) ResolveServiceMetricEntity(_ context.Context, _ map[string]string, service, namespace string) (entities.EntityID, bool) {
	return entities.EntityID("service:" + namespace + ":" + service), true
}
