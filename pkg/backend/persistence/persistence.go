package persistence

import (
	"context"
	"time"

	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/topology"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

type MetricStore interface {
	Write(ctx context.Context, samples []metrics.MetricSample) error
	Query(ctx context.Context, q query.MetricQuery) ([]metrics.MetricSample, error)
	// QueryCatalogStats returns per-(canonical metric, source) sample counts and
	// last-seen timestamps over the given lookback window. The source dimension
	// is read from the `source` label on samples (semconv.LabelSource).
	// Metrics not present in the store during the window are omitted; the API
	// layer is responsible for zero-filling against the catalog.
	QueryCatalogStats(ctx context.Context, lookback time.Duration) ([]CatalogStat, error)
}

// CatalogStat aggregates per-source sample stats for a single canonical metric.
type CatalogStat struct {
	MetricName string
	BySource   []SourceStat
}

// SourceStat is the per-source count and last-seen for one canonical metric.
// A SourceID of "" represents samples that arrived without a source label
// (legacy / partially-instrumented producers).
type SourceStat struct {
	SourceID    string
	SampleCount int64
	LastSeenAt  time.Time
}

type GraphStore interface {
	PutSnapshot(ctx context.Context, snapshot topology.Snapshot) error
	GetLatest(ctx context.Context) (*topology.Snapshot, error)
	QueryEdges(ctx context.Context, q query.EdgeQuery) ([]topology.Edge, error)
}

// RunsOnRetirer is the narrow lifecycle-to-storage boundary for retiring a
// terminal Pod assignment without inferring retirement from missing telemetry.
// Retired edges remain available through QueryEdges as audit history.
type RunsOnRetirer interface {
	RetireRunsOn(ctx context.Context, podID, nodeID entities.EntityID, podUID string, retiredAt time.Time) error
}

// RunsOnAssignment is the observed identity of an active Pod-to-Node assignment.
type RunsOnAssignment struct {
	PodID  entities.EntityID
	NodeID entities.EntityID
	PodUID string
}

// RunsOnTerminalGrace is a persisted terminal-Pod grace window. The Kubernetes
// identity fields allow a restarted lifecycle tracker to re-arm retirement at
// the original deadline without inferring the old assignment from telemetry.
type RunsOnTerminalGrace struct {
	ClusterName string
	Namespace   string
	PodName     string
	PodUID      string
	NodeName    string
	PodID       entities.EntityID
	NodeID      entities.EntityID
	ObservedAt  time.Time
	Deadline    time.Time
}

// RunsOnTerminalMarker durably records a terminal Pod's grace deadline before
// retirement. The marker keeps the assignment latest-active until Deadline.
type RunsOnTerminalMarker interface {
	MarkRunsOnTerminal(ctx context.Context, grace RunsOnTerminalGrace) error
}

// RunsOnReconciler retires persisted active runs_on assignments that no longer
// match the initial authoritative Kubernetes List/Watch state for one cluster.
// Unexpired terminal grace windows are returned so the tracker can re-arm them.
type RunsOnReconciler interface {
	ReconcileRunsOn(ctx context.Context, clusterName string, active []RunsOnAssignment, retiredAt time.Time) ([]RunsOnTerminalGrace, error)
}

type MetadataStore interface {
	PutDiscovery(ctx context.Context, snapshot profile.ClusterProfile) error
	GetDiscovery(ctx context.Context) (*profile.ClusterProfile, error)
}
