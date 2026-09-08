package kubecluster

import (
	"fmt"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources/contract"
)

// ValidateCapability owns the supported Kubernetes cluster-metric contract.
func ValidateCapability(capability profile.SourceCapability) error {
	if err := contract.ValidateIdentity(capability, metrics.SourceKubeCluster, profile.SourceKindDirect); err != nil {
		return err
	}
	if err := contract.ValidateStatusSignals(capability, profile.CapabilityAvailable, "metric"); err != nil {
		return err
	}
	if err := contract.RequireNoEvidence(capability.Evidence); err != nil {
		return fmt.Errorf("kube-cluster %w", err)
	}
	return nil
}
