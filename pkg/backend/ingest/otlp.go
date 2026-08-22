package ingest

import (
	"context"
	"fmt"
	"time"

	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/erickivel/oksg/pkg/core/schema"
)

type EnvelopeBuilder struct {
	clock func() time.Time
}

func NewEnvelopeBuilder(clock func() time.Time) *EnvelopeBuilder {
	if clock == nil {
		clock = func() time.Time {
			return time.Now().UTC()
		}
	}
	return &EnvelopeBuilder{clock: clock}
}

func (b *EnvelopeBuilder) BuildMetricEnvelopes(source string, request *colmetricpb.ExportMetricsServiceRequest) ([]schema.RawEnvelope, error) {
	if request == nil {
		return nil, fmt.Errorf("metrics request is required")
	}

	envelopes := make([]schema.RawEnvelope, 0, len(request.ResourceMetrics))
	for _, resourceMetrics := range request.ResourceMetrics {
		payload, err := marshalPayload(resourceMetrics)
		if err != nil {
			return nil, err
		}

		envelope, err := schema.NewRawEnvelope(source, schema.SignalMetric, resourceAttributes(resourceMetrics.Resource), payload, b.timestamp())
		if err != nil {
			return nil, err
		}
		envelopes = append(envelopes, envelope)
	}
	return envelopes, nil
}

func (b *EnvelopeBuilder) BuildTraceEnvelopes(source string, request *coltracepb.ExportTraceServiceRequest) ([]schema.RawEnvelope, error) {
	if request == nil {
		return nil, fmt.Errorf("trace request is required")
	}

	envelopes := make([]schema.RawEnvelope, 0, len(request.ResourceSpans))
	for _, resourceSpans := range request.ResourceSpans {
		payload, err := marshalPayload(resourceSpans)
		if err != nil {
			return nil, err
		}

		envelope, err := schema.NewRawEnvelope(source, schema.SignalTrace, resourceAttributes(resourceSpans.Resource), payload, b.timestamp())
		if err != nil {
			return nil, err
		}
		envelopes = append(envelopes, envelope)
	}
	return envelopes, nil
}

func (b *EnvelopeBuilder) timestamp() time.Time {
	if b == nil || b.clock == nil {
		return time.Now().UTC()
	}
	now := b.clock()
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

type SignalRouter struct {
	metrics Ingestor
	traces  Ingestor
}

// NewSignalRouter creates a router that dispatches envelopes by signal type.
func NewSignalRouter(metrics, traces Ingestor) *SignalRouter {
	return &SignalRouter{
		metrics: metrics,
		traces:  traces,
	}
}

func (r *SignalRouter) Route(ctx context.Context, envelope schema.RawEnvelope) error {
	switch envelope.SignalType {
	case schema.SignalMetric:
		if r.metrics == nil {
			return fmt.Errorf("metric ingestor is not configured")
		}
		return r.metrics.Ingest(ctx, envelope)
	case schema.SignalTrace:
		if r.traces == nil {
			return fmt.Errorf("trace ingestor is not configured")
		}
		return r.traces.Ingest(ctx, envelope)
	default:
		return fmt.Errorf("unsupported signal type: %s", envelope.SignalType)
	}
}

func marshalPayload(message proto.Message) ([]byte, error) {
	payload, err := protojson.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	return payload, nil
}

func resourceAttributes(resource *resourcepb.Resource) map[string]string {
	if resource == nil {
		return nil
	}

	attributes := make(map[string]string, len(resource.Attributes))
	for _, attribute := range resource.Attributes {
		if attribute == nil || attribute.Value == nil {
			continue
		}
		attributes[attribute.Key] = attributeValue(attribute.Value)
	}
	if len(attributes) == 0 {
		return nil
	}
	return attributes
}

func attributeValue(value *commonpb.AnyValue) string {
	switch current := value.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return current.StringValue
	case *commonpb.AnyValue_BoolValue:
		if current.BoolValue {
			return "true"
		}
		return "false"
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", current.IntValue)
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%g", current.DoubleValue)
	case *commonpb.AnyValue_BytesValue:
		return string(current.BytesValue)
	default:
		payload, err := protojson.Marshal(value)
		if err != nil {
			return ""
		}
		return string(payload)
	}
}
