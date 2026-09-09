package contract

import (
	"fmt"

	"github.com/erickivel/karthos/pkg/discovery/profile"
)

// ValidateIdentity verifies the immutable identity supplied by one compiled
// source module. Source-specific packages add their own status and evidence
// rules on top of this shared structural check.
func ValidateIdentity(capability profile.SourceCapability, sourceID string, kind profile.SourceKind) error {
	if capability.SourceID != sourceID {
		return fmt.Errorf("expected source ID %q, got %q", sourceID, capability.SourceID)
	}
	if capability.Kind != kind {
		return fmt.Errorf("expected source kind %q, got %q", kind, capability.Kind)
	}
	return nil
}

func ValidateStatusSignals(capability profile.SourceCapability, status profile.CapabilityStatus, signals ...string) error {
	if capability.Status != status {
		return fmt.Errorf("expected status %q, got %q", status, capability.Status)
	}
	if len(capability.Signals) != len(signals) {
		return fmt.Errorf("expected signals %q, got %q", signals, capability.Signals)
	}
	for index, signal := range signals {
		if capability.Signals[index] != signal {
			return fmt.Errorf("expected signals %q, got %q", signals, capability.Signals)
		}
	}
	return nil
}

func HasEvidence(evidence profile.CapabilityEvidence) bool {
	return len(evidence.PrometheusTargets) != 0 || evidence.OTLP != nil || evidence.Mesh != nil
}

func RequireNoEvidence(evidence profile.CapabilityEvidence) error {
	if HasEvidence(evidence) {
		return fmt.Errorf("does not allow capability evidence")
	}
	return nil
}
