package contract

import (
	"fmt"

	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
)

// StaticSourceInventory is the compiled source inventory used to validate a
// mounted source plan without coupling that contract to a source registry.
type StaticSourceInventory struct {
	ModuleIDs       []string
	MetricSourceIDs []string
}

// ResolvedSourcePlan records the complete, deterministic result of source
// arbitration. Runtime code validates this artifact but never resolves it again.
type ResolvedSourcePlan struct {
	ProfileFingerprint string             `json:"profile_fingerprint"`
	SelectedSources    []string           `json:"selected_sources"`
	MetricAssignments  metrics.Assignment `json:"metric_assignments"`
}

func (p ResolvedSourcePlan) Validate(capabilityProfile *profile.ClusterProfile, inventory StaticSourceInventory) error {
	if err := capabilityProfile.Validate(inventory.ModuleIDs); err != nil {
		return fmt.Errorf("validate capability profile: %w", err)
	}
	fingerprint, err := capabilityProfile.Fingerprint()
	if err != nil {
		return fmt.Errorf("fingerprint capability profile: %w", err)
	}
	if p.ProfileFingerprint == "" || p.ProfileFingerprint != fingerprint {
		return fmt.Errorf("source plan profile fingerprint does not match capability profile")
	}

	metricSources := make(map[string]struct{}, len(inventory.MetricSourceIDs))
	for _, sourceID := range inventory.MetricSourceIDs {
		metricSources[sourceID] = struct{}{}
	}
	selected := make(map[string]struct{}, len(p.SelectedSources))
	for _, sourceID := range p.SelectedSources {
		if _, ok := metricSources[sourceID]; !ok {
			return fmt.Errorf("source plan selects unknown telemetry source %q", sourceID)
		}
		if _, duplicate := selected[sourceID]; duplicate {
			return fmt.Errorf("source plan selects source %q more than once", sourceID)
		}
		if !capabilityProfile.IsAvailable(sourceID) {
			return fmt.Errorf("source plan selects unavailable source %q", sourceID)
		}
		selected[sourceID] = struct{}{}
	}

	catalog := metrics.CatalogByName()
	selectedUsed := make(map[string]bool, len(selected))
	for metricName, sourceID := range p.MetricAssignments {
		metric, ok := catalog[metricName]
		if !ok {
			return fmt.Errorf("source plan assigns unknown catalog metric %q", metricName)
		}
		if _, ok := selected[sourceID]; !ok {
			return fmt.Errorf("source plan assigns metric %q to unselected source %q", metricName, sourceID)
		}
		if !metricHasSource(metric, sourceID) {
			return fmt.Errorf("source plan assigns metric %q to unsupported source %q", metricName, sourceID)
		}
		selectedUsed[sourceID] = true
	}
	for sourceID := range selected {
		if !selectedUsed[sourceID] {
			return fmt.Errorf("source plan selects unused source %q", sourceID)
		}
	}
	for metricName, metric := range catalog {
		available := false
		for _, source := range metric.Sources {
			if capabilityProfile.IsAvailable(source.ID) {
				available = true
				break
			}
		}
		_, assigned := p.MetricAssignments[metricName]
		if available && !assigned {
			return fmt.Errorf("source plan must assign every available catalog metric: %q", metricName)
		}
		if !available && assigned {
			return fmt.Errorf("source plan assigns unavailable catalog metric %q", metricName)
		}
	}
	return nil
}

func metricHasSource(metric metrics.CanonicalMetric, sourceID string) bool {
	for _, source := range metric.Sources {
		if source.ID == sourceID {
			return true
		}
	}
	return false
}
