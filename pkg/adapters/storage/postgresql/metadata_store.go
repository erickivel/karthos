package postgresql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/erickivel/karthos/pkg/discovery/profile"
)

// PostgresMetadataStore implements persistence.MetadataStore using PostgreSQL.
type PostgresMetadataStore struct {
	conn *Conn
}

// NewPostgresMetadataStore creates a new PostgresMetadataStore with the given connection.
func NewPostgresMetadataStore(conn *Conn) *PostgresMetadataStore {
	return &PostgresMetadataStore{conn: conn}
}

// PutDiscovery stores a cluster profile as a new row in the cluster_profiles table.
func (s *PostgresMetadataStore) PutDiscovery(ctx context.Context, snapshot profile.ClusterProfile) error {
	profileJSON, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("pg metadata marshal profile: %w", err)
	}

	_, err = s.conn.pool.Exec(ctx,
		`INSERT INTO cluster_profiles (cluster_id, cluster_name, discovered_at, profile)
		 VALUES ($1, $2, $3, $4)`,
		string(snapshot.ClusterID), snapshot.ClusterName, snapshot.DiscoveredAt, profileJSON,
	)
	if err != nil {
		return fmt.Errorf("pg metadata put discovery: %w", err)
	}
	return nil
}

// GetDiscovery returns the most recently stored cluster profile.
// Returns nil, nil if no profile has been stored.
func (s *PostgresMetadataStore) GetDiscovery(ctx context.Context) (*profile.ClusterProfile, error) {
	var profileJSON []byte
	err := s.conn.pool.QueryRow(ctx,
		`SELECT profile FROM cluster_profiles ORDER BY created_at DESC LIMIT 1`,
	).Scan(&profileJSON)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("pg metadata get discovery: %w", err)
	}

	var prof profile.ClusterProfile
	if err := json.Unmarshal(profileJSON, &prof); err != nil {
		return nil, fmt.Errorf("pg metadata unmarshal profile: %w", err)
	}
	return &prof, nil
}
