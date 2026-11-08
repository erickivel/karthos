//go:build integration

package postgresql

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/topology"
)

var integrationDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:16.6-alpine",
		postgres.WithDatabase("karthos_test"),
		postgres.WithUsername("karthos_test"),
		postgres.WithPassword("karthos_test"),
		testcontainers.WithAdditionalWaitStrategy(wait.ForSQL("5432/tcp", "pgx", postgresTestURL)),
	)
	if err != nil {
		panic(err)
	}
	integrationDSN, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		panic(err)
	}
	code := m.Run()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

func postgresTestURL(host string, port network.Port) string {
	return fmt.Sprintf("postgres://karthos_test:karthos_test@%s:%s/karthos_test?sslmode=disable", host, port.Port())
}

func TestPostgresMaterializedSnapshotsRetainStructureAndFreshObservations(t *testing.T) {
	ctx := context.Background()
	conn, err := NewConn(ctx, integrationDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	require.NoError(t, Migrate(ctx, conn))
	require.NoError(t, resetTopology(ctx, conn))
	store := NewPostgresGraphStore(conn)
	now := time.Now().UTC()
	ingress := entities.ComputeEntityID(entities.KindIngress, "cluster", "demo", "public")
	service := entities.ComputeEntityID(entities.KindService, "cluster", "demo", "api")
	database := entities.ComputeEntityID(entities.KindService, "cluster", "demo", "database")
	route := topology.NewEdge(ingress, service, topology.RelRoutesTo, now)
	calls := topology.NewEdge(service, database, topology.RelCalls, now)
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{CreatedAt: now, Graph: topology.Graph{
		Nodes: []topology.Node{{ID: ingress, Kind: entities.KindIngress}, {ID: service, Kind: entities.KindService}, {ID: database, Kind: entities.KindService}}, Edges: []topology.Edge{route},
	}}))
	require.NoError(t, store.PutObservedEdges(ctx, []topology.Edge{calls}))
	first, err := store.Materialize(ctx, now)
	require.NoError(t, err)
	require.Len(t, first.Graph.Edges, 2)
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{CreatedAt: now.Add(time.Minute), Graph: topology.Graph{
		Nodes: []topology.Node{{ID: service, Kind: entities.KindService}},
	}}))
	second, err := store.Materialize(ctx, now.Add(time.Minute))
	require.NoError(t, err)

	latest, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Equal(t, second.ID, latest.ID)
	require.Len(t, latest.Graph.Nodes, 1)
	require.Equal(t, service, latest.Graph.Nodes[0].ID)
	require.Empty(t, latest.Graph.Edges)

	historical, err := store.GetAt(ctx, now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, first.ID, historical.ID)
	require.Len(t, historical.Graph.Edges, 2)
}

func TestPostgresObservationWindowAndNodeInterval(t *testing.T) {
	ctx := context.Background()
	conn, err := NewConn(ctx, integrationDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	require.NoError(t, Migrate(ctx, conn))
	require.NoError(t, resetTopology(ctx, conn))
	store := NewPostgresGraphStore(conn)
	first := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Second)
	second := first.Add(6 * time.Minute)
	service := entities.ComputeEntityID(entities.KindService, "cluster", "demo", "api")
	peer := entities.ComputeEntityID(entities.KindService, "cluster", "demo", "db")
	nodes := []topology.Node{{ID: service, Kind: entities.KindService, Provenance: []string{"kubernetes.structure"}, FirstSeen: first, LastSeen: first}, {ID: peer, Kind: entities.KindService, Provenance: []string{"kubernetes.structure"}, FirstSeen: first, LastSeen: first}}
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{CreatedAt: first, Graph: topology.Graph{Nodes: nodes}}))
	edge := topology.NewEdge(service, peer, topology.RelCalls, first)
	edge.Provenance = []string{"mesh.istio"}
	require.NoError(t, store.PutObservedEdges(ctx, []topology.Edge{edge}))
	_, err = store.Materialize(ctx, first)
	require.NoError(t, err)
	for i := range nodes {
		nodes[i].FirstSeen, nodes[i].LastSeen = second, second
	}
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{CreatedAt: second, Graph: topology.Graph{Nodes: nodes}}))
	latest, err := store.Materialize(ctx, second)
	require.NoError(t, err)
	require.True(t, first.Equal(latest.From))
	require.True(t, second.Equal(latest.To))
	require.True(t, first.Equal(latest.Graph.Nodes[0].FirstSeen))
	observed, err := store.ObservedEdgesInWindow(ctx, first.Add(-time.Second), first.Add(time.Second), "mesh.istio")
	require.NoError(t, err)
	require.Len(t, observed, 1)
	require.Equal(t, []string{"mesh.istio"}, observed[0].Provenance)
}

func TestPostgresGraphStoreNodeTTLExpiresEntireStructuralProjection(t *testing.T) {
	ctx := context.Background()
	conn, err := NewConn(ctx, integrationDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	require.NoError(t, Migrate(ctx, conn))
	require.NoError(t, resetTopology(ctx, conn))
	store := NewPostgresGraphStore(conn, WithPostgresNodeTTL(time.Hour))
	service := entities.ComputeEntityID(entities.KindService, "cluster", "demo", "api")
	workload := entities.ComputeEntityID(entities.KindWorkload, "cluster", "demo", "api")
	edge := topology.NewEdge(workload, service, topology.RelContains, time.Now().UTC())
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{CreatedAt: time.Now().UTC().Add(-2 * time.Hour), Graph: topology.Graph{
		Nodes: []topology.Node{{ID: service, Kind: entities.KindService}, {ID: workload, Kind: entities.KindWorkload}},
		Edges: []topology.Edge{edge},
	}}))
	materialized, err := store.Materialize(ctx, time.Now().UTC())
	require.NoError(t, err)
	assert.Nil(t, materialized)
	latest, err := store.GetLatest(ctx)
	require.NoError(t, err)
	assert.Nil(t, latest)
	var historyCount int
	require.NoError(t, conn.pool.QueryRow(ctx, `SELECT count(*) FROM topology_snapshots WHERE scope = 'structural'`).Scan(&historyCount))
	assert.Equal(t, 1, historyCount)
}

func TestPostgresObservedEdgeMergeUsesNewestPayloadAndRetainsProvenance(t *testing.T) {
	ctx := context.Background()
	conn, err := NewConn(ctx, integrationDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	require.NoError(t, Migrate(ctx, conn))
	require.NoError(t, resetTopology(ctx, conn))
	store := NewPostgresGraphStore(conn)
	// PostgreSQL TIMESTAMPTZ retains microsecond precision, so use a timestamp
	// that survives the persistence round trip exactly.
	base := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Minute)
	first := topology.NewEdge(
		entities.ComputeEntityID(entities.KindService, "cluster", "demo", "a"),
		entities.ComputeEntityID(entities.KindService, "cluster", "demo", "b"),
		topology.RelCalls, base,
	)
	first.LastSeen = base.Add(10 * time.Second)
	first.Confidence = 0.4
	first.Metadata = map[string]string{"protocol": "http"}
	first.Provenance = []string{"traces.servicegraph"}
	second := topology.NewEdge(first.Source, first.Target, first.Relation, base.Add(5*time.Second))
	second.LastSeen = base.Add(20 * time.Second)
	second.Confidence = 0.9
	second.Metadata = map[string]string{"protocol": "grpc"}
	second.Provenance = []string{"mesh.istio"}
	require.NoError(t, store.PutObservedEdges(ctx, []topology.Edge{first}))
	require.NoError(t, store.PutObservedEdges(ctx, []topology.Edge{second}))
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{CreatedAt: time.Now().UTC(), Graph: topology.Graph{
		Nodes: []topology.Node{{ID: first.Source, Kind: entities.KindService}, {ID: first.Target, Kind: entities.KindService}},
	}}))

	latest, err := store.Materialize(ctx, time.Now().UTC())

	require.NoError(t, err)
	require.Len(t, latest.Graph.Edges, 1)
	merged := latest.Graph.Edges[0]
	assert.True(t, first.FirstSeen.Equal(merged.FirstSeen))
	assert.True(t, second.LastSeen.Equal(merged.LastSeen))
	assert.Equal(t, second.Confidence, merged.Confidence)
	assert.Equal(t, second.Metadata, merged.Metadata)
	assert.ElementsMatch(t, []string{"traces.servicegraph", "mesh.istio"}, merged.Provenance)
}

func TestPostgresObservedEdgeMergeDoesNotLetDelayedPayloadOverwriteNewerState(t *testing.T) {
	ctx := context.Background()
	conn, err := NewConn(ctx, integrationDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	require.NoError(t, Migrate(ctx, conn))
	require.NoError(t, resetTopology(ctx, conn))
	store := NewPostgresGraphStore(conn)
	// PostgreSQL TIMESTAMPTZ retains microsecond precision, so use a timestamp
	// that survives the persistence round trip exactly.
	base := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Minute)
	newer := topology.NewEdge(
		entities.ComputeEntityID(entities.KindService, "cluster", "demo", "a"),
		entities.ComputeEntityID(entities.KindService, "cluster", "demo", "b"),
		topology.RelCalls, base,
	)
	newer.LastSeen = base.Add(20 * time.Second)
	newer.Confidence = 0.9
	newer.Metadata = map[string]string{"protocol": "grpc", "mesh.security.policy": "mutual_tls"}
	newer.Provenance = []string{"mesh.istio"}
	delayed := topology.NewEdge(newer.Source, newer.Target, newer.Relation, base.Add(-5*time.Second))
	delayed.LastSeen = base.Add(10 * time.Second)
	delayed.Confidence = 0.4
	delayed.Metadata = map[string]string{"protocol": "http"}
	delayed.Provenance = []string{"traces.servicegraph"}

	require.NoError(t, store.PutObservedEdges(ctx, []topology.Edge{newer}))
	require.NoError(t, store.PutObservedEdges(ctx, []topology.Edge{delayed}))
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{CreatedAt: time.Now().UTC(), Graph: topology.Graph{
		Nodes: []topology.Node{{ID: newer.Source, Kind: entities.KindService}, {ID: newer.Target, Kind: entities.KindService}},
	}}))

	latest, err := store.Materialize(ctx, time.Now().UTC())

	require.NoError(t, err)
	require.Len(t, latest.Graph.Edges, 1)
	merged := latest.Graph.Edges[0]
	assert.True(t, delayed.FirstSeen.Equal(merged.FirstSeen))
	assert.True(t, newer.LastSeen.Equal(merged.LastSeen))
	assert.Equal(t, newer.Confidence, merged.Confidence)
	assert.Equal(t, newer.Metadata, merged.Metadata)
	assert.Equal(t, []string{"mesh.istio", "traces.servicegraph"}, merged.Provenance)
}

func resetTopology(ctx context.Context, conn *Conn) error {
	_, err := conn.pool.Exec(ctx, "TRUNCATE topology_edge_observations, topology_snapshot_edges, topology_nodes, topology_edges, topology_snapshots RESTART IDENTITY")
	return err
}
