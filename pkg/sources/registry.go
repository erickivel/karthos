// Package sources declares the compiled source modules. Modules are linked into
// the binary deliberately; this is one static ownership boundary, not a
// runtime plugin loader.
package sources

import (
	"context"
	"fmt"
	"sort"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	collectorspec "github.com/erickivel/karthos/pkg/collector/spec"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/contract"
	"github.com/erickivel/karthos/pkg/sources/detection"
	"github.com/erickivel/karthos/pkg/sources/hostos"
	"github.com/erickivel/karthos/pkg/sources/istio"
	"github.com/erickivel/karthos/pkg/sources/kubecluster"
	"github.com/erickivel/karthos/pkg/sources/kubelet"
	"github.com/erickivel/karthos/pkg/sources/prometheusapp"
	"github.com/erickivel/karthos/pkg/sources/servicegraph"
	"github.com/erickivel/karthos/pkg/sources/structure"
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

// Module owns one source's capability identity, raw-name parser, semantic
// collection requirements, and source-specific normalization. Structural
// modules intentionally have no metric parser or collection requirements.
type Module struct {
	ID           string
	Kind         profile.SourceKind
	MetricSource bool
	Parse        contract.MetricParser
	Detect       func(detection.Inventory) profile.SourceCapability
	Validate     func(profile.SourceCapability) error
	Contribute   func(*collectorspec.CollectionPlan, profile.SourceCapability, bool)
	Normalize    contract.Normalizer
	CallsEdge    contract.CallsEdgeClassifier
	QueryViews   func() []contract.QueryView
}

// Registry is the one compiled inventory of source modules.
type Registry struct{ modules []Module }

func DefaultRegistry() Registry {
	return Registry{modules: []Module{
		{ID: IDKubernetesStructure, Kind: profile.SourceKindDirect, Detect: structure.Detect, Validate: structure.ValidateCapability},
		{ID: IDKubelet, Kind: profile.SourceKindDirect, Parse: kubelet.Parse, MetricSource: true, Detect: kubelet.Detect, Validate: kubelet.ValidateCapability, Contribute: kubelet.Contribute, Normalize: kubelet.Normalize},
		{ID: IDHostOS, Kind: profile.SourceKindDirect, Parse: hostos.Parse, MetricSource: true, Detect: hostos.Detect, Validate: hostos.ValidateCapability, Contribute: hostos.Contribute, Normalize: hostos.Normalize},
		{ID: IDKubeCluster, Kind: profile.SourceKindDirect, Parse: kubecluster.Parse, MetricSource: true, Detect: kubecluster.Detect, Validate: kubecluster.ValidateCapability, Contribute: kubecluster.Contribute, Normalize: kubecluster.Normalize},
		{ID: IDPrometheusApp, Kind: profile.SourceKindDirect, Parse: prometheusapp.Parse, MetricSource: true, Detect: prometheusapp.Detect, Validate: prometheusapp.ValidateCapability, Contribute: prometheusapp.Contribute, Normalize: prometheusapp.Normalize},
		{ID: IDMeshIstio, Kind: profile.SourceKindDirect, Parse: istio.Parse, MetricSource: true, Detect: istio.Detect, Validate: istio.ValidateCapability, Contribute: istio.Contribute, Normalize: istio.Normalize, CallsEdge: istio.CallsEdgeMetadata, QueryViews: istio.QueryViews},
		{ID: IDTracesServicegraph, Kind: profile.SourceKindDerived, Parse: servicegraph.Parse, MetricSource: true, Detect: servicegraph.Detect, Validate: servicegraph.ValidateCapability, Contribute: servicegraph.Contribute, Normalize: servicegraph.Normalize, CallsEdge: servicegraph.CallsEdgeMetadata, QueryViews: servicegraph.QueryViews},
	}}
}

// ParseMetric dispatches raw metric interpretation through exactly the source
// named by the datapoint stamp. No cross-source fallback is allowed.
func (r Registry) ParseMetric(sourceID, rawMetricName string, attributes map[string]string) (contract.ParsedMetric, bool) {
	module, ok := r.Lookup(sourceID)
	if !ok || !module.MetricSource || module.Parse == nil {
		return contract.ParsedMetric{}, false
	}
	return module.Parse(rawMetricName, attributes)
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
		if module.Validate == nil {
			return nil, fmt.Errorf("compiled source module %q has no capability validator", module.ID)
		}
		if err := module.Validate(capability); err != nil {
			return nil, fmt.Errorf("validate capability from module %q: %w", module.ID, err)
		}
		capabilities = append(capabilities, capability)
	}
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i].SourceID < capabilities[j].SourceID })
	return capabilities, nil
}

// ValidateCapabilityProfile validates one complete capability profile through
// the static module that owns every source-specific rule.
func (r Registry) ValidateCapabilityProfile(capabilityProfile *profile.ClusterProfile) error {
	if err := capabilityProfile.Validate(r.IDs()); err != nil {
		return err
	}
	for _, module := range r.modules {
		capability, _ := capabilityProfile.Capability(module.ID)
		if module.Validate == nil {
			return fmt.Errorf("compiled source module %q has no capability validator", module.ID)
		}
		if err := module.Validate(capability); err != nil {
			return fmt.Errorf("validate %s capability: %w", module.ID, err)
		}
	}
	return nil
}

func (r Registry) Normalize(ctx context.Context, resolver contract.EntityResolver, plan *contract.Plan) error {
	module, ok := r.Lookup(plan.Source)
	if !ok || !module.MetricSource || module.Normalize == nil {
		return fmt.Errorf("no compiled source module registered for %q", plan.Source)
	}
	return module.Normalize(ctx, resolver, plan)
}

// CallsEdgeMetadata dispatches source-specific calls-edge recognition and
// metadata extraction through the compiled source module.
func (r Registry) CallsEdgeMetadata(sourceID, rawMetricName, canonicalMetricName string, attrs []*commonpb.KeyValue) (map[string]string, bool) {
	module, ok := r.Lookup(sourceID)
	if !ok || module.CallsEdge == nil {
		return nil, false
	}
	return module.CallsEdge(rawMetricName, canonicalMetricName, attrs)
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

// ResolveSourcePlan performs source arbitration once, producing the mounted
// contract used later by generic collection composition and backend startup.
func (r Registry) ResolveSourcePlan(capabilityProfile *profile.ClusterProfile) (contract.ResolvedSourcePlan, error) {
	if err := r.ValidateCapabilityProfile(capabilityProfile); err != nil {
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
	if err := r.ValidateCapabilityProfile(capabilityProfile); err != nil {
		return fmt.Errorf("validate capability profile: %w", err)
	}
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

// QueryViews returns the static source-owned interpretations of a public metric.
func (r Registry) QueryViews(metric string) []contract.QueryView {
	var views []contract.QueryView
	for _, module := range r.modules {
		if module.QueryViews != nil {
			for _, v := range module.QueryViews() {
				if v.MetricName == metric {
					views = append(views, v)
				}
			}
		}
	}
	return views
}
