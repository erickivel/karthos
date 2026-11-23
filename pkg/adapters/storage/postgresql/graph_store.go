package postgresql

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/erickivel/karthos/pkg/backend/persistence"
	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/topology"
)

const edgeUpsertSQL = `INSERT INTO topology_edges (id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata)
	 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	 ON CONFLICT (id) DO UPDATE SET
	   first_seen = LEAST(topology_edges.first_seen, EXCLUDED.first_seen),
	   last_seen = CASE WHEN topology_edges.relation = 'runs_on' AND topology_edges.metadata ? 'retired_at' THEN LEAST(topology_edges.last_seen, EXCLUDED.last_seen) ELSE GREATEST(topology_edges.last_seen, EXCLUDED.last_seen) END,
	   confidence = CASE WHEN EXCLUDED.last_seen >= topology_edges.last_seen THEN EXCLUDED.confidence ELSE topology_edges.confidence END,
	   provenance = ARRAY(SELECT DISTINCT source FROM unnest(topology_edges.provenance || EXCLUDED.provenance) AS source ORDER BY source),
	   metadata = CASE
	     WHEN topology_edges.relation = 'runs_on' AND (topology_edges.metadata ? 'retired_at' OR topology_edges.metadata ? 'terminal_deadline') THEN topology_edges.metadata || EXCLUDED.metadata
	     WHEN EXCLUDED.last_seen >= topology_edges.last_seen THEN EXCLUDED.metadata
	     ELSE topology_edges.metadata
	   END`

const (
	graphSnapshotRetention          = topology.HistoryRetention
	graphObservationFreshnessWindow = 5 * time.Minute
)

const retireRunsOnSQL = `INSERT INTO topology_edges (id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata)
	 VALUES ($1, $2, $3, 'runs_on', 1.0, '{}', $4, $4, jsonb_build_object('retired_at', $5::text, 'k8s.pod.uid', $6::text))
	 ON CONFLICT (id) DO UPDATE SET
	   last_seen = LEAST(topology_edges.last_seen, EXCLUDED.last_seen),
	   metadata = jsonb_set(topology_edges.metadata, '{retired_at}', to_jsonb($5::text), true)
	 WHERE topology_edges.relation = 'runs_on' AND NOT (topology_edges.metadata ? 'retired_at')`

const markRunsOnTerminalSQL = `INSERT INTO topology_edges (id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata)
	 VALUES ($1, $2, $3, 'runs_on', 1.0, '{}', $4, $4, $5)
	 ON CONFLICT (id) DO UPDATE SET
	   first_seen = LEAST(topology_edges.first_seen, EXCLUDED.first_seen),
	   last_seen = GREATEST(topology_edges.last_seen, EXCLUDED.last_seen),
	   metadata = CASE
	     WHEN topology_edges.metadata ? 'terminal_deadline'
	       THEN topology_edges.metadata || (EXCLUDED.metadata - 'terminal_deadline')
	     ELSE topology_edges.metadata || EXCLUDED.metadata
	   END
	 WHERE topology_edges.relation = 'runs_on' AND NOT (topology_edges.metadata ? 'retired_at')`

const reconcileActiveRunsOnSQL = `SELECT id, source, target, metadata
	 FROM topology_edges
	 WHERE relation = 'runs_on'
	   AND NOT (metadata ? 'retired_at')
	   AND metadata->>'k8s.cluster.name' = $1`

// PostgresGraphStore implements persistence.GraphStore using PostgreSQL.
type PostgresGraphStore struct {
	conn    *Conn
	nodeTTL time.Duration // 0 = no structural freshness guard
}

// PostgresGraphStoreOption configures a PostgresGraphStore.
type PostgresGraphStoreOption func(*PostgresGraphStore)

// WithPostgresNodeTTL refuses to materialize from an expired structural
// projection. Retained materialized snapshots remain queryable. A zero ttl
// disables the structural freshness guard.
func WithPostgresNodeTTL(ttl time.Duration) PostgresGraphStoreOption {
	return func(s *PostgresGraphStore) { s.nodeTTL = ttl }
}

// NewPostgresGraphStore creates a new PostgresGraphStore with the given connection.
func NewPostgresGraphStore(conn *Conn, opts ...PostgresGraphStoreOption) *PostgresGraphStore {
	s := &PostgresGraphStore{conn: conn}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// PutStructuralSnapshot retains one complete Kubernetes projection. Current
// graph reads select only the newest projection; earlier snapshots remain
// available as structural history.
func (s *PostgresGraphStore) PutStructuralSnapshot(ctx context.Context, snapshot topology.Snapshot) error {
	_, err := s.putSnapshot(ctx, snapshot, "structural")
	return err
}

// PutObservedEdges stores current telemetry-derived and lifecycle edges.
// Observations do not create graph snapshots: only Materialize persists a
// complete historical graph projection.
func (s *PostgresGraphStore) PutObservedEdges(ctx context.Context, edges []topology.Edge) error {
	tx, err := s.conn.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pg graph begin observation write: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, edge := range edges {
		metadataJSON, err := json.Marshal(edge.Metadata)
		if err != nil {
			return fmt.Errorf("pg graph marshal observed edge metadata: %w", err)
		}
		if _, err := tx.Exec(ctx, edgeUpsertSQL,
			edge.ID, string(edge.Source), string(edge.Target), string(edge.Relation),
			edge.Confidence, postgresProvenance(edge.Provenance), edge.FirstSeen, edge.LastSeen, metadataJSON,
		); err != nil {
			return fmt.Errorf("pg graph upsert observed edge: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO topology_edge_observations (edge_id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			edge.ID, string(edge.Source), string(edge.Target), string(edge.Relation), edge.Confidence, postgresProvenance(edge.Provenance), edge.FirstSeen, edge.LastSeen, metadataJSON); err != nil {
			return fmt.Errorf("pg graph insert edge observation: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pg graph commit observation write: %w", err)
	}
	return nil
}

func (s *PostgresGraphStore) ObservedEdgesInWindow(ctx context.Context, from, to time.Time, sourceID string) ([]topology.Edge, error) {
	statement := `SELECT edge_id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata FROM topology_edge_observations WHERE first_seen <= $1 AND last_seen >= $2`
	args := []any{to, from}
	if sourceID != "" {
		statement += ` AND $3 = ANY(provenance)`
		args = append(args, sourceID)
	}
	statement += ` ORDER BY first_seen, id`
	rows, err := s.conn.pool.Query(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("pg graph query edge observations: %w", err)
	}
	defer rows.Close()
	var result []topology.Edge
	for rows.Next() {
		var edge topology.Edge
		if err := rows.Scan(&edge.ID, &edge.Source, &edge.Target, &edge.Relation, &edge.Confidence, &edge.Provenance, &edge.FirstSeen, &edge.LastSeen, &edge.Metadata); err != nil {
			return nil, fmt.Errorf("pg graph scan edge observation: %w", err)
		}
		if sourceID != "" {
			edge.Provenance = []string{sourceID}
		}
		result = append(result, edge)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg graph iterate edge observations: %w", err)
	}
	return result, nil
}

func (s *PostgresGraphStore) putSnapshot(ctx context.Context, snapshot topology.Snapshot, scope string) (int64, error) {
	tx, err := s.conn.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("pg graph put snapshot begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Insert snapshot row.
	var snapshotID int64
	createdAt := snapshot.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO topology_snapshots (scope, version_major, version_minor, from_ts, to_ts, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		scope, snapshot.Version.Major, snapshot.Version.Minor, snapshot.From, snapshot.To, createdAt,
	).Scan(&snapshotID)
	if err != nil {
		return 0, fmt.Errorf("pg graph insert snapshot: %w", err)
	}

	// Batch insert nodes.
	for _, n := range snapshot.Graph.Nodes {
		labelsJSON, err := json.Marshal(n.Labels)
		if err != nil {
			return 0, fmt.Errorf("pg graph marshal node labels: %w", err)
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO topology_nodes (snapshot_id, entity_id, kind, labels, provenance, first_seen, last_seen)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (snapshot_id, entity_id) DO NOTHING`,
			snapshotID, string(n.ID), string(n.Kind), labelsJSON, postgresProvenance(n.Provenance), nullableTime(n.FirstSeen), nullableTime(n.LastSeen),
		)
		if err != nil {
			return 0, fmt.Errorf("pg graph insert node: %w", err)
		}
	}

	// Structural and materialized edges belong to one complete projection.
	for _, e := range snapshot.Graph.Edges {
		metadataJSON, err := json.Marshal(e.Metadata)
		if err != nil {
			return 0, fmt.Errorf("pg graph marshal edge metadata: %w", err)
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO topology_snapshot_edges (snapshot_id, id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			snapshotID, e.ID, string(e.Source), string(e.Target), string(e.Relation),
			e.Confidence, postgresProvenance(e.Provenance), e.FirstSeen, e.LastSeen, metadataJSON,
		)
		if err != nil {
			return 0, fmt.Errorf("pg graph upsert edge: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("pg graph put snapshot commit: %w", err)
	}
	return snapshotID, nil
}

func nullableTime(at time.Time) any {
	if at.IsZero() {
		return nil
	}
	return at
}

func postgresProvenance(provenance []string) []string {
	if provenance == nil {
		return []string{}
	}
	return provenance
}

// Materialize stores one immutable graph snapshot from the newest Kubernetes
// structure and fresh active observed edges. Kubernetes structure is the
// authority for node membership.
func (s *PostgresGraphStore) Materialize(ctx context.Context, at time.Time) (*topology.Snapshot, error) {
	structural, structuralID, hasStructural, err := s.latestSnapshot(ctx, "structural")
	if err != nil {
		return nil, err
	}
	if !hasStructural || (s.nodeTTL > 0 && structural.CreatedAt.Before(at.Add(-s.nodeTTL))) {
		return nil, nil
	}

	snap := structural
	nodesByID := make(map[string]topology.Node)
	if err := s.loadStructuralNodes(ctx, structuralID, nodesByID); err != nil {
		return nil, err
	}
	previous, err := s.GetLatest(ctx)
	if err != nil {
		return nil, err
	}
	if previous != nil {
		for _, node := range previous.Graph.Nodes {
			current, ok := nodesByID[string(node.ID)]
			if ok && !node.FirstSeen.IsZero() && (current.FirstSeen.IsZero() || node.FirstSeen.Before(current.FirstSeen)) {
				current.FirstSeen = node.FirstSeen
				nodesByID[string(node.ID)] = current
			}
		}
	}

	edgesByID := make(map[string]topology.Edge)
	if err := s.loadEdges(ctx,
		`SELECT id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata
		 FROM topology_snapshot_edges WHERE snapshot_id = $1`,
		[]any{structuralID}, edgesByID); err != nil {
		return nil, err
	}
	if previous != nil {
		for _, prior := range previous.Graph.Edges {
			current, ok := edgesByID[prior.ID]
			if ok && prior.FirstSeen.Before(current.FirstSeen) {
				current.FirstSeen = prior.FirstSeen
				edgesByID[prior.ID] = current
			}
		}
	}
	observed := make(map[string]topology.Edge)
	if err := s.loadEdges(ctx,
		`SELECT id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata
		 FROM topology_edges
		 WHERE relation IN ('runs_on', 'calls')
		   AND last_seen BETWEEN $1 AND $2
		   AND (relation <> 'runs_on' OR (
		     NOT (metadata ? 'retired_at') AND
			 (NOT (metadata ? 'terminal_deadline') OR (metadata->>'terminal_deadline')::timestamptz > $2)
		   ))`,
		[]any{at.Add(-graphObservationFreshnessWindow), at}, observed); err != nil {
		return nil, err
	}
	for _, edge := range observed {
		if _, ok := nodesByID[string(edge.Source)]; !ok {
			continue
		}
		if _, ok := nodesByID[string(edge.Target)]; !ok {
			continue
		}
		edgesByID[edge.ID] = edge
	}

	snap.Graph.Nodes = make([]topology.Node, 0, len(nodesByID))
	for _, node := range nodesByID {
		snap.Graph.Nodes = append(snap.Graph.Nodes, node)
	}
	sort.Slice(snap.Graph.Nodes, func(i, j int) bool { return snap.Graph.Nodes[i].ID < snap.Graph.Nodes[j].ID })
	snap.Graph.Edges = make([]topology.Edge, 0, len(edgesByID))
	for _, edge := range edgesByID {
		snap.Graph.Edges = append(snap.Graph.Edges, edge)
	}
	sort.Slice(snap.Graph.Edges, func(i, j int) bool { return snap.Graph.Edges[i].ID < snap.Graph.Edges[j].ID })
	snap.CreatedAt = at.UTC()
	topology.SetObservationInterval(&snap)
	applySnapshotTimestamp(&snap)
	id, err := s.putSnapshot(ctx, snap, "materialized")
	if err != nil {
		return nil, err
	}
	snap.ID = id
	if err := s.pruneMaterializedSnapshots(ctx, at.Add(-graphSnapshotRetention)); err != nil {
		return nil, err
	}
	return &snap, nil
}

// GetLatest returns the newest complete materialized graph snapshot.
func (s *PostgresGraphStore) GetLatest(ctx context.Context) (*topology.Snapshot, error) {
	return s.getMaterialized(ctx, time.Time{})
}

// GetAt returns the newest retained materialized snapshot at or before at.
func (s *PostgresGraphStore) GetAt(ctx context.Context, at time.Time) (*topology.Snapshot, error) {
	return s.getMaterialized(ctx, at)
}

func (s *PostgresGraphStore) getMaterialized(ctx context.Context, at time.Time) (*topology.Snapshot, error) {
	snapshot, snapshotID, found, err := s.latestSnapshotAt(ctx, "materialized", at)
	if err != nil || !found {
		return nil, err
	}
	snapshot.ID = snapshotID
	snapshot.ObservedAt = snapshot.CreatedAt
	nodes := make(map[string]topology.Node)
	if err := s.loadStructuralNodes(ctx, snapshotID, nodes); err != nil {
		return nil, err
	}
	edges := make(map[string]topology.Edge)
	if err := s.loadEdges(ctx,
		`SELECT id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata
		 FROM topology_snapshot_edges WHERE snapshot_id = $1`, []any{snapshotID}, edges); err != nil {
		return nil, err
	}
	snapshot.Graph.Nodes = make([]topology.Node, 0, len(nodes))
	for _, node := range nodes {
		snapshot.Graph.Nodes = append(snapshot.Graph.Nodes, node)
	}
	sort.Slice(snapshot.Graph.Nodes, func(i, j int) bool { return snapshot.Graph.Nodes[i].ID < snapshot.Graph.Nodes[j].ID })
	snapshot.Graph.Edges = make([]topology.Edge, 0, len(edges))
	for _, edge := range edges {
		snapshot.Graph.Edges = append(snapshot.Graph.Edges, edge)
	}
	sort.Slice(snapshot.Graph.Edges, func(i, j int) bool { return snapshot.Graph.Edges[i].ID < snapshot.Graph.Edges[j].ID })
	applySnapshotTimestamp(&snapshot)
	return &snapshot, nil
}

func (s *PostgresGraphStore) pruneMaterializedSnapshots(ctx context.Context, before time.Time) error {
	tx, err := s.conn.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pg graph begin materialized snapshot prune: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `DELETE FROM topology_edge_observations WHERE last_seen < $1`, before); err != nil {
		return fmt.Errorf("pg graph delete old edge observations: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM topology_edges WHERE last_seen < $1`, before); err != nil {
		return fmt.Errorf("pg graph delete old current edges: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM topology_snapshot_edges WHERE snapshot_id IN (SELECT id FROM topology_snapshots WHERE scope = 'structural' AND created_at < $1 AND id <> (SELECT id FROM topology_snapshots WHERE scope = 'structural' ORDER BY created_at DESC, id DESC LIMIT 1))`, before); err != nil {
		return fmt.Errorf("pg graph delete old structural edges: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM topology_nodes WHERE snapshot_id IN (SELECT id FROM topology_snapshots WHERE scope = 'structural' AND created_at < $1 AND id <> (SELECT id FROM topology_snapshots WHERE scope = 'structural' ORDER BY created_at DESC, id DESC LIMIT 1))`, before); err != nil {
		return fmt.Errorf("pg graph delete old structural nodes: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM topology_snapshots WHERE scope = 'structural' AND created_at < $1 AND id <> (SELECT id FROM topology_snapshots WHERE scope = 'structural' ORDER BY created_at DESC, id DESC LIMIT 1)`, before); err != nil {
		return fmt.Errorf("pg graph delete old structural snapshots: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM topology_snapshot_edges WHERE snapshot_id IN (SELECT id FROM topology_snapshots WHERE scope = 'materialized' AND created_at < $1)`, before); err != nil {
		return fmt.Errorf("pg graph delete materialized edges: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM topology_nodes WHERE snapshot_id IN (SELECT id FROM topology_snapshots WHERE scope = 'materialized' AND created_at < $1)`, before); err != nil {
		return fmt.Errorf("pg graph delete materialized nodes: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM topology_snapshots WHERE scope = 'materialized' AND created_at < $1`, before); err != nil {
		return fmt.Errorf("pg graph delete materialized snapshots: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pg graph commit materialized snapshot prune: %w", err)
	}
	return nil
}

func (s *PostgresGraphStore) latestSnapshot(ctx context.Context, scope string) (topology.Snapshot, int64, bool, error) {
	return s.latestSnapshotAt(ctx, scope, time.Time{})
}

func (s *PostgresGraphStore) latestSnapshotAt(ctx context.Context, scope string, at time.Time) (topology.Snapshot, int64, bool, error) {
	var snapshot topology.Snapshot
	var snapshotID int64
	statement := `SELECT id, version_major, version_minor, from_ts, to_ts, created_at
	 FROM topology_snapshots WHERE scope = $1`
	args := []any{scope}
	if !at.IsZero() {
		statement += ` AND created_at <= $2`
		args = append(args, at)
	}
	statement += ` ORDER BY created_at DESC, id DESC LIMIT 1`
	err := s.conn.pool.QueryRow(ctx, statement, args...).Scan(&snapshotID, &snapshot.Version.Major, &snapshot.Version.Minor, &snapshot.From, &snapshot.To, &snapshot.CreatedAt)
	if err == pgx.ErrNoRows {
		return topology.Snapshot{}, 0, false, nil
	}
	if err != nil {
		return topology.Snapshot{}, 0, false, fmt.Errorf("pg graph get latest %s snapshot: %w", scope, err)
	}
	snapshot.ID = snapshotID
	return snapshot, snapshotID, true, nil
}

func (s *PostgresGraphStore) loadStructuralNodes(ctx context.Context, snapshotID int64, nodes map[string]topology.Node) error {
	rows, err := s.conn.pool.Query(ctx,
		`SELECT entity_id, kind, labels, provenance, first_seen, last_seen FROM topology_nodes WHERE snapshot_id = $1`, snapshotID)
	if err != nil {
		return fmt.Errorf("pg graph get structural nodes: %w", err)
	}
	defer rows.Close()
	if err := scanNodes(rows, nodes); err != nil {
		return err
	}
	return nil
}

func scanNodes(rows pgx.Rows, nodes map[string]topology.Node) error {
	for rows.Next() {
		var node topology.Node
		var id, kind string
		var labelsJSON []byte
		var firstSeen, lastSeen *time.Time
		if err := rows.Scan(&id, &kind, &labelsJSON, &node.Provenance, &firstSeen, &lastSeen); err != nil {
			return fmt.Errorf("pg graph scan node: %w", err)
		}
		if firstSeen != nil {
			node.FirstSeen = *firstSeen
		}
		if lastSeen != nil {
			node.LastSeen = *lastSeen
		}
		node.ID = entities.EntityID(id)
		node.Kind = entities.EntityKind(kind)
		if err := json.Unmarshal(labelsJSON, &node.Labels); err != nil {
			return fmt.Errorf("pg graph unmarshal node labels: %w", err)
		}
		nodes[id] = node
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("pg graph nodes iteration: %w", err)
	}
	return nil
}

func (s *PostgresGraphStore) loadEdges(ctx context.Context, query string, args []any, edges map[string]topology.Edge) error {
	rows, err := s.conn.pool.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("pg graph get edges: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var edge topology.Edge
		var source, target, relation string
		var metadataJSON []byte
		if err := rows.Scan(
			&edge.ID, &source, &target, &relation, &edge.Confidence,
			&edge.Provenance, &edge.FirstSeen, &edge.LastSeen, &metadataJSON,
		); err != nil {
			return fmt.Errorf("pg graph scan edge: %w", err)
		}
		edge.Source = entities.EntityID(source)
		edge.Target = entities.EntityID(target)
		edge.Relation = topology.RelationType(relation)
		if err := json.Unmarshal(metadataJSON, &edge.Metadata); err != nil {
			return fmt.Errorf("pg graph unmarshal edge metadata: %w", err)
		}
		edges[edge.ID] = edge
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("pg graph edges iteration: %w", err)
	}
	return nil
}

// RetireRunsOn records terminal Pod lifecycle state in edge metadata without
// changing first_seen or last_seen. Materialization excludes retired edges,
// while lifecycle reconciliation retains their state for its own decisions.
func (s *PostgresGraphStore) RetireRunsOn(ctx context.Context, podID, nodeID entities.EntityID, podUID string, retiredAt time.Time) error {
	edgeID := topology.ComputeEdgeIDWithDiscriminator(podID, nodeID, topology.RelRunsOn, podUID)
	_, err := s.conn.pool.Exec(ctx, retireRunsOnSQL,
		edgeID, string(podID), string(nodeID), retiredAt, retiredAt.UTC().Format(time.RFC3339Nano), podUID,
	)
	if err != nil {
		return fmt.Errorf("pg graph retire runs_on edge: %w", err)
	}
	return nil
}

// MarkRunsOnTerminal durably records the grace deadline for a terminal Pod UID
// so a restarted backend can keep the assignment active until that deadline.
func (s *PostgresGraphStore) MarkRunsOnTerminal(ctx context.Context, grace persistence.RunsOnTerminalGrace) (persistence.RunsOnTerminalState, error) {
	if grace.ClusterName == "" || grace.PodUID == "" || grace.Deadline.IsZero() {
		return persistence.RunsOnTerminalPending, fmt.Errorf("terminal runs_on grace requires cluster, pod UID, and deadline")
	}
	edgeID := topology.ComputeEdgeIDWithDiscriminator(grace.PodID, grace.NodeID, topology.RelRunsOn, grace.PodUID)
	markerMetadata := map[string]string{
		"k8s.cluster.name":   grace.ClusterName,
		"k8s.namespace.name": grace.Namespace,
		"k8s.pod.name":       grace.PodName,
		"k8s.pod.uid":        grace.PodUID,
		"k8s.node.name":      grace.NodeName,
		"terminal_deadline":  grace.Deadline.UTC().Format(time.RFC3339Nano),
	}
	if grace.Confirmed {
		markerMetadata["terminal_grace_confirmed"] = "true"
	}
	metadata, err := json.Marshal(markerMetadata)
	if err != nil {
		return persistence.RunsOnTerminalPending, fmt.Errorf("marshal terminal runs_on metadata: %w", err)
	}
	tag, err := s.conn.pool.Exec(ctx, markRunsOnTerminalSQL, edgeID, string(grace.PodID), string(grace.NodeID), grace.ObservedAt, metadata)
	if err != nil {
		return persistence.RunsOnTerminalPending, fmt.Errorf("pg graph mark terminal runs_on edge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return persistence.RunsOnTerminalRetired, nil
	}
	return persistence.RunsOnTerminalPending, nil
}

// ReconcileRunsOn retires persisted active UID-versioned assignments that are
// absent from the authoritative initial Kubernetes Pod List/Watch state.
func (s *PostgresGraphStore) ReconcileRunsOn(ctx context.Context, clusterName string, active []persistence.RunsOnAssignment, retiredAt time.Time) ([]persistence.RunsOnTerminalGrace, error) {
	activeIDs := make(map[string]struct{}, len(active))
	for _, assignment := range active {
		activeIDs[topology.ComputeEdgeIDWithDiscriminator(assignment.PodID, assignment.NodeID, topology.RelRunsOn, assignment.PodUID)] = struct{}{}
	}

	rows, err := s.conn.pool.Query(ctx, reconcileActiveRunsOnSQL, clusterName)
	if err != nil {
		return nil, fmt.Errorf("pg graph list active runs_on assignments: %w", err)
	}
	defer rows.Close()
	type staleAssignment struct {
		podID     entities.EntityID
		nodeID    entities.EntityID
		podUID    string
		retiredAt time.Time
	}
	stale := make([]staleAssignment, 0)
	pending := make([]persistence.RunsOnTerminalGrace, 0)
	for rows.Next() {
		var id, source, target string
		var metadataJSON []byte
		if err := rows.Scan(&id, &source, &target, &metadataJSON); err != nil {
			return nil, fmt.Errorf("pg graph scan active runs_on assignment: %w", err)
		}
		var metadata map[string]string
		if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
			return nil, fmt.Errorf("pg graph unmarshal active runs_on metadata: %w", err)
		}
		podUID := metadata["k8s.pod.uid"]
		if podUID == "" {
			continue
		}
		if deadlineText := metadata["terminal_deadline"]; deadlineText != "" {
			deadline, err := time.Parse(time.RFC3339Nano, deadlineText)
			if err != nil {
				return nil, fmt.Errorf("parse terminal runs_on deadline: %w", err)
			}
			confirmed := metadata["terminal_grace_confirmed"] == "true"
			if !confirmed {
				if _, active := activeIDs[id]; !active {
					stale = append(stale, staleAssignment{podID: entities.EntityID(source), nodeID: entities.EntityID(target), podUID: podUID})
					continue
				}
			}
			if deadline.After(retiredAt) {
				pending = append(pending, persistence.RunsOnTerminalGrace{
					ClusterName: clusterName,
					Namespace:   metadata["k8s.namespace.name"],
					PodName:     metadata["k8s.pod.name"],
					PodUID:      podUID,
					NodeName:    metadata["k8s.node.name"],
					PodID:       entities.EntityID(source),
					NodeID:      entities.EntityID(target),
					Deadline:    deadline,
					Confirmed:   confirmed,
				})
				continue
			}
			stale = append(stale, staleAssignment{podID: entities.EntityID(source), nodeID: entities.EntityID(target), podUID: podUID, retiredAt: deadline})
			continue
		}
		if _, ok := activeIDs[id]; ok {
			continue
		}
		stale = append(stale, staleAssignment{podID: entities.EntityID(source), nodeID: entities.EntityID(target), podUID: podUID})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg graph active runs_on iteration: %w", err)
	}
	for _, assignment := range stale {
		at := retiredAt
		if !assignment.retiredAt.IsZero() {
			at = assignment.retiredAt
		}
		if err := s.RetireRunsOn(ctx, assignment.podID, assignment.nodeID, assignment.podUID, at); err != nil {
			return nil, err
		}
	}
	return pending, nil
}

func applySnapshotTimestamp(snap *topology.Snapshot) {
	if snap == nil {
		return
	}
	snap.Graph.Timestamp = snap.CreatedAt
}
