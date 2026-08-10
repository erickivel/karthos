package ingest

import (
	"context"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/sources"
	"github.com/erickivel/oksg/pkg/sources/contract"
)

func applySourceNormalization(ctx context.Context, resolver EntityResolver, plan *contract.Plan) error {
	if err := sources.DefaultRegistry().Normalize(ctx, resolver, plan); err != nil {
		return err
	}
	if plan.Skip || plan.EntityID != "" {
		return nil
	}
	entityID, err := resolver.Resolve(ctx, plan.Resource)
	if err != nil {
		return err
	}
	plan.EntityID = entityID
	return nil
}
func newNormalizationPlan(raw, canonical, source string, resource map[string]string, attrs []*commonpb.KeyValue, mapping metrics.OTLPMetricMapping) contract.Plan {
	return contract.NewPlan(raw, canonical, source, resource, attrs, mapping)
}
func firstKVString(attrs []*commonpb.KeyValue, keys ...string) string {
	return contract.FirstLabelValue(attrs, keys...)
}
func kvString(attrs []*commonpb.KeyValue, key string) string { return contract.LabelValue(attrs, key) }
