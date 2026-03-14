package topology

import (
	"time"

	"github.com/erickivel/oksg/pkg/core/schema"
)

type Snapshot struct {
	Version   schema.SchemaVersion `json:"version"`
	Graph     Graph                `json:"graph"`
	From      time.Time            `json:"from"`
	To        time.Time            `json:"to"`
	CreatedAt time.Time            `json:"created_at"`
}
