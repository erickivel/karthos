package postgresql

import (
	"testing"
	"time"

	"github.com/erickivel/oksg/pkg/core/topology"
	"github.com/stretchr/testify/assert"
)

func TestApplySnapshotTimestampSetsGraphTimestampFromCreatedAt(t *testing.T) {
	createdAt := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	snap := topology.Snapshot{
		CreatedAt: createdAt,
	}

	applySnapshotTimestamp(&snap)

	assert.Equal(t, createdAt, snap.Graph.Timestamp)
}

func TestBuildLatestNodeQueryUsesSubSecondTTLPrecision(t *testing.T) {
	t.Parallel()

	sql, args := latestNodeQuery(250 * time.Millisecond)

	assert.Contains(t, sql, "WHERE ts.created_at > NOW() - $1::interval")
	assert.Equal(t, []any{"250ms"}, args)
}

func TestRetiredRunsOnSQLKeepsAuditEdgeOutOfLatestAndBlocksStaleRefreshes(t *testing.T) {
	assert.Contains(t, latestActiveEdgeQuery, "relation <> 'runs_on' OR NOT (metadata ? 'retired_at')")
	assert.Contains(t, edgeUpsertSQL, "topology_edges.relation = 'runs_on' AND topology_edges.metadata ? 'retired_at'")
	assert.Contains(t, retireRunsOnSQL, "jsonb_set")
	assert.Contains(t, retireRunsOnSQL, "relation = 'runs_on'")
}
