package hostos

import (
	collectorspec "github.com/erickivel/karthos/pkg/collector/spec"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
)

func Contribute(plan *collectorspec.CollectionPlan, _ profile.SourceCapability, selected bool) {
	if !selected {
		return
	}
	plan.Agent.HostOS = &collectorspec.HostOS{SourceID: metrics.SourceHostOS, CollectionInterval: "30s", RootPath: "/hostfs", Scrapers: []string{"cpu", "memory", "disk", "filesystem", "network"}}
	plan.Agent.SourceStamps = append(plan.Agent.SourceStamps, collectorspec.SourceStamp{SourceID: metrics.SourceHostOS, MetricPatterns: []string{`^system\.`}, OnlyIfUnset: true})
}
