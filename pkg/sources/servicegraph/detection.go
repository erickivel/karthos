package servicegraph

import (
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources/detection"
)

const (
	otlpAnnotationTraces = "oksg.io/otlp-traces"
	otlpGRPCPort         = int32(4317)
	otlpHTTPPort         = int32(4318)
)

// Detect reports the first deterministic supported trace producer for the
// derived servicegraph source.
func Detect(inventory detection.Inventory) profile.SourceCapability {
	services := append([]corev1.Service(nil), inventory.Services...)
	sort.Slice(services, func(i, j int) bool {
		if services[i].Namespace != services[j].Namespace {
			return services[i].Namespace < services[j].Namespace
		}
		return services[i].Name < services[j].Name
	})
	capability := profile.SourceCapability{SourceID: metrics.SourceTracesServicegraph, Kind: profile.SourceKindDerived, Status: profile.CapabilityUnavailable}
	for _, service := range services {
		if evidence, ok := traceEvidence(service); ok {
			capability.Status = profile.CapabilityAvailable
			capability.Signals = []string{"metric"}
			capability.Evidence.OTLP = &evidence
			return capability
		}
	}
	return capability
}

func traceEvidence(service corev1.Service) (profile.OTLPEvidence, bool) {
	annotated := strings.EqualFold(service.Annotations[otlpAnnotationTraces], "true")
	grpc, http := false, false
	for _, port := range service.Spec.Ports {
		grpc = grpc || port.Port == otlpGRPCPort
		http = http || port.Port == otlpHTTPPort
	}
	if !annotated && !grpc && !http {
		return profile.OTLPEvidence{}, false
	}
	return profile.OTLPEvidence{TracesEnabled: true}, true
}
