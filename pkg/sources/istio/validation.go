package istio

import (
	"fmt"
	"strings"

	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

// ValidateCapability owns the supported Istio coverage and evidence contract.
func ValidateCapability(capability profile.SourceCapability) error {
	if err := contract.ValidateIdentity(capability, metrics.SourceMeshIstio, profile.SourceKindDirect); err != nil {
		return err
	}
	switch capability.Status {
	case profile.CapabilityUnavailable:
		if err := contract.ValidateStatusSignals(capability, profile.CapabilityUnavailable); err != nil {
			return err
		}
		return contract.RequireNoEvidence(capability.Evidence)
	case profile.CapabilityPartial:
		if err := contract.ValidateStatusSignals(capability, profile.CapabilityPartial); err != nil {
			return err
		}
	case profile.CapabilityAvailable:
		if err := contract.ValidateStatusSignals(capability, profile.CapabilityAvailable, "metric"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("istio has unsupported status %q", capability.Status)
	}
	if len(capability.Evidence.PrometheusTargets) != 0 || capability.Evidence.OTLP != nil || capability.Evidence.Mesh == nil {
		return fmt.Errorf("istio requires mesh evidence only")
	}
	mesh := capability.Evidence.Mesh
	if mesh.Flavor != "istio" || mesh.Mode != "sidecar" || mesh.MetricsPort != istioMetricsPort || mesh.MetricsPath != istioMetricsPath || len(mesh.InjectedNamespaces) == 0 {
		return fmt.Errorf("istio contains invalid mesh evidence")
	}
	seen := make(map[string]struct{}, len(mesh.InjectedNamespaces))
	for _, namespace := range mesh.InjectedNamespaces {
		if strings.TrimSpace(namespace) == "" {
			return fmt.Errorf("istio contains an empty injected namespace")
		}
		if _, duplicate := seen[namespace]; duplicate {
			return fmt.Errorf("istio contains duplicate injected namespace %q", namespace)
		}
		seen[namespace] = struct{}{}
	}
	return nil
}
