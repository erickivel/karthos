package ingest

import (
	"strings"
	"sync/atomic"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
)

// SourceRejectionReason identifies why a recognized canonical metric
// datapoint was rejected by the provenance policy.
type SourceRejectionReason string

const (
	SourceRejectionMissing            SourceRejectionReason = "missing_source"
	SourceRejectionUnknown            SourceRejectionReason = "unknown_source"
	SourceRejectionAssignmentMismatch SourceRejectionReason = "assignment_mismatch"
)

// SourceValidation is the pure result of applying metric source policy.
// Callers that own ingestion accounting may record a rejection; topology
// derivation deliberately only consumes the decision to avoid double counts.
type SourceValidation struct {
	Source string
	Reason SourceRejectionReason
}

// Accepted reports whether the datapoint satisfies the source policy.
func (v SourceValidation) Accepted() bool { return v.Reason == "" }

// MetricSourcePolicy validates registered provenance and optional catalog
// assignment while maintaining concurrent-safe rejection counters.
type MetricSourcePolicy struct {
	registered map[string]struct{}
	assignment metrics.Assignment
	missing    atomic.Uint64
	unknown    atomic.Uint64
	mismatch   atomic.Uint64
}

// NewMetricSourcePolicy constructs a policy from the compiled telemetry source IDs.
func NewMetricSourcePolicy(registry []string, assignment metrics.Assignment) *MetricSourcePolicy {
	registered := make(map[string]struct{}, len(registry))
	for _, sourceID := range registry {
		if sourceID != "" {
			registered[sourceID] = struct{}{}
		}
	}
	assignmentCopy := make(metrics.Assignment, len(assignment))
	for metricName, sourceID := range assignment {
		assignmentCopy[metricName] = sourceID
	}
	return &MetricSourcePolicy{registered: registered, assignment: assignmentCopy}
}

// Validate checks a recognized canonical datapoint without changing counters.
func (p *MetricSourcePolicy) Validate(canonicalName string, attrs []*commonpb.KeyValue) SourceValidation {
	if p == nil {
		return SourceValidation{Reason: SourceRejectionMissing}
	}
	source := firstKVString(attrs, semconv.AttrOKSGSource, semconv.AttrOKSGSourceProm)
	if source == "" {
		return SourceValidation{Reason: SourceRejectionMissing}
	}
	if _, ok := p.registered[source]; !ok {
		return SourceValidation{Source: source, Reason: SourceRejectionUnknown}
	}
	expected := p.expectedSource(canonicalName)
	if expected.ambiguous || (expected.assigned && expected.source != source) {
		return SourceValidation{Source: source, Reason: SourceRejectionAssignmentMismatch}
	}
	return SourceValidation{Source: source}
}

type sourceExpectation struct {
	source    string
	assigned  bool
	ambiguous bool
}

// expectedSource first uses an exact catalog assignment. Some OTLP metrics are
// specialized by a source dialect after provenance validation (for example,
// system.disk.io becomes .read/.write). For those base names, enforce the
// winner when every assigned specialization agrees on one source and mark
// conflicting descendant assignments ambiguous so validation fails closed.
func (p *MetricSourcePolicy) expectedSource(canonicalName string) sourceExpectation {
	if exact := p.assignment[canonicalName]; exact != "" {
		return sourceExpectation{source: exact, assigned: true}
	}
	prefix := canonicalName + "."
	var inferred string
	for metricName, sourceID := range p.assignment {
		if sourceID == "" || !strings.HasPrefix(metricName, prefix) {
			continue
		}
		if inferred == "" {
			inferred = sourceID
			continue
		}
		if inferred != sourceID {
			return sourceExpectation{assigned: true, ambiguous: true}
		}
	}
	return sourceExpectation{source: inferred, assigned: inferred != ""}
}

// RecordRejection increments the counter corresponding to a rejected decision.
func (p *MetricSourcePolicy) RecordRejection(validation SourceValidation) {
	if p == nil {
		return
	}
	switch validation.Reason {
	case SourceRejectionMissing:
		p.missing.Add(1)
	case SourceRejectionUnknown:
		p.unknown.Add(1)
	case SourceRejectionAssignmentMismatch:
		p.mismatch.Add(1)
	}
}

// DroppedSampleCount returns all recorded provenance rejections.
func (p *MetricSourcePolicy) DroppedSampleCount() uint64 {
	if p == nil {
		return 0
	}
	return p.missing.Load() + p.unknown.Load() + p.mismatch.Load()
}

// DroppedSampleCountForReason returns the recorded count for one reason.
func (p *MetricSourcePolicy) DroppedSampleCountForReason(reason SourceRejectionReason) uint64 {
	if p == nil {
		return 0
	}
	switch reason {
	case SourceRejectionMissing:
		return p.missing.Load()
	case SourceRejectionUnknown:
		return p.unknown.Load()
	case SourceRejectionAssignmentMismatch:
		return p.mismatch.Load()
	default:
		return 0
	}
}
