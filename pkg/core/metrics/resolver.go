package metrics

import (
	"fmt"
	"sort"

	"github.com/erickivel/oksg/pkg/discovery/profile"
)

// Assignment maps a canonical metric name to the logical source ID that won
// the priority race for it. Metrics whose Sources are all unavailable are
// omitted (best-effort missing data).
type Assignment map[string]string

// SelectedSources is the set of source IDs that won at least one metric in
// the catalog given a particular ClusterProfile. The planner uses this to
// decide which receivers/connectors to deploy.
type SelectedSources map[string]bool

// Has reports whether sourceID was selected.
func (s SelectedSources) Has(sourceID string) bool { return s[sourceID] }

// Resolve picks the winning source for every metric in catalog given the
// availability information in prof. Returns the canonical-to-source assignment
// and the set of sources that won at least one metric.
//
// PolicyPriority (the default) picks the SourceRef with the smallest Priority
// whose registry Detect predicate returns true. PolicyDisjoint and PolicyAll
// are reserved for future work and currently produce an error so unimplemented
// behavior never silently ships.
func Resolve(catalog []CanonicalMetric, registry []LogicalSource, prof *profile.ClusterProfile) (Assignment, SelectedSources, error) {
	known := make(map[string]LogicalSource, len(registry))
	for _, source := range registry {
		known[source.ID] = source
	}

	available := make(map[string]bool, len(registry))
	for _, source := range registry {
		if source.Detect != nil && source.Detect(prof) {
			available[source.ID] = true
		}
	}

	assignment := make(Assignment)
	selected := make(SelectedSources)

	for _, metric := range catalog {
		if len(metric.Sources) == 0 {
			continue
		}
		policy := metric.SelectionPolicy
		if policy == "" {
			policy = PolicyPriority
		}
		if policy != PolicyPriority {
			return nil, nil, fmt.Errorf("metric %q uses unimplemented selection policy %q", metric.Name, policy)
		}

		for _, ref := range metric.Sources {
			if _, ok := known[ref.ID]; !ok {
				return nil, nil, fmt.Errorf("metric %q references unknown source %q", metric.Name, ref.ID)
			}
		}

		refs := make([]SourceRef, len(metric.Sources))
		copy(refs, metric.Sources)
		sort.SliceStable(refs, func(i, j int) bool { return refs[i].Priority < refs[j].Priority })

		for _, ref := range refs {
			if !available[ref.ID] {
				continue
			}
			assignment[metric.Name] = ref.ID
			selected[ref.ID] = true
			break
		}
	}

	return assignment, selected, nil
}
