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
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/topology"
)

var integrationDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:16.6-alpine",
		postgres.WithDatabase("oksg_test"),
		postgres.WithUsername("oksg_test"),
		postgres.WithPassword("oksg_test"),
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
	return fmt.Sprintf("postgres://oksg_test:oksg_test@%s:%s/oksg_test?sslmode=disable", host, port.Port())
}

func TestPostgresStructuralSnapshotsRetainHistoryAndComposeObservations(t *testing.T) {
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
		Nodes: []topology.Node{{ID: ingress, Kind: entities.KindIngress}, {ID: service, Kind: entities.KindService}}, Edges: []topology.Edge{route},
	}}))
	require.NoError(t, store.PutObservedSnapshot(ctx, topology.Snapshot{CreatedAt: now, Graph: topology.Graph{Edges: []topology.Edge{calls}}}))
	currentRoutes, err := store.QueryEdges(ctx, query.EdgeQuery{Relation: topology.RelRoutesTo})
	require.NoError(t, err)
	require.Len(t, currentRoutes, 1)
	currentRoute := currentRoutes[0]
	require.Equal(t, route.ID, currentRoute.ID)
	require.Equal(t, route.Source, currentRoute.Source)
	require.Equal(t, route.Target, currentRoute.Target)
	require.Equal(t, route.Relation, currentRoute.Relation)
	require.Equal(t, route.Confidence, currentRoute.Confidence)
	require.True(t, route.FirstSeen.Equal(currentRoute.FirstSeen))
	require.True(t, route.LastSeen.Equal(currentRoute.LastSeen))
	require.Empty(t, currentRoute.Provenance)
	require.Empty(t, currentRoute.Metadata)
	require.NoError(t, store.PutStructuralSnapshot(ctx, topology.Snapshot{CreatedAt: now.Add(time.Minute), Graph: topology.Graph{
		Nodes: []topology.Node{{ID: service, Kind: entities.KindService}},
	}}))

	latest, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Len(t, latest.Graph.Nodes, 1)
	require.Equal(t, service, latest.Graph.Nodes[0].ID)
	require.Len(t, latest.Graph.Edges, 1)
	require.Equal(t, topology.RelCalls, latest.Graph.Edges[0].Relation)
	var historicalEdges int
	require.NoError(t, conn.pool.QueryRow(ctx, "SELECT count(*) FROM topology_snapshot_edges WHERE id = $1", route.ID).Scan(&historicalEdges))
	require.Equal(t, 1, historicalEdges)
}

func resetTopology(ctx context.Context, conn *Conn) error {
	_, err := conn.pool.Exec(ctx, "TRUNCATE topology_snapshot_edges, topology_nodes, topology_edges, topology_snapshots RESTART IDENTITY")
	return err
}
