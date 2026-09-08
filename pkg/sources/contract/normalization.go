// Package contract contains narrow source-module normalization and source-plan
// boundaries shared by the static registry, collector composition, and backend.
package contract

import (
	"context"
	"strconv"
	"strings"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
)

type EntityResolver interface {
	Resolve(context.Context, map[string]string) (entities.EntityID, error)
}
type ServiceResolver interface {
	ResolveServiceMetricEntity(context.Context, map[string]string, string, string) (entities.EntityID, bool)
}
type ClusterResolver interface{ ClusterName() string }

// ParsedMetric is the source-owned interpretation of one raw metric name.
// Generic ingestion validates the returned canonical name against the catalog
// and source plan before it persists any sample.
type ParsedMetric struct {
	CanonicalName    string
	ConversionFactor float64
	UnitOverride     metrics.Unit
}

// MetricParser translates one stamped source's raw metric into a canonical
// candidate. Attributes are exposed as a simple string map because their
// source-specific meaning belongs to the source module, not generic ingest.
type MetricParser func(rawMetricName string, attributes map[string]string) (ParsedMetric, bool)

type Plan struct {
	RawMetricName    string
	MetricName       string
	Source           string
	Resource         map[string]string
	Attrs            []*commonpb.KeyValue
	Labels           map[string]string
	EntityID         entities.EntityID
	Skip             bool
	ConversionFactor float64
	Unit             metrics.Unit
	Derived          []DerivedMetric
}
type DerivedMetric struct {
	MetricName       string
	ConversionFactor float64
}
type Normalizer func(context.Context, EntityResolver, *Plan) error

// CallsEdgeClassifier identifies source-owned metrics that can derive a calls
// edge and returns the source-specific metadata to retain on that edge.
// Generic ingestion owns provenance validation and edge assembly.
type CallsEdgeClassifier func(rawMetricName, canonicalMetricName string, attrs []*commonpb.KeyValue) (map[string]string, bool)

func NewPlan(raw, canonical, source string, resource map[string]string, attrs []*commonpb.KeyValue, parsed ParsedMetric) Plan {
	return Plan{RawMetricName: raw, MetricName: canonical, Source: source, Resource: resource, Attrs: attrs, Labels: map[string]string{semconv.LabelSource: source}, ConversionFactor: parsed.ConversionFactor, Unit: parsed.UnitOverride}
}
func PreserveLabels(attrs []*commonpb.KeyValue, labels map[string]string, aliases ...string) {
	for _, alias := range aliases {
		source, target := splitAlias(alias)
		if value := LabelValue(attrs, source); value != "" {
			labels[target] = value
		}
	}
}
func PreserveResourceLabels(resource, labels map[string]string, aliases ...string) {
	for _, alias := range aliases {
		source, target := splitAlias(alias)
		if value := resource[source]; value != "" {
			labels[target] = value
		}
	}
}
func PreserveTraceLabels(attrs []*commonpb.KeyValue, labels map[string]string) {
	PreserveLabels(attrs, labels, "http.method:http_method", "http_method", semconv.AttrHTTPStatusCode+":http_status_code", semconv.AttrHTTPStatusCodeProm+":http_status_code", "http.status_code:http_status_code", "status_code", "status.code:status_code", "span.kind:span_kind", "span_kind")
}
func splitAlias(alias string) (string, string) {
	parts := strings.SplitN(alias, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return alias, alias
}
func LabelValue(attrs []*commonpb.KeyValue, key string) string {
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
func FirstLabelValue(attrs []*commonpb.KeyValue, keys ...string) string {
	for _, key := range keys {
		if value := LabelValue(attrs, key); value != "" {
			return value
		}
	}
	return ""
}

// AttributeStrings exposes raw telemetry attributes to a source parser without
// making generic ingestion understand source-specific attribute semantics.
func AttributeStrings(attrs []*commonpb.KeyValue) map[string]string {
	out := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		if attr == nil || attr.Key == "" {
			continue
		}
		if value := LabelValue(attrs, attr.Key); value != "" {
			out[attr.Key] = value
		}
	}
	return out
}

func ApplyServiceGraphEdgeLabels(ctx context.Context, resolver EntityResolver, plan *Plan) bool {
	client := LabelValue(plan.Attrs, semconv.AttrClient)
	clientResolved := false
	if client != "" {
		plan.Labels[semconv.LabelEdgeClient] = client
		if id, ok := ResolveServiceMetricEntity(ctx, resolver, plan.Resource, client, LabelValue(plan.Attrs, semconv.AttrClientNamespace)); ok {
			plan.Labels[semconv.LabelEdgeClientID] = string(id)
			plan.EntityID = id
			clientResolved = true
		}
	}
	server := LabelValue(plan.Attrs, semconv.AttrServer)
	if server != "" {
		plan.Labels[semconv.LabelEdgeServer] = server
		if id, ok := ResolveServiceMetricEntity(ctx, resolver, plan.Resource, server, LabelValue(plan.Attrs, semconv.AttrServerNamespace)); ok {
			plan.Labels[semconv.LabelEdgeServerID] = string(id)
		}
	}
	return clientResolved
}
func ResolveServiceMetricEntity(ctx context.Context, resolver EntityResolver, resource map[string]string, service, namespace string) (entities.EntityID, bool) {
	if service == "" {
		return "", false
	}
	if resolved, ok := resolver.(ServiceResolver); ok {
		return resolved.ResolveServiceMetricEntity(ctx, resource, service, namespace)
	}
	return "", false
}
