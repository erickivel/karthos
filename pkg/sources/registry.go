// Package sources declares the compiled source modules. Modules are linked into
// the binary deliberately; this is one static ownership boundary, not a
// runtime plugin loader.
package sources

import (
	"context"
	"fmt"
	"sort"

	collectorspec "github.com/erickivel/oksg/pkg/collector/spec"
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

// Module owns one source's capability identity, wire-name dialect, semantic
// collection requirements, and source-specific normalization. Structural
// modules intentionally have no metric dialect or collection requirements.
type Module struct {
	ID           string
	Kind         profile.SourceKind
	MetricSource bool
	Dialect      metrics.Dialect
	Detect       func(detection.Inventory) profile.SourceCapability
	Contribute   func(*collectorspec.CollectionPlan, profile.SourceCapability, bool)
	Normalize    contract.Normalizer
}

// Registry is the one compiled inventory of source modules.
type Registry struct{ modules []Module }

func DefaultRegistry() Registry {
	return Registry{modules: []Module{
		{ID: IDKubernetesStructure, Kind: profile.SourceKindDirect, Detect: structure.Detect},
		{ID: IDKubelet, Kind: profile.SourceKindDirect, Dialect: metrics.KubeletDialect, MetricSource: true, Detect: kubelet.Detect, Contribute: kubelet.Contribute, Normalize: kubelet.Normalize},
		{ID: IDHostOS, Kind: profile.SourceKindDirect, MetricSource: true, Detect: hostos.Detect, Contribute: hostos.Contribute, Normalize: hostos.Normalize},
		{ID: IDKubeCluster, Kind: profile.SourceKindDirect, Dialect: metrics.KubeClusterDialect, MetricSource: true, Detect: kubecluster.Detect, Contribute: kubecluster.Contribute, Normalize: kubecluster.Normalize},
		{ID: IDPrometheusApp, Kind: profile.SourceKindDirect, Dialect: metrics.PrometheusDialect, MetricSource: true, Detect: prometheusapp.Detect, Contribute: prometheusapp.Contribute, Normalize: prometheusapp.Normalize},
		{ID: IDMeshIstio, Kind: profile.SourceKindDirect, Dialect: metrics.IstioDialect, MetricSource: true, Detect: istio.Detect, Contribute: istio.Contribute, Normalize: istio.Normalize},
		{ID: IDTracesServicegraph, Kind: profile.SourceKindDerived, Dialect: metrics.ServicegraphDialect, MetricSource: true, Detect: servicegraph.Detect, Contribute: servicegraph.Contribute, Normalize: servicegraph.Normalize},
	}}
}

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

func (r Registry) ContractInventory() contract.StaticSourceInventory {
	return contract.StaticSourceInventory{ModuleIDs: r.IDs(), MetricSourceIDs: r.MetricIDs()}
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

// ResolveSourcePlan performs source arbitration once, producing the mounted
// contract used later by generic collection composition and backend startup.
func (r Registry) ResolveSourcePlan(capabilityProfile *profile.ClusterProfile) (contract.ResolvedSourcePlan, error) {
	if err := capabilityProfile.Validate(r.IDs()); err != nil {
		return contract.ResolvedSourcePlan{}, fmt.Errorf("validate capability profile: %w", err)
	}
	assignment, selected, err := metrics.Resolve(metrics.DefaultCatalog(), r.MetricIDs(), capabilityProfile.IsAvailable)
	if err != nil {
		return contract.ResolvedSourcePlan{}, fmt.Errorf("resolve sources: %w", err)
	}
	fingerprint, err := capabilityProfile.Fingerprint()
	if err != nil {
		return contract.ResolvedSourcePlan{}, fmt.Errorf("fingerprint capability profile: %w", err)
	}
	selectedSourceIDs := make([]string, 0, len(selected))
	for sourceID := range selected {
		selectedSourceIDs = append(selectedSourceIDs, sourceID)
	}
	sort.Strings(selectedSourceIDs)
	return contract.ResolvedSourcePlan{ProfileFingerprint: fingerprint, SelectedSources: selectedSourceIDs, MetricAssignments: assignment}, nil
}

// AppendRequirements lets each available static module append typed semantic
// requirements. Selection gates metric collection; servicegraph may still
// request trace intake when it is available but loses metric arbitration.
func (r Registry) AppendRequirements(out *collectorspec.CollectionPlan, capabilityProfile *profile.ClusterProfile, sourcePlan contract.ResolvedSourcePlan) error {
	if err := sourcePlan.Validate(capabilityProfile, r.ContractInventory()); err != nil {
		return err
	}
	selected := make(map[string]bool, len(sourcePlan.SelectedSources))
	for _, sourceID := range sourcePlan.SelectedSources {
		selected[sourceID] = true
	}
	for _, module := range r.modules {
		capability, _ := capabilityProfile.Capability(module.ID)
		if capability.Status != profile.CapabilityAvailable || module.Contribute == nil {
			continue
		}
		module.Contribute(out, capability, selected[module.ID])
	}
	for sourceID := range selected {
		module, ok := r.Lookup(sourceID)
		if !ok || !module.MetricSource || module.Contribute == nil {
			return fmt.Errorf("selected source %q has no semantic collection requirements", sourceID)
		}
	}
	out.Normalize()
	return nil
}
