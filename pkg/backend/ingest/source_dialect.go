package ingest

import (
	"context"
	"strconv"
	"strings"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
)

// sourceDialect contains the source-specific part of metric normalization.
// The shared normalizer remains responsible for OTLP traversal and sample emission.
type sourceDialect struct {
	sourceID string
	matches  func(rawMetricName, canonicalName string) bool
	apply    func(context.Context, EntityResolver, *normalizationPlan) error
}

type sourceDialectRegistry struct {
	bySource  map[string]sourceDialect
	fallbacks []sourceDialect
}

type normalizationPlan struct {
	rawMetricName    string
	metricName       string
	source           string
	resource         map[string]string
	attrs            []*commonpb.KeyValue
	labels           map[string]string
	entityID         entities.EntityID
	skip             bool
	conversionFactor float64
	unit             metrics.Unit
	derived          []derivedMetricPlan
}

type derivedMetricPlan struct {
	metricName       string
	conversionFactor float64
	unit             metrics.Unit
}

func newSourceDialectRegistry() sourceDialectRegistry {
	dialects := []sourceDialect{
		kubeletSourceDialect(),
		hostOSSourceDialect(),
		kubeClusterSourceDialect(),
		prometheusAppSourceDialect(),
		spanmetricsSourceDialect(),
		servicegraphSourceDialect(),
		istioSourceDialect(),
	}

	registry := sourceDialectRegistry{bySource: make(map[string]sourceDialect, len(dialects))}
	for _, dialect := range dialects {
		registry.bySource[dialect.sourceID] = dialect
		if dialect.matches != nil {
			registry.fallbacks = append(registry.fallbacks, dialect)
		}
	}
	return registry
}

func (r sourceDialectRegistry) apply(ctx context.Context, resolver EntityResolver, plan *normalizationPlan) error {
	dialect, ok := r.bySource[plan.source]
	if !ok {
		for _, candidate := range r.fallbacks {
			if candidate.matches(plan.rawMetricName, plan.metricName) {
				dialect = candidate
				ok = true
				break
			}
		}
	}

	if ok && dialect.apply != nil {
		if err := dialect.apply(ctx, resolver, plan); err != nil {
			return err
		}
	}
	if plan.skip || plan.entityID != "" {
		return nil
	}

	entityID, err := resolver.Resolve(ctx, plan.resource)
	if err != nil {
		return err
	}
	plan.entityID = entityID
	return nil
}

func newNormalizationPlan(
	rawMetricName string,
	canonicalName string,
	source string,
	resource map[string]string,
	attrs []*commonpb.KeyValue,
	mapping metrics.OTLPMetricMapping,
) normalizationPlan {
	return normalizationPlan{
		rawMetricName:    rawMetricName,
		metricName:       canonicalName,
		source:           source,
		resource:         resource,
		attrs:            attrs,
		labels:           map[string]string{semconv.LabelSource: source},
		conversionFactor: mapping.ConversionFactor,
		unit:             mapping.Unit,
	}
}

type labelAlias string

func preserveLabels(attrs []*commonpb.KeyValue, labels map[string]string, aliases ...labelAlias) {
	for _, alias := range aliases {
		sourceKey, targetKey := splitLabelAlias(string(alias))
		if value := kvLabelValue(attrs, sourceKey); value != "" {
			labels[targetKey] = value
		}
	}
}

func preserveResourceLabels(resource map[string]string, labels map[string]string, aliases ...labelAlias) {
	for _, alias := range aliases {
		sourceKey, targetKey := splitLabelAlias(string(alias))
		if value := resource[sourceKey]; value != "" {
			labels[targetKey] = value
		}
	}
}

func splitLabelAlias(alias string) (string, string) {
	parts := strings.SplitN(alias, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return alias, alias
}

// kvLabelValue converts bounded scalar OTLP attributes into Prometheus-safe
// string label values. Identity resolution continues to use kvString.
func kvLabelValue(attrs []*commonpb.KeyValue, key string) string {
	for _, kv := range attrs {
		if kv.Key != key || kv.Value == nil {
			continue
		}
		switch value := kv.Value.Value.(type) {
		case *commonpb.AnyValue_StringValue:
			return value.StringValue
		case *commonpb.AnyValue_BoolValue:
			return strconv.FormatBool(value.BoolValue)
		case *commonpb.AnyValue_IntValue:
			return strconv.FormatInt(value.IntValue, 10)
		case *commonpb.AnyValue_DoubleValue:
			return strconv.FormatFloat(value.DoubleValue, 'g', -1, 64)
		}
	}
	return ""
}

func applyServiceGraphEdgeLabels(
	ctx context.Context,
	resolver EntityResolver,
	plan *normalizationPlan,
) bool {
	clientResolved := false
	client := kvString(plan.attrs, semconv.AttrClient)
	if client != "" {
		plan.labels[semconv.LabelEdgeClient] = client
		if clientID, ok := resolveServiceMetricEntityID(
			ctx,
			resolver,
			plan.resource,
			client,
			kvString(plan.attrs, semconv.AttrClientNamespace),
		); ok {
			plan.labels[semconv.LabelEdgeClientID] = string(clientID)
			plan.entityID = clientID
			clientResolved = true
		}
	}

	server := kvString(plan.attrs, semconv.AttrServer)
	if server != "" {
		plan.labels[semconv.LabelEdgeServer] = server
		if serverID, ok := resolveServiceMetricEntityID(
			ctx,
			resolver,
			plan.resource,
			server,
			kvString(plan.attrs, semconv.AttrServerNamespace),
		); ok {
			plan.labels[semconv.LabelEdgeServerID] = string(serverID)
		}
	}
	return clientResolved
}

func firstKVString(attrs []*commonpb.KeyValue, keys ...string) string {
	for _, key := range keys {
		if value := kvString(attrs, key); value != "" {
			return value
		}
	}
	return ""
}

func kvString(attrs []*commonpb.KeyValue, key string) string {
	for _, kv := range attrs {
		if kv.Key == key && kv.Value != nil {
			if value, ok := kv.Value.Value.(*commonpb.AnyValue_StringValue); ok {
				return value.StringValue
			}
		}
	}
	return ""
}

func resolveServiceMetricEntityID(
	ctx context.Context,
	resolver EntityResolver,
	resource map[string]string,
	serviceName, namespaceHint string,
) (entities.EntityID, bool) {
	if serviceName == "" {
		return "", false
	}

	if otlpResolver, ok := resolver.(*OTLPEntityResolver); ok {
		if serviceID, _, ok := otlpResolver.resolveCanonicalService(ctx, serviceName, namespaceHint); ok {
			return serviceID, true
		}
	}

	if fallbackNS := fallbackMetricServiceNamespace(resource, serviceName); fallbackNS != "" {
		clusterName := resource[semconv.ResourceK8sClusterName]
		if otlpResolver, ok := resolver.(*OTLPEntityResolver); ok && otlpResolver.clusterName != "" {
			clusterName = otlpResolver.clusterName
		}
		return entities.ComputeEntityID(entities.KindService, clusterName, fallbackNS, serviceName), true
	}

	return "", false
}

func fallbackMetricServiceNamespace(resource map[string]string, serviceName string) string {
	namespace := resource[semconv.ResourceK8sNamespaceName]
	if namespace == "" {
		return ""
	}

	resourceService := resource[semconv.ResourceServiceName]
	if resourceService == "" || resourceService == serviceName {
		return namespace
	}
	if resource[semconv.ResourceK8sNodeName] != "" {
		return ""
	}
	return namespace
}
