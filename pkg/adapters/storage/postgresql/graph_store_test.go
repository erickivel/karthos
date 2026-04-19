package postgresql

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/erickivel/oksg/pkg/core/topology"
)

func TestApplySnapshotTimestampSetsGraphTimestampFromCreatedAt(t *testing.T) {
	createdAt := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	snap := topology.Snapshot{
		CreatedAt: createdAt,
	}

	applySnapshotTimestamp(&snap)

	assert.Equal(t, createdAt, snap.Graph.Timestamp)
}
