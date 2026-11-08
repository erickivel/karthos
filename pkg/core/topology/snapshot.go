package topology

import (
	"time"

	"github.com/erickivel/karthos/pkg/core/schema"
)

// HistoryRetention is the public graph history window for both stores.
const HistoryRetention = 7 * 24 * time.Hour

type Snapshot struct {
	// ID identifies one immutable, materialized graph projection. It is zero
	// for transient structural and observation writes before materialization.
	ID         int64                `json:"id,omitempty"`
	Version    schema.SchemaVersion `json:"version"`
	Graph      Graph                `json:"graph"`
	From       time.Time            `json:"from"`
	To         time.Time            `json:"to"`
	CreatedAt  time.Time            `json:"created_at"`
	ObservedAt time.Time            `json:"observed_at"`
}

// SetObservationInterval records the span of evidence represented by a graph.
// CreatedAt remains the instant when this projection was materialized.
func SetObservationInterval(snapshot *Snapshot) {
	var from, to time.Time
	add := func(first, last time.Time) {
		if !first.IsZero() && (from.IsZero() || first.Before(from)) {
			from = first
		}
		if !last.IsZero() && (to.IsZero() || last.After(to)) {
			to = last
		}
	}
	for _, node := range snapshot.Graph.Nodes {
		add(node.FirstSeen, node.LastSeen)
	}
	for _, edge := range snapshot.Graph.Edges {
		add(edge.FirstSeen, edge.LastSeen)
	}
	if from.IsZero() {
		from = snapshot.CreatedAt
	}
	if to.IsZero() {
		to = snapshot.CreatedAt
	}
	snapshot.From, snapshot.To, snapshot.ObservedAt = from, to, snapshot.CreatedAt
}
