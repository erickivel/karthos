package topology

import (
	"time"

	"github.com/erickivel/karthos/pkg/core/schema"
)

type Snapshot struct {
	// ID identifies one immutable, materialized graph projection. It is zero
	// for transient structural and observation writes before materialization.
	ID        int64                `json:"id,omitempty"`
	Version   schema.SchemaVersion `json:"version"`
	Graph     Graph                `json:"graph"`
	From      time.Time            `json:"from"`
	To        time.Time            `json:"to"`
	CreatedAt time.Time            `json:"created_at"`
}
