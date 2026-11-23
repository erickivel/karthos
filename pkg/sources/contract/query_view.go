package contract

import "github.com/erickivel/karthos/pkg/core/metrics"

// LabelMatcher is a source-owned semantic predicate rendered by a storage adapter.
type LabelMatcher struct{ Name, Operator, Value string }

// QueryBranch selects original series before counter rollups. Zero retains
// classified successful series as evidence of zero failures.
type QueryBranch struct {
	Matchers []LabelMatcher
	Zero     bool
}
type QueryView struct {
	SourceID    string
	MetricName  string
	InputMetric string
	Branches    []QueryBranch
	Derivation  metrics.Derivation
}
