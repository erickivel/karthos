package kubelet

import (
	collectorspec "github.com/erickivel/karthos/pkg/collector/spec"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
)

func Contribute(plan *collectorspec.CollectionPlan, _ profile.SourceCapability, selected bool) {
	if !selected {
		return
	}
	plan.Agent.Kubelet = &collectorspec.Kubelet{SourceID: metrics.SourceKubelet, CollectionInterval: "30s", MetricGroups: []string{"node", "pod", "container", "volume"}, ExtraMetadataLabels: []string{"k8s.volume.type"}}
	plan.Agent.SourceStamps = append(plan.Agent.SourceStamps, collectorspec.SourceStamp{SourceID: metrics.SourceKubelet, MetricPatterns: []string{`^k8s\.pod\.(cpu|memory)\.`, `^k8s\.volume\.`, `^container\.`, `^k8s\.node\.(cpu\.usage|memory\.working_set)$`}, OnlyIfUnset: true})
}
