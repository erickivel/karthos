package kubelet

import (
	"fmt"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources/contract"
)

// ValidateCapability owns the supported kubelet capability contract.
func ValidateCapability(capability profile.SourceCapability) error {
	if err := contract.ValidateIdentity(capability, metrics.SourceKubelet, profile.SourceKindDirect); err != nil {
		return err
	}
	if err := contract.ValidateStatusSignals(capability, profile.CapabilityAvailable, "metric"); err != nil {
		return err
	}
	if err := contract.RequireNoEvidence(capability.Evidence); err != nil {
		return fmt.Errorf("kubelet %w", err)
	}
	return nil
}
