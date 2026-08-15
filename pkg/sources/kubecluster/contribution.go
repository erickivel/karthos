package kubecluster

import (
	collectorspec "github.com/erickivel/oksg/pkg/collector/spec"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func Contribute(plan *collectorspec.CollectionPlan, _ profile.SourceCapability, selected bool) {
	if !selected {
		return
	}
	plan.Gateway.KubeCluster = &collectorspec.KubeCluster{SourceID: metrics.SourceKubeCluster, CollectionInterval: "30s", NodeConditions: []string{"Ready", "MemoryPressure", "DiskPressure"}, EnabledMetricNames: []string{"k8s.node.condition", "k8s.container.ready", "k8s.container.restarts", "k8s.pod.status_reason"}}
	plan.Gateway.SourceStamps = append(plan.Gateway.SourceStamps, collectorspec.SourceStamp{SourceID: metrics.SourceKubeCluster, ExactMetricNames: []string{"k8s.container.cpu_request", "k8s.container.cpu_limit", "k8s.container.memory_request", "k8s.container.memory_limit", "k8s.container.ready", "k8s.container.restarts", "k8s.pod.phase", "k8s.pod.status_reason"}, MetricPatterns: []string{`^k8s\.node\.condition\.(ready|memory_pressure|disk_pressure)$`}, OnlyIfUnset: true})
}
