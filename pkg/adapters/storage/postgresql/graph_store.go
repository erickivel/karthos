package postgresql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/topology"
)

// PostgresGraphStore implements persistence.GraphStore using PostgreSQL.
type PostgresGraphStore struct {
	conn *Conn
}

// NewPostgresGraphStore creates a new PostgresGraphStore with the given connection.
func NewPostgresGraphStore(conn *Conn) *PostgresGraphStore {
	return &PostgresGraphStore{conn: conn}
}

// PutSnapshot stores a topology snapshot. Edges are upserted (accumulated)
// using ON CONFLICT to update last_seen. Nodes are stored per-snapshot.
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
		_, err = tx.Exec(ctx,
			`INSERT INTO topology_edges (id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (id) DO UPDATE SET
			   last_seen = GREATEST(topology_edges.last_seen, EXCLUDED.last_seen),
			   confidence = EXCLUDED.confidence,
			   provenance = EXCLUDED.provenance,
			   metadata = EXCLUDED.metadata`,
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

// GetLatest returns the latest snapshot with its nodes and all accumulated edges.
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

	// Get nodes for the latest snapshot.
	nodeRows, err := s.conn.pool.Query(ctx,
		`SELECT entity_id, kind, labels FROM topology_nodes WHERE snapshot_id = $1`, snapshotID,
	)
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
	edgeRows, err := s.conn.pool.Query(ctx,
		`SELECT id, source, target, relation, confidence, provenance, first_seen, last_seen, metadata
		 FROM topology_edges`,
	)
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

	return &snap, nil
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
