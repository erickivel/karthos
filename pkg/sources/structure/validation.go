package structure

import (
	"fmt"

	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

// ValidateCapability owns the supported structural-discovery contract.
func ValidateCapability(capability profile.SourceCapability) error {
	if err := contract.ValidateIdentity(capability, metrics.SourceKubernetesStructure, profile.SourceKindDirect); err != nil {
		return err
	}
	if err := contract.ValidateStatusSignals(capability, profile.CapabilityAvailable, "structure"); err != nil {
		return err
	}
	if err := contract.RequireNoEvidence(capability.Evidence); err != nil {
		return fmt.Errorf("kubernetes.structure %w", err)
	}
	return nil
}
