// Package spec defines the provider-neutral collection requirements compiled
// from the selected static source modules.
package spec

import (
	"sort"

	"github.com/erickivel/oksg/pkg/discovery/profile"
)

type CollectionPlan struct {
	ClusterName    string
	Namespace      string
	EnabledSignals []string
	Agent          AgentRequirements
	Gateway        GatewayRequirements
}

type AgentRequirements struct {
	Kubelet      *Kubelet
	HostOS       *HostOS
	Istio        *Istio
	OTLP         OTLPReceiver
	SourceStamps []SourceStamp
}

type GatewayRequirements struct {
	KubeCluster  *KubeCluster
	Prometheus   *Prometheus
	ServiceGraph *ServiceGraph
	OTLP         OTLPReceiver
	SourceStamps []SourceStamp
}

// OTLPReceiver describes signal intake, not a collector-specific component.
type OTLPReceiver struct {
	Metrics bool
	Traces  bool
}

type Kubelet struct {
	SourceID            string
	CollectionInterval  string
	MetricGroups        []string
	ExtraMetadataLabels []string
}

type HostOS struct {
	SourceID           string
	CollectionInterval string
	RootPath           string
	Scrapers           []string
}

type KubeCluster struct {
	SourceID           string
	CollectionInterval string
	NodeConditions     []string
	EnabledMetricNames []string
}

type Prometheus struct {
	SourceID string
	Targets  []profile.PrometheusTarget
}

type Istio struct {
	SourceID           string
	Flavor             string
	MetricsPort        int32
	MetricsPath        string
	InjectedNamespaces []string
}

type ServiceGraph struct {
	SourceID                string
	MetricsFlushInterval    string
	LatencyHistogramBuckets []string
	Dimensions              []string
	StoreTTL                string
	StoreMaxItems           int
}

// SourceStamp describes source attribution independently from the eventual
// collector processor syntax.
type SourceStamp struct {
	SourceID         string
	ExactMetricNames []string
	MetricPatterns   []string
	OnlyIfUnset      bool
}

func (p *CollectionPlan) Normalize() {
	sort.Strings(p.EnabledSignals)
	sortSourceStamps(p.Agent.SourceStamps)
	sortSourceStamps(p.Gateway.SourceStamps)
	if p.Agent.Kubelet != nil {
		sort.Strings(p.Agent.Kubelet.MetricGroups)
		sort.Strings(p.Agent.Kubelet.ExtraMetadataLabels)
	}
	if p.Agent.HostOS != nil {
		sort.Strings(p.Agent.HostOS.Scrapers)
	}
	if p.Agent.Istio != nil {
		sort.Strings(p.Agent.Istio.InjectedNamespaces)
	}
	if p.Gateway.KubeCluster != nil {
		sort.Strings(p.Gateway.KubeCluster.NodeConditions)
		sort.Strings(p.Gateway.KubeCluster.EnabledMetricNames)
	}
	if p.Gateway.Prometheus != nil {
		sort.Slice(p.Gateway.Prometheus.Targets, func(i, j int) bool {
			left, right := p.Gateway.Prometheus.Targets[i], p.Gateway.Prometheus.Targets[j]
			if left.Namespace != right.Namespace {
				return left.Namespace < right.Namespace
			}
			if left.ServiceName != right.ServiceName {
				return left.ServiceName < right.ServiceName
			}
			return left.Port < right.Port
		})
	}
	if p.Gateway.ServiceGraph != nil {
		sort.Strings(p.Gateway.ServiceGraph.LatencyHistogramBuckets)
		sort.Strings(p.Gateway.ServiceGraph.Dimensions)
	}
}

func sortSourceStamps(stamps []SourceStamp) {
	sort.Slice(stamps, func(i, j int) bool { return stamps[i].SourceID < stamps[j].SourceID })
	for i := range stamps {
		sort.Strings(stamps[i].ExactMetricNames)
		sort.Strings(stamps[i].MetricPatterns)
	}
}
