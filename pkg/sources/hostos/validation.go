package hostos

import (
	"fmt"

	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

// ValidateCapability owns the supported host operating-system capability contract.
func ValidateCapability(capability profile.SourceCapability) error {
	if err := contract.ValidateIdentity(capability, metrics.SourceHostOS, profile.SourceKindDirect); err != nil {
		return err
	}
	if err := contract.ValidateStatusSignals(capability, profile.CapabilityAvailable, "metric"); err != nil {
		return err
	}
	if err := contract.RequireNoEvidence(capability.Evidence); err != nil {
		return fmt.Errorf("host-os %w", err)
	}
	return nil
}
