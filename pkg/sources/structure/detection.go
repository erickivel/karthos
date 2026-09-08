package structure

import (
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/detection"
)

// Detect reports structural discovery after the Kubernetes adapter has
// successfully listed its required inventory.
func Detect(detection.Inventory) profile.SourceCapability {
	return profile.SourceCapability{SourceID: metrics.SourceKubernetesStructure, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"structure"}}
}
