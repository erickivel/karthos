// Package sources declares the compiled source modules. Modules are linked into
// the binary deliberately; this is one static ownership boundary, not a
// runtime plugin loader.
package sources

import (
	"context"
	"fmt"
	"sort"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources/contract"
	"github.com/erickivel/oksg/pkg/sources/detection"
	"github.com/erickivel/oksg/pkg/sources/hostos"
	"github.com/erickivel/oksg/pkg/sources/istio"
	"github.com/erickivel/oksg/pkg/sources/kubecluster"
	"github.com/erickivel/oksg/pkg/sources/kubelet"
	"github.com/erickivel/oksg/pkg/sources/prometheusapp"
	"github.com/erickivel/oksg/pkg/sources/servicegraph"
	"github.com/erickivel/oksg/pkg/sources/structure"
)

const (
	IDKubernetesStructure = metrics.SourceKubernetesStructure
	IDKubelet             = metrics.SourceKubelet
	IDHostOS              = metrics.SourceHostOS
	IDKubeCluster         = metrics.SourceKubeCluster
	IDPrometheusApp       = metrics.SourcePrometheusApp
	IDMeshIstio           = metrics.SourceMeshIstio
	IDTracesServicegraph  = metrics.SourceTracesServicegraph
)

// CollectorContribution is the source-owned part of compiled OTel
// configuration. The OTel adapter only composes these fragments with generic
// transport, batching and export configuration.
type CollectorContribution struct {
	AgentReceivers         map[string]any
	GatewayReceivers       map[string]any
	GatewayConnectors      map[string]any
	AgentMetricReceivers   []string
	GatewayMetricReceivers []string
	AgentSourceStamps      []string
	GatewaySourceStamps    []string
}

// Module owns one source's capability identity, wire-name dialect, collector
// contribution and source stamps. Structural modules intentionally have no
// metric dialect or collector contribution.
type Module struct {
	ID           string
	Kind         profile.SourceKind
	MetricSource bool
	Dialect      metrics.Dialect
	Detect       func(detection.Inventory) profile.SourceCapability
	Contribute   func(*CollectorContribution, *profile.ClusterProfile)
	Normalize    contract.Normalizer
}

// Registry is the one compiled inventory of source modules.
type Registry struct{ modules []Module }

func DefaultRegistry() Registry {
	return Registry{modules: []Module{
		{ID: IDKubernetesStructure, Kind: profile.SourceKindDirect, Detect: structure.Detect},
		{
			ID: IDKubelet, Kind: profile.SourceKindDirect, Dialect: metrics.KubeletDialect, MetricSource: true, Detect: kubelet.Detect,
			Contribute: contributeKubelet, Normalize: kubelet.Normalize,
		},
		{ID: IDHostOS, Kind: profile.SourceKindDirect, MetricSource: true, Detect: hostos.Detect, Contribute: contributeHostOS, Normalize: hostos.Normalize},
		{
			ID: IDKubeCluster, Kind: profile.SourceKindDirect, Dialect: metrics.KubeClusterDialect, MetricSource: true, Detect: kubecluster.Detect,
			Contribute: contributeKubeCluster, Normalize: kubecluster.Normalize,
		},
		{
			ID: IDPrometheusApp, Kind: profile.SourceKindDirect, Dialect: metrics.PrometheusDialect, MetricSource: true, Detect: prometheusapp.Detect,
			Contribute: contributePrometheusApp, Normalize: prometheusapp.Normalize,
		},
		{
			ID: IDMeshIstio, Kind: profile.SourceKindDirect, Dialect: metrics.IstioDialect, MetricSource: true, Detect: istio.Detect,
			Contribute: contributeMeshIstio, Normalize: istio.Normalize,
		},
		{
			ID: IDTracesServicegraph, Kind: profile.SourceKindDerived, Dialect: metrics.ServicegraphDialect, MetricSource: true, Detect: servicegraph.Detect,
			Contribute: contributeServicegraph, Normalize: servicegraph.Normalize,
		},
	}}
}

// Detect runs every compiled source detector in static registry order. A
// detector may mark its source unavailable, but cannot impersonate another
// module or change its direct/derived identity.
func (r Registry) Detect(inventory detection.Inventory) ([]profile.SourceCapability, error) {
	capabilities := make([]profile.SourceCapability, 0, len(r.modules))
	for _, module := range r.modules {
		if module.Detect == nil {
			return nil, fmt.Errorf("compiled source module %q has no detector", module.ID)
		}
		capability := module.Detect(inventory)
		if capability.SourceID != module.ID {
			return nil, fmt.Errorf("compiled source module %q returned capability for %q", module.ID, capability.SourceID)
		}
		if capability.Kind != module.Kind {
			return nil, fmt.Errorf("compiled source module %q returned kind %q, expected %q", module.ID, capability.Kind, module.Kind)
		}
		capabilities = append(capabilities, capability)
	}
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i].SourceID < capabilities[j].SourceID })
	return capabilities, nil
}

// Normalize applies the selected compiled module and leaves the default entity
// resolution to the generic ingestion layer when the module did not specialize it.
func (r Registry) Normalize(ctx context.Context, resolver contract.EntityResolver, plan *contract.Plan) error {
	module, ok := r.Lookup(plan.Source)
	if !ok || !module.MetricSource || module.Normalize == nil {
		return fmt.Errorf("no compiled source module registered for %q", plan.Source)
	}
	return module.Normalize(ctx, resolver, plan)
}

func (r Registry) IDs() []string {
	ids := make([]string, len(r.modules))
	for i, module := range r.modules {
		ids[i] = module.ID
	}
	return ids
}

func (r Registry) MetricIDs() []string {
	ids := make([]string, 0, len(r.modules))
	for _, module := range r.modules {
		if module.MetricSource {
			ids = append(ids, module.ID)
		}
	}
	return ids
}

func (r Registry) Lookup(id string) (Module, bool) {
	for _, module := range r.modules {
		if module.ID == id {
			return module, true
		}
	}
	return Module{}, false
}

func (r Registry) Dialects() map[string]metrics.Dialect {
	dialects := make(map[string]metrics.Dialect, len(r.modules))
	for _, module := range r.modules {
		if module.Dialect != nil {
			dialects[module.ID] = module.Dialect
		}
	}
	return dialects
}

// Contribute applies exactly the selected source modules in stable registry
// order. A selected ID without a compiled telemetry module fails closed.
func (r Registry) Contribute(selected metrics.SelectedSources, capabilityProfile *profile.ClusterProfile) (CollectorContribution, error) {
	result := CollectorContribution{
		AgentReceivers:    map[string]any{},
		GatewayReceivers:  map[string]any{},
		GatewayConnectors: map[string]any{},
	}
	for _, module := range r.modules {
		if !selected.Has(module.ID) {
			continue
		}
		if !module.MetricSource || module.Contribute == nil {
			return CollectorContribution{}, fmt.Errorf("selected source %q has no telemetry contribution", module.ID)
		}
		module.Contribute(&result, capabilityProfile)
	}
	for sourceID := range selected {
		module, ok := r.Lookup(sourceID)
		if !ok || !module.MetricSource {
			return CollectorContribution{}, fmt.Errorf("selected source %q is not a compiled telemetry module", sourceID)
		}
	}
	sort.Strings(result.AgentMetricReceivers)
	sort.Strings(result.GatewayMetricReceivers)
	return result, nil
}
