package kubecluster

import (
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/detection"
)

// Detect reports cluster-state telemetry capability after the Kubernetes
// adapter has successfully completed its required inventory collection.
func Detect(detection.Inventory) profile.SourceCapability {
	return profile.SourceCapability{SourceID: metrics.SourceKubeCluster, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}}
}
