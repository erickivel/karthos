package postgresql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
)

// PostgresEntityRegistry implements persistence.EntityRegistry using PostgreSQL.
type PostgresEntityRegistry struct {
	conn *Conn
}

// NewPostgresEntityRegistry creates a new PostgresEntityRegistry with the given connection.
func NewPostgresEntityRegistry(conn *Conn) *PostgresEntityRegistry {
	return &PostgresEntityRegistry{conn: conn}
}

// UpsertEntity inserts or updates an entity record. On conflict, labels and
// last_seen are updated.
func (r *PostgresEntityRegistry) UpsertEntity(ctx context.Context, entity entities.Entity) error {
	labels := entity.Labels()
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return fmt.Errorf("pg entity marshal labels: %w", err)
	}

	cluster := labels["cluster"]
	namespace := labels["namespace"]
	name := labels["name"]

	_, err = r.conn.pool.Exec(ctx,
		`INSERT INTO entities (id, kind, cluster, namespace, name, labels)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (id) DO UPDATE SET
		   labels = EXCLUDED.labels,
		   name = EXCLUDED.name,
		   last_seen = NOW()`,
		string(entity.ID()), string(entity.Kind()), cluster, namespace, name, labelsJSON,
	)
	if err != nil {
		return fmt.Errorf("pg entity upsert: %w", err)
	}
	return nil
}

// GetEntity returns the entity with the given ID.
// Returns an error if the entity is not found.
func (r *PostgresEntityRegistry) GetEntity(ctx context.Context, id entities.EntityID) (entities.Entity, error) {
	var e persistence.StoredEntity
	var kind string
	var labelsJSON []byte
	err := r.conn.pool.QueryRow(ctx,
		`SELECT id, kind, cluster, namespace, name, labels FROM entities WHERE id = $1`,
		string(id),
	).Scan(&e.EntityID, &kind, &e.Cluster, &e.Namespace, &e.Name, &labelsJSON)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", persistence.ErrEntityNotFound, id)
		}
		return nil, fmt.Errorf("pg entity get: %w", err)
	}
	e.EntityKind = entities.EntityKind(kind)
	if err := json.Unmarshal(labelsJSON, &e.Metadata); err != nil {
		return nil, fmt.Errorf("pg entity unmarshal labels: %w", err)
	}
	return e, nil
}

// ListEntities returns entities matching the query filters.
func (r *PostgresEntityRegistry) ListEntities(ctx context.Context, q persistence.EntityQuery) ([]entities.Entity, error) {
	sql := `SELECT id, kind, cluster, namespace, name, labels FROM entities WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if q.Kind != "" {
		sql += fmt.Sprintf(" AND kind = $%d", argIdx)
		args = append(args, string(q.Kind))
		argIdx++
	}
	if q.Namespace != "" {
		sql += fmt.Sprintf(" AND namespace = $%d", argIdx)
		args = append(args, q.Namespace)
	}

	rows, err := r.conn.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("pg entity list: %w", err)
	}
	defer rows.Close()

	var result []entities.Entity
	for rows.Next() {
		var e persistence.StoredEntity
		var kind string
		var labelsJSON []byte
		if err := rows.Scan(&e.EntityID, &kind, &e.Cluster, &e.Namespace, &e.Name, &labelsJSON); err != nil {
			return nil, fmt.Errorf("pg entity scan: %w", err)
		}
		e.EntityKind = entities.EntityKind(kind)
		if err := json.Unmarshal(labelsJSON, &e.Metadata); err != nil {
			return nil, fmt.Errorf("pg entity unmarshal labels: %w", err)
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg entity list iteration: %w", err)
	}

	return result, nil
}
