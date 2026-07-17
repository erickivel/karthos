package query

import (
	"context"
	"time"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/topology"
)

type MetricQuery struct {
	EntityIDs    []entities.EntityID
	MetricName   string
	From         time.Time
	To           time.Time
	EdgeSourceID entities.EntityID
	EdgeTargetID entities.EntityID
	Aggregation  metrics.AggregationMode
	Window       time.Duration
	Unit         metrics.Unit
}

var servicegraphRequestContextLabels = []string{
	"entity_id",
	"source",
	"edge_client",
	"edge_client_id",
	"edge_server",
	"edge_server_id",
	"connection_type",
	"virtual_node",
	"client_http_method",
	"server_http_method",
}

// ServicegraphRequestContextLabels returns the canonical label identity used
// when request and error branches are combined into an error ratio.
func ServicegraphRequestContextLabels() []string {
	return append([]string(nil), servicegraphRequestContextLabels...)
}

type EdgeQuery struct {
	Source entities.EntityID
	Target entities.EntityID
}

type GraphQuerier interface {
	GetLatestGraph(ctx context.Context) (*topology.Snapshot, error)
	QueryEdges(ctx context.Context, query EdgeQuery) ([]topology.Edge, error)
}

type MetricQuerier interface {
	QueryMetrics(ctx context.Context, query MetricQuery) ([]metrics.MetricSample, error)
}
