// Package profile defines the capability snapshot used to compile one
// deterministic telemetry deployment. It intentionally contains no structural
// topology: kubernetes.structure refreshes topology directly at runtime.
package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/erickivel/oksg/pkg/core/entities"
)

type ClusterProfile struct {
	ClusterID    entities.EntityID  `json:"cluster_id"`
	ClusterName  string             `json:"cluster_name"`
	DiscoveredAt time.Time          `json:"discovered_at"`
	Capabilities []SourceCapability `json:"capabilities"`
}

type SourceKind string

const (
	SourceKindDirect  SourceKind = "direct"
	SourceKindDerived SourceKind = "derived"
)

type CapabilityStatus string

const (
	CapabilityAvailable   CapabilityStatus = "available"
	CapabilityPartial     CapabilityStatus = "partial"
	CapabilityUnavailable CapabilityStatus = "unavailable"
)

// SourceCapability records the bounded evidence from one successfully-run
// module detector. All compiled modules must have exactly one record.
type SourceCapability struct {
	SourceID string             `json:"source_id"`
	Kind     SourceKind         `json:"kind"`
	Status   CapabilityStatus   `json:"status"`
	Signals  []string           `json:"signals"`
	Evidence CapabilityEvidence `json:"evidence,omitempty"`
}

type CapabilityEvidence struct {
	PrometheusTargets []PrometheusTarget `json:"prometheus_targets,omitempty"`
	OTLP              *OTLPEvidence      `json:"otlp,omitempty"`
	Mesh              *MeshEvidence      `json:"mesh,omitempty"`
}

type PrometheusTarget struct {
	ServiceName string `json:"service_name"`
	Namespace   string `json:"namespace"`
	Port        int32  `json:"port"`
	Path        string `json:"path"`
	Reachable   bool   `json:"reachable"`
}

type OTLPEvidence struct {
	MetricsEnabled bool   `json:"metrics_enabled"`
	TracesEnabled  bool   `json:"traces_enabled"`
	GRPCEndpoint   string `json:"grpc_endpoint,omitempty"`
	HTTPEndpoint   string `json:"http_endpoint,omitempty"`
}

type MeshEvidence struct {
	Flavor             string   `json:"flavor"`
	Mode               string   `json:"mode"`
	InjectedNamespaces []string `json:"injected_namespaces,omitempty"`
	MetricsPort        int32    `json:"metrics_port,omitempty"`
	MetricsPath        string   `json:"metrics_path,omitempty"`
}

func (p *ClusterProfile) Capability(sourceID string) (SourceCapability, bool) {
	if p == nil {
		return SourceCapability{}, false
	}
	for _, capability := range p.Capabilities {
		if capability.SourceID == sourceID {
			return capability, true
		}
	}
	return SourceCapability{}, false
}

func (p *ClusterProfile) IsAvailable(sourceID string) bool {
	capability, ok := p.Capability(sourceID)
	return ok && capability.Status == CapabilityAvailable
}

func (p *ClusterProfile) PrometheusTargets() []PrometheusTarget {
	capability, ok := p.Capability("prometheus.app")
	if !ok {
		return nil
	}
	return append([]PrometheusTarget(nil), capability.Evidence.PrometheusTargets...)
}

func (p *ClusterProfile) Mesh() *MeshEvidence {
	capability, ok := p.Capability("mesh.istio")
	if !ok || capability.Status != CapabilityAvailable || capability.Evidence.Mesh == nil {
		return nil
	}
	copy := *capability.Evidence.Mesh
	copy.InjectedNamespaces = append([]string(nil), copy.InjectedNamespaces...)
	return &copy
}

func (p *ClusterProfile) OTLPSignals() (metricsEnabled, tracesEnabled bool) {
	capability, ok := p.Capability("traces.servicegraph")
	if !ok || capability.Evidence.OTLP == nil {
		return false, false
	}
	return capability.Evidence.OTLP.MetricsEnabled, capability.Evidence.OTLP.TracesEnabled
}

// Sort normalizes collection ordering before serializing an artifact.
func (p *ClusterProfile) Sort() {
	if p == nil {
		return
	}
	sort.Slice(p.Capabilities, func(i, j int) bool { return p.Capabilities[i].SourceID < p.Capabilities[j].SourceID })
	for i := range p.Capabilities {
		sort.Strings(p.Capabilities[i].Signals)
		sort.Slice(p.Capabilities[i].Evidence.PrometheusTargets, func(a, b int) bool {
			left, right := p.Capabilities[i].Evidence.PrometheusTargets[a], p.Capabilities[i].Evidence.PrometheusTargets[b]
			if left.Namespace != right.Namespace {
				return left.Namespace < right.Namespace
			}
			if left.ServiceName != right.ServiceName {
				return left.ServiceName < right.ServiceName
			}
			return left.Port < right.Port
		})
	}
}

// Fingerprint binds a generated source plan to the exact capability snapshot
// that selected it. The digest is computed over a normalized copy so callers
// do not need to pre-sort discovery output.
func (p *ClusterProfile) Fingerprint() (string, error) {
	if p == nil {
		return "", nil
	}
	copy := *p
	copy.Capabilities = append([]SourceCapability(nil), p.Capabilities...)
	for i := range copy.Capabilities {
		copy.Capabilities[i].Signals = append([]string(nil), copy.Capabilities[i].Signals...)
		copy.Capabilities[i].Evidence.PrometheusTargets = append([]PrometheusTarget(nil), copy.Capabilities[i].Evidence.PrometheusTargets...)
	}
	copy.Sort()
	payload, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

// Validate requires an exact capability record for every compiled module.
// This prevents a partially generated profile from silently changing source
// selection at planning time.
func (p *ClusterProfile) Validate(expectedSourceIDs []string) error {
	if p == nil {
		return fmt.Errorf("capability profile is required")
	}
	seen := make(map[string]struct{}, len(p.Capabilities))
	for _, capability := range p.Capabilities {
		if capability.SourceID == "" {
			return fmt.Errorf("capability profile contains an empty source ID")
		}
		if _, exists := seen[capability.SourceID]; exists {
			return fmt.Errorf("capability profile contains duplicate source %q", capability.SourceID)
		}
		seen[capability.SourceID] = struct{}{}
	}
	for _, sourceID := range expectedSourceIDs {
		if _, exists := seen[sourceID]; !exists {
			return fmt.Errorf("capability profile is missing compiled module %q", sourceID)
		}
	}
	if len(seen) != len(expectedSourceIDs) {
		return fmt.Errorf("capability profile contains an unknown compiled module")
	}
	return nil
}
