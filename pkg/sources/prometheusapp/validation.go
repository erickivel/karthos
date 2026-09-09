package prometheusapp

import (
	"fmt"
	"strings"

	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

// ValidateCapability owns the supported Prometheus application-target contract.
func ValidateCapability(capability profile.SourceCapability) error {
	if err := contract.ValidateIdentity(capability, metrics.SourcePrometheusApp, profile.SourceKindDirect); err != nil {
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
	if capability.Evidence.OTLP != nil || capability.Evidence.Mesh != nil {
		return fmt.Errorf("prometheus.app only allows Prometheus target evidence")
	}
	if len(capability.Evidence.PrometheusTargets) == 0 {
		return fmt.Errorf("prometheus.app available capability requires at least one target")
	}
	seen := make(map[string]struct{}, len(capability.Evidence.PrometheusTargets))
	for _, target := range capability.Evidence.PrometheusTargets {
		if target.ServiceName == "" || target.Namespace == "" || target.Port < 1 || target.Port > 65535 || !strings.HasPrefix(target.Path, "/") {
			return fmt.Errorf("prometheus.app contains an invalid target")
		}
		key := target.Namespace + "/" + target.ServiceName + ":" + fmt.Sprint(target.Port) + target.Path
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("prometheus.app contains duplicate target %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}
