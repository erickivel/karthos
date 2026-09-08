package istio

import (
	collectorspec "github.com/erickivel/oksg/pkg/collector/spec"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func Contribute(plan *collectorspec.CollectionPlan, capability profile.SourceCapability, selected bool) {
	if !selected || capability.Evidence.Mesh == nil {
		return
	}
	mesh := capability.Evidence.Mesh
	port, path := mesh.MetricsPort, mesh.MetricsPath
	if port == 0 {
		port = 15020
	}
	if path == "" {
		path = "/stats/prometheus"
	}
	plan.Agent.Istio = &collectorspec.Istio{SourceID: metrics.SourceMeshIstio, Flavor: mesh.Flavor, MetricsPort: port, MetricsPath: path, InjectedNamespaces: append([]string(nil), mesh.InjectedNamespaces...)}
}
