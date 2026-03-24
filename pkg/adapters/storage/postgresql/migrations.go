package postgresql

import (
	"context"
	"fmt"
)

const schemaDDL = `
-- Entity registry: canonical entity records
CREATE TABLE IF NOT EXISTS entities (
    id         TEXT        PRIMARY KEY,
    kind       TEXT        NOT NULL,
    cluster    TEXT        NOT NULL DEFAULT '',
    namespace  TEXT        NOT NULL DEFAULT '',
    name       TEXT        NOT NULL DEFAULT '',
    labels     JSONB       NOT NULL DEFAULT '{}',
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_entities_kind ON entities (kind);
CREATE INDEX IF NOT EXISTS idx_entities_cluster_ns_name ON entities (cluster, namespace, name);

-- Topology snapshots
CREATE TABLE IF NOT EXISTS topology_snapshots (
    id            BIGSERIAL   PRIMARY KEY,
    version_major INT         NOT NULL DEFAULT 1,
    version_minor INT         NOT NULL DEFAULT 0,
    from_ts       TIMESTAMPTZ NOT NULL,
    to_ts         TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Topology nodes (per snapshot)
CREATE TABLE IF NOT EXISTS topology_nodes (
    snapshot_id BIGINT NOT NULL REFERENCES topology_snapshots(id),
    entity_id   TEXT   NOT NULL,
    kind        TEXT   NOT NULL,
    labels      JSONB  NOT NULL DEFAULT '{}',
    PRIMARY KEY (snapshot_id, entity_id)
);

-- Topology edges (accumulated, not per-snapshot)
CREATE TABLE IF NOT EXISTS topology_edges (
    id         TEXT             PRIMARY KEY,
    source     TEXT             NOT NULL,
    target     TEXT             NOT NULL,
    relation   TEXT             NOT NULL,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    provenance TEXT[]           NOT NULL DEFAULT '{}',
    first_seen TIMESTAMPTZ      NOT NULL,
    last_seen  TIMESTAMPTZ      NOT NULL,
    metadata   JSONB            NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_edges_source ON topology_edges (source);
CREATE INDEX IF NOT EXISTS idx_edges_target ON topology_edges (target);
CREATE INDEX IF NOT EXISTS idx_edges_relation ON topology_edges (relation);

-- Cluster profiles
CREATE TABLE IF NOT EXISTS cluster_profiles (
    id            BIGSERIAL   PRIMARY KEY,
    cluster_id    TEXT        NOT NULL,
    cluster_name  TEXT        NOT NULL,
    discovered_at TIMESTAMPTZ NOT NULL,
    profile       JSONB       NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cluster_profiles_name ON cluster_profiles (cluster_name, created_at DESC);

-- Raw envelopes (optional, retention-managed)
CREATE TABLE IF NOT EXISTS raw_envelopes (
    id            TEXT        PRIMARY KEY,
    version_major INT         NOT NULL DEFAULT 1,
    version_minor INT         NOT NULL DEFAULT 0,
    source        TEXT        NOT NULL,
    signal_type   TEXT        NOT NULL,
    timestamp     TIMESTAMPTZ NOT NULL,
    resource      JSONB       NOT NULL DEFAULT '{}',
    payload       JSONB       NOT NULL,
    received_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_raw_signal_type ON raw_envelopes (signal_type, timestamp);
CREATE INDEX IF NOT EXISTS idx_raw_source ON raw_envelopes (source, timestamp);
`

// Migrate runs the schema DDL to create all required tables and indexes.
// Uses CREATE IF NOT EXISTS so it is safe to call on every startup.
func Migrate(ctx context.Context, conn *Conn) error {
	if _, err := conn.pool.Exec(ctx, schemaDDL); err != nil {
		return fmt.Errorf("postgresql migrate: %w", err)
	}
	return nil
}
