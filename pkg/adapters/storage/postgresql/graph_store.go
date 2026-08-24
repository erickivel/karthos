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

// PutStructuralSnapshot retains one complete Kubernetes projection. Current
// graph reads select only the newest projection; earlier snapshots remain
// available as structural history.
func (s *PostgresGraphStore) PutStructuralSnapshot(ctx context.Context, snapshot topology.Snapshot) error {
	return s.putSnapshot(ctx, snapshot, "structural", true)
}

// PutObservedSnapshot stores telemetry-derived and lifecycle observations.
// Their edges remain current state and never alter structural history.
func (s *PostgresGraphStore) PutObservedSnapshot(ctx context.Context, snapshot topology.Snapshot) error {
	return s.putSnapshot(ctx, snapshot, "observation", false)
}

func (s *PostgresGraphStore) putSnapshot(ctx context.Context, snapshot topology.Snapshot, scope string, structural bool) error {
	tx, err := s.conn.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pg graph put snapshot begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Insert snapshot row.
	var snapshotID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO topology_snapshots (scope, version_major, version_minor, from_ts, to_ts)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		scope, snapshot.Version.Major, snapshot.Version.Minor, snapshot.From, snapshot.To,
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

	// Structural edges belong to this complete projection. Observation edges are
	// current telemetry/lifecycle state and retain their existing lifecycle rules.
	for _, e := range snapshot.Graph.Edges {
		metadataJSON, err := json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("pg graph marshal edge metadata: %w", err)
		}
		if structural {
			_, err = tx.Exec(ctx,
				`INSERT INTO topology_snapshot_edges (snapshot_id, id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
				snapshotID, e.ID, string(e.Source), string(e.Target), string(e.Relation),
				e.Confidence, postgresProvenance(e.Provenance), e.FirstSeen, e.LastSeen, metadataJSON,
			)
		} else {
			_, err = tx.Exec(ctx, edgeUpsertSQL,
				e.ID, string(e.Source), string(e.Target), string(e.Relation),
				e.Confidence, postgresProvenance(e.Provenance), e.FirstSeen, e.LastSeen, metadataJSON,
			)
		}
		if err != nil {
			return fmt.Errorf("pg graph upsert edge: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pg graph put snapshot commit: %w", err)
	}
	return nil
}

func postgresProvenance(provenance []string) []string {
	if provenance == nil {
		return []string{}
	}
	return provenance
}

// GetLatest composes the newest complete structural projection with current
// telemetry and lifecycle observations. Structural history is never replayed:
// objects omitted from the newest Kubernetes projection are absent here while
// their earlier snapshots remain stored for historical inspection.
func (s *PostgresGraphStore) GetLatest(ctx context.Context) (*topology.Snapshot, error) {
	structural, structuralID, hasStructural, err := s.latestSnapshot(ctx, "structural")
	if err != nil {
		return nil, err
	}
	observation, _, hasObservation, err := s.latestSnapshot(ctx, "observation")
	if err != nil {
		return nil, err
	}
	if !hasStructural && !hasObservation {
		return nil, nil
	}

	snap := observation
	if hasStructural {
		snap = structural
	}
	nodesByID := make(map[string]topology.Node)
	if err := s.loadLatestObservedNodes(ctx, nodesByID); err != nil {
		return nil, err
	}
	if hasStructural {
		if err := s.loadStructuralNodes(ctx, structuralID, nodesByID); err != nil {
			return nil, err
		}
	}

	edgesByID := make(map[string]topology.Edge)
	if err := s.loadEdges(ctx, latestActiveEdgeQuery, nil, edgesByID); err != nil {
		return nil, err
	}
	if hasStructural {
		if err := s.loadEdges(ctx,
			`SELECT id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata
			 FROM topology_snapshot_edges WHERE snapshot_id = $1`,
			[]any{structuralID}, edgesByID); err != nil {
			return nil, err
		}
	}

	snap.Graph.Nodes = make([]topology.Node, 0, len(nodesByID))
	for _, node := range nodesByID {
		snap.Graph.Nodes = append(snap.Graph.Nodes, node)
	}
	snap.Graph.Edges = make([]topology.Edge, 0, len(edgesByID))
	for _, edge := range edgesByID {
		snap.Graph.Edges = append(snap.Graph.Edges, edge)
	}
	applySnapshotTimestamp(&snap)
	return &snap, nil
}

func (s *PostgresGraphStore) latestSnapshot(ctx context.Context, scope string) (topology.Snapshot, int64, bool, error) {
	var snapshot topology.Snapshot
	var snapshotID int64
	err := s.conn.pool.QueryRow(ctx,
		`SELECT id, version_major, version_minor, from_ts, to_ts, created_at
		 FROM topology_snapshots WHERE scope = $1 ORDER BY id DESC LIMIT 1`, scope,
	).Scan(&snapshotID, &snapshot.Version.Major, &snapshot.Version.Minor, &snapshot.From, &snapshot.To, &snapshot.CreatedAt)
	if err == pgx.ErrNoRows {
		return topology.Snapshot{}, 0, false, nil
	}
	if err != nil {
		return topology.Snapshot{}, 0, false, fmt.Errorf("pg graph get latest %s snapshot: %w", scope, err)
	}
	return snapshot, snapshotID, true, nil
}

func (s *PostgresGraphStore) loadStructuralNodes(ctx context.Context, snapshotID int64, nodes map[string]topology.Node) error {
	rows, err := s.conn.pool.Query(ctx,
		`SELECT entity_id, kind, labels FROM topology_nodes WHERE snapshot_id = $1`, snapshotID)
	if err != nil {
		return fmt.Errorf("pg graph get structural nodes: %w", err)
	}
	defer rows.Close()
	if err := scanNodes(rows, nodes); err != nil {
		return err
	}
	return nil
}

func (s *PostgresGraphStore) loadLatestObservedNodes(ctx context.Context, nodes map[string]topology.Node) error {
	query := `SELECT DISTINCT ON (n.entity_id) n.entity_id, n.kind, n.labels
		FROM topology_nodes n
		JOIN topology_snapshots ts ON ts.id = n.snapshot_id
		WHERE ts.scope = 'observation'`
	args := []any(nil)
	if s.nodeTTL > 0 {
		query += ` AND ts.created_at > NOW() - $1::interval`
		args = []any{s.nodeTTL.String()}
	}
	query += ` ORDER BY n.entity_id, n.snapshot_id DESC`
	rows, err := s.conn.pool.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("pg graph get observed nodes: %w", err)
	}
	defer rows.Close()
	return scanNodes(rows, nodes)
}

func scanNodes(rows pgx.Rows, nodes map[string]topology.Node) error {
	for rows.Next() {
		var node topology.Node
		var id, kind string
		var labelsJSON []byte
		if err := rows.Scan(&id, &kind, &labelsJSON); err != nil {
			return fmt.Errorf("pg graph scan node: %w", err)
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
// changing first_seen or last_seen. Current graph reads filter retired edges
// through latestActiveEdgeQuery while lifecycle reconciliation retains them.
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

// QueryEdges filters the current composed graph by Source, Target, and/or
// Relation. Zero-value filters are ignored.
func (s *PostgresGraphStore) QueryEdges(ctx context.Context, q query.EdgeQuery) ([]topology.Edge, error) {
	snapshot, err := s.GetLatest(ctx)
	if err != nil || snapshot == nil {
		return nil, err
	}

	edges := make([]topology.Edge, 0, len(snapshot.Graph.Edges))
	for _, edge := range snapshot.Graph.Edges {
		if q.Source != "" && edge.Source != q.Source {
			continue
		}
		if q.Target != "" && edge.Target != q.Target {
			continue
		}
		if q.Relation != "" && edge.Relation != q.Relation {
			continue
		}
		edges = append(edges, edge)
	}
	return edges, nil
}
