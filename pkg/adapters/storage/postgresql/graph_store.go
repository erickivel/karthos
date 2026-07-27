package postgresql

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/topology"
)

const edgeUpsertSQL = `INSERT INTO topology_edges (id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata)
	 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	 ON CONFLICT (id) DO UPDATE SET
	   first_seen = LEAST(topology_edges.first_seen, EXCLUDED.first_seen),
	   last_seen = CASE WHEN topology_edges.relation = 'runs_on' AND topology_edges.metadata ? 'retired_at' THEN LEAST(topology_edges.last_seen, EXCLUDED.last_seen) ELSE GREATEST(topology_edges.last_seen, EXCLUDED.last_seen) END,
	   confidence = EXCLUDED.confidence,
	   provenance = EXCLUDED.provenance,
	   metadata = CASE WHEN topology_edges.relation = 'runs_on' AND (topology_edges.metadata ? 'retired_at' OR topology_edges.metadata ? 'terminal_deadline') THEN topology_edges.metadata || EXCLUDED.metadata ELSE EXCLUDED.metadata END`

const latestActiveEdgeQuery = `SELECT id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata
	 FROM topology_edges
	 WHERE relation <> 'runs_on' OR (
	   NOT (metadata ? 'retired_at') AND
	   (NOT (metadata ? 'terminal_deadline') OR (metadata->>'terminal_deadline')::timestamptz > NOW())
	 )`

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
	nodeTTL time.Duration // 0 = no TTL filter
}

// PostgresGraphStoreOption configures a PostgresGraphStore.
type PostgresGraphStoreOption func(*PostgresGraphStore)

// WithPostgresNodeTTL filters GetLatest to nodes whose containing snapshot was
// created within ttl. 0 disables the TTL.
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

// PutSnapshot stores a topology snapshot. Edges are upserted (accumulated)
// using ON CONFLICT to update last_seen. Nodes are stored per-snapshot and
// materialized cumulatively at read time.
func (s *PostgresGraphStore) PutSnapshot(ctx context.Context, snapshot topology.Snapshot) error {
	tx, err := s.conn.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pg graph put snapshot begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Insert snapshot row.
	var snapshotID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO topology_snapshots (version_major, version_minor, from_ts, to_ts)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		snapshot.Version.Major, snapshot.Version.Minor, snapshot.From, snapshot.To,
	).Scan(&snapshotID)
	if err != nil {
		return fmt.Errorf("pg graph insert snapshot: %w", err)
	}

	// Batch insert nodes.
	for _, n := range snapshot.Graph.Nodes {
		labelsJSON, err := json.Marshal(n.Labels)
		if err != nil {
			return fmt.Errorf("pg graph marshal node labels: %w", err)
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO topology_nodes (snapshot_id, entity_id, kind, labels)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (snapshot_id, entity_id) DO NOTHING`,
			snapshotID, string(n.ID), string(n.Kind), labelsJSON,
		)
		if err != nil {
			return fmt.Errorf("pg graph insert node: %w", err)
		}
	}

	// Upsert edges (accumulated).
	for _, e := range snapshot.Graph.Edges {
		metadataJSON, err := json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("pg graph marshal edge metadata: %w", err)
		}
		_, err = tx.Exec(ctx, edgeUpsertSQL,
			e.ID, string(e.Source), string(e.Target), string(e.Relation),
			e.Confidence, e.Provenance, e.FirstSeen, e.LastSeen, metadataJSON,
		)
		if err != nil {
			return fmt.Errorf("pg graph upsert edge: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pg graph put snapshot commit: %w", err)
	}
	return nil
}

// GetLatest returns the latest snapshot with the accumulated node and edge set.
// Returns nil, nil if no snapshot exists.
func (s *PostgresGraphStore) GetLatest(ctx context.Context) (*topology.Snapshot, error) {
	// Get latest snapshot.
	var snap topology.Snapshot
	var snapshotID int64
	err := s.conn.pool.QueryRow(ctx,
		`SELECT id, version_major, version_minor, from_ts, to_ts, created_at
		 FROM topology_snapshots ORDER BY id DESC LIMIT 1`,
	).Scan(&snapshotID, &snap.Version.Major, &snap.Version.Minor, &snap.From, &snap.To, &snap.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("pg graph get latest snapshot: %w", err)
	}

	// Get the latest version of each node across all snapshots so structural nodes
	// remain visible after newer runtime-only snapshots are written.
	// When nodeTTL > 0, drop nodes whose most recent containing snapshot was created
	// before NOW() - nodeTTL so deleted entities eventually age out.
	var nodeRows pgx.Rows
	if s.nodeTTL > 0 {
		sql, args := latestNodeQuery(s.nodeTTL)
		nodeRows, err = s.conn.pool.Query(ctx, sql, args...)
	} else {
		sql, args := latestNodeQuery(0)
		nodeRows, err = s.conn.pool.Query(ctx, sql, args...)
	}
	if err != nil {
		return nil, fmt.Errorf("pg graph get nodes: %w", err)
	}
	defer nodeRows.Close()

	for nodeRows.Next() {
		var n topology.Node
		var id, kind string
		var labelsJSON []byte
		if err := nodeRows.Scan(&id, &kind, &labelsJSON); err != nil {
			return nil, fmt.Errorf("pg graph scan node: %w", err)
		}
		n.ID = entities.EntityID(id)
		n.Kind = entities.EntityKind(kind)
		if err := json.Unmarshal(labelsJSON, &n.Labels); err != nil {
			return nil, fmt.Errorf("pg graph unmarshal node labels: %w", err)
		}
		snap.Graph.Nodes = append(snap.Graph.Nodes, n)
	}
	if err := nodeRows.Err(); err != nil {
		return nil, fmt.Errorf("pg graph nodes iteration: %w", err)
	}

	// Get all accumulated edges.
	edgeRows, err := s.conn.pool.Query(ctx, latestActiveEdgeQuery)
	if err != nil {
		return nil, fmt.Errorf("pg graph get edges: %w", err)
	}
	defer edgeRows.Close()

	for edgeRows.Next() {
		var e topology.Edge
		var source, target, relation string
		var metadataJSON []byte
		if err := edgeRows.Scan(
			&e.ID, &source, &target, &relation, &e.Confidence,
			&e.Provenance, &e.FirstSeen, &e.LastSeen, &metadataJSON,
		); err != nil {
			return nil, fmt.Errorf("pg graph scan edge: %w", err)
		}
		e.Source = entities.EntityID(source)
		e.Target = entities.EntityID(target)
		e.Relation = topology.RelationType(relation)
		if err := json.Unmarshal(metadataJSON, &e.Metadata); err != nil {
			return nil, fmt.Errorf("pg graph unmarshal edge metadata: %w", err)
		}
		snap.Graph.Edges = append(snap.Graph.Edges, e)
	}
	if err := edgeRows.Err(); err != nil {
		return nil, fmt.Errorf("pg graph edges iteration: %w", err)
	}

	applySnapshotTimestamp(&snap)
	return &snap, nil
}

// RetireRunsOn records terminal Pod lifecycle state in edge metadata without
// changing first_seen or last_seen. QueryEdges keeps the retired audit edge;
// GetLatest filters it through latestActiveEdgeQuery.
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
	metadata, err := json.Marshal(map[string]string{
		"k8s.cluster.name":   grace.ClusterName,
		"k8s.namespace.name": grace.Namespace,
		"k8s.pod.name":       grace.PodName,
		"k8s.pod.uid":        grace.PodUID,
		"k8s.node.name":      grace.NodeName,
		"terminal_deadline":  grace.Deadline.UTC().Format(time.RFC3339Nano),
	})
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
		rows.Close()
		return nil, fmt.Errorf("pg graph active runs_on iteration: %w", err)
	}
	rows.Close()
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

func latestNodeQuery(ttl time.Duration) (string, []any) {
	if ttl > 0 {
		return `SELECT DISTINCT ON (n.entity_id) n.entity_id, n.kind, n.labels
			 FROM topology_nodes n
			 JOIN topology_snapshots ts ON ts.id = n.snapshot_id
			 WHERE ts.created_at > NOW() - $1::interval
			 ORDER BY n.entity_id, n.snapshot_id DESC`,
			[]any{ttl.String()}
	}

	return `SELECT DISTINCT ON (entity_id) entity_id, kind, labels
			 FROM topology_nodes
			 ORDER BY entity_id, snapshot_id DESC`,
		nil
}

func applySnapshotTimestamp(snap *topology.Snapshot) {
	if snap == nil {
		return
	}
	snap.Graph.Timestamp = snap.CreatedAt
}

// QueryEdges filters accumulated edges by Source and/or Target.
// Zero-value EntityID means no filter on that field.
func (s *PostgresGraphStore) QueryEdges(ctx context.Context, q query.EdgeQuery) ([]topology.Edge, error) {
	sql := `SELECT id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata
	        FROM topology_edges WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if q.Source != "" {
		sql += fmt.Sprintf(" AND source = $%d", argIdx)
		args = append(args, string(q.Source))
		argIdx++
	}
	if q.Target != "" {
		sql += fmt.Sprintf(" AND target = $%d", argIdx)
		args = append(args, string(q.Target))
	}

	rows, err := s.conn.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("pg graph query edges: %w", err)
	}
	defer rows.Close()

	var edges []topology.Edge
	for rows.Next() {
		var e topology.Edge
		var source, target, relation string
		var metadataJSON []byte
		if err := rows.Scan(
			&e.ID, &source, &target, &relation, &e.Confidence,
			&e.Provenance, &e.FirstSeen, &e.LastSeen, &metadataJSON,
		); err != nil {
			return nil, fmt.Errorf("pg graph scan edge: %w", err)
		}
		e.Source = entities.EntityID(source)
		e.Target = entities.EntityID(target)
		e.Relation = topology.RelationType(relation)
		if err := json.Unmarshal(metadataJSON, &e.Metadata); err != nil {
			return nil, fmt.Errorf("pg graph unmarshal edge metadata: %w", err)
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg graph query edges iteration: %w", err)
	}

	return edges, nil
}
