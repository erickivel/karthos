package kubelet

import (
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources/detection"
)

// Detect reports kubelet telemetry capability after the Kubernetes adapter
// has successfully completed its required inventory collection.
func Detect(detection.Inventory) profile.SourceCapability {
	return profile.SourceCapability{SourceID: metrics.SourceKubelet, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}}
}
