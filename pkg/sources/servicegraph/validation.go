package servicegraph

import (
	"fmt"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources/contract"
)

// ValidateCapability owns the derived servicegraph trace-producer contract.
func ValidateCapability(capability profile.SourceCapability) error {
	if err := contract.ValidateIdentity(capability, metrics.SourceTracesServicegraph, profile.SourceKindDerived); err != nil {
		return err
	}
	if capability.Status == profile.CapabilityUnavailable {
		if err := contract.ValidateStatusSignals(capability, profile.CapabilityUnavailable); err != nil {
			return err
		}
		return contract.RequireNoEvidence(capability.Evidence)
	}
	if err := contract.ValidateStatusSignals(capability, profile.CapabilityAvailable, "metric"); err != nil {
		return err
	}
	if len(capability.Evidence.PrometheusTargets) != 0 || capability.Evidence.Mesh != nil || capability.Evidence.OTLP == nil || !capability.Evidence.OTLP.TracesEnabled {
		return fmt.Errorf("traces.servicegraph requires trace-enabled OTLP evidence only")
	}
	return nil
}
