package postgresql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/schema"
)

// PostgresRawStore implements persistence.RawStore using PostgreSQL.
type PostgresRawStore struct {
	conn *Conn
}

// NewPostgresRawStore creates a new PostgresRawStore with the given connection.
func NewPostgresRawStore(conn *Conn) *PostgresRawStore {
	return &PostgresRawStore{conn: conn}
}

// WriteRaw inserts raw envelopes into the raw_envelopes table.
func (s *PostgresRawStore) WriteRaw(ctx context.Context, envelopes []schema.RawEnvelope) error {
	if len(envelopes) == 0 {
		return nil
	}

	tx, err := s.conn.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pg raw write begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, env := range envelopes {
		resourceJSON, err := json.Marshal(env.Resource)
		if err != nil {
			return fmt.Errorf("pg raw marshal resource: %w", err)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO raw_envelopes (id, version_major, version_minor, source, signal_type, timestamp, resource, payload, received_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (id) DO NOTHING`,
			env.ID, env.Version.Major, env.Version.Minor,
			env.Source, string(env.SignalType), env.Timestamp,
			resourceJSON, env.Payload, env.ReceivedAt,
		)
		if err != nil {
			return fmt.Errorf("pg raw insert envelope: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pg raw write commit: %w", err)
	}
	return nil
}

// QueryRaw retrieves raw envelopes matching the given query filters.
func (s *PostgresRawStore) QueryRaw(ctx context.Context, q persistence.RawQuery) ([]schema.RawEnvelope, error) {
	sql := `SELECT id, version_major, version_minor, source, signal_type, timestamp, resource, payload, received_at
	        FROM raw_envelopes WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if q.SignalType != "" {
		sql += fmt.Sprintf(" AND signal_type = $%d", argIdx)
		args = append(args, string(q.SignalType))
		argIdx++
	}
	if q.Source != "" {
		sql += fmt.Sprintf(" AND source = $%d", argIdx)
		args = append(args, q.Source)
		argIdx++
	}
	if !q.From.IsZero() {
		sql += fmt.Sprintf(" AND timestamp >= $%d", argIdx)
		args = append(args, q.From)
		argIdx++
	}
	if !q.To.IsZero() {
		sql += fmt.Sprintf(" AND timestamp <= $%d", argIdx)
		args = append(args, q.To)
	}

	sql += " ORDER BY timestamp"

	rows, err := s.conn.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("pg raw query: %w", err)
	}
	defer rows.Close()

	var envelopes []schema.RawEnvelope
	for rows.Next() {
		var env schema.RawEnvelope
		var signalType string
		var resourceJSON []byte
		if err := rows.Scan(
			&env.ID, &env.Version.Major, &env.Version.Minor,
			&env.Source, &signalType, &env.Timestamp,
			&resourceJSON, &env.Payload, &env.ReceivedAt,
		); err != nil {
			return nil, fmt.Errorf("pg raw scan: %w", err)
		}
		env.SignalType = schema.SignalType(signalType)
		if err := json.Unmarshal(resourceJSON, &env.Resource); err != nil {
			return nil, fmt.Errorf("pg raw unmarshal resource: %w", err)
		}
		envelopes = append(envelopes, env)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg raw query iteration: %w", err)
	}

	return envelopes, nil
}
