//go:build integration

package postgresql

import (
	"context"
	"testing"
	"time"

	"github.com/erickivel/karthos/pkg/backend/ingest"
	"github.com/stretchr/testify/require"
)

func TestEnvelopeRegistryHeartbeatPreservesFirstSeenAndRefreshesPerEnvelope(t *testing.T) {
	ctx := context.Background()
	conn, err := NewConn(ctx, integrationDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	require.NoError(t, Migrate(ctx, conn))
	resolver := ingest.NewOTLPEntityResolverWithRegistry("heartbeat-integration", NewPostgresEntityRegistry(conn))
	resource := map[string]string{"k8s.namespace.name": "heartbeat", "k8s.pod.name": "same-name", "k8s.pod.uid": "A"}
	serverTime := func() time.Time {
		var now time.Time
		require.NoError(t, conn.pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now))
		return now
	}
	type heartbeat struct {
		first, last time.Time
		uid         string
	}
	var id string
	read := func() heartbeat {
		var state heartbeat
		require.NoError(t, conn.pool.QueryRow(ctx, `SELECT first_seen,last_seen,labels->>'k8s.pod.uid' FROM entities WHERE id=$1`, id).Scan(&state.first, &state.last, &state.uid))
		return state
	}
	scope := resolver.ForEnvelope()
	before := serverTime()
	resolved, err := scope.Resolve(ctx, resource)
	require.NoError(t, err)
	id = string(resolved)
	after := serverTime()
	first := read()
	require.False(t, first.first.Before(before))
	require.False(t, first.last.After(after))
	require.Equal(t, first.first, first.last)
	_, err = scope.Resolve(ctx, resource)
	require.NoError(t, err)
	require.Equal(t, first, read(), "unchanged metadata in one envelope must not create another heartbeat")
	before = serverTime()
	_, err = resolver.ForEnvelope().Resolve(ctx, resource)
	require.NoError(t, err)
	after = serverTime()
	second := read()
	require.Equal(t, first.first, second.first)
	require.False(t, second.last.Before(before))
	require.False(t, second.last.After(after))
	before = serverTime()
	resource["k8s.pod.uid"] = "B"
	// Same canonical name/ID, new lifecycle metadata: persist even in the old scope.
	replacement, err := scope.Resolve(ctx, resource)
	require.NoError(t, err)
	require.Equal(t, id, string(replacement))
	after = serverTime()
	third := read()
	require.Equal(t, first.first, third.first)
	require.Equal(t, "B", third.uid)
	require.False(t, third.last.Before(before))
	require.False(t, third.last.After(after))
}
