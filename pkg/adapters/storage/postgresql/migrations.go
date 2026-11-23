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
    scope         TEXT        NOT NULL CHECK (scope IN ('structural', 'materialized')),
    version_major INT         NOT NULL DEFAULT 1,
    version_minor INT         NOT NULL DEFAULT 0,
    from_ts       TIMESTAMPTZ NOT NULL,
    to_ts         TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_topology_snapshots_scope_id ON topology_snapshots (scope, id DESC);

-- Topology nodes (per snapshot)
CREATE TABLE IF NOT EXISTS topology_nodes (
    snapshot_id BIGINT NOT NULL REFERENCES topology_snapshots(id),
    entity_id   TEXT   NOT NULL,
    kind        TEXT   NOT NULL,
    labels      JSONB  NOT NULL DEFAULT '{}',
    provenance TEXT[] NOT NULL DEFAULT '{}',
    first_seen TIMESTAMPTZ,
    last_seen TIMESTAMPTZ,
    PRIMARY KEY (snapshot_id, entity_id)
);
ALTER TABLE topology_nodes ADD COLUMN IF NOT EXISTS provenance TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE topology_nodes ADD COLUMN IF NOT EXISTS first_seen TIMESTAMPTZ;
ALTER TABLE topology_nodes ADD COLUMN IF NOT EXISTS last_seen TIMESTAMPTZ;

-- Structural and materialized graph edges are retained per complete projection.
-- They are intentionally separate from current telemetry/lifecycle observations.
CREATE TABLE IF NOT EXISTS topology_snapshot_edges (
    snapshot_id BIGINT           NOT NULL REFERENCES topology_snapshots(id),
    id          TEXT             NOT NULL,
    source      TEXT             NOT NULL,
    target      TEXT             NOT NULL,
    relation    TEXT             NOT NULL,
    confidence  DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    provenance  TEXT[]           NOT NULL DEFAULT '{}',
    first_seen  TIMESTAMPTZ      NOT NULL,
    last_seen   TIMESTAMPTZ      NOT NULL,
    metadata    JSONB            NOT NULL DEFAULT '{}',
    PRIMARY KEY (snapshot_id, id)
);

CREATE INDEX IF NOT EXISTS idx_snapshot_edges_snapshot ON topology_snapshot_edges (snapshot_id);

-- Current telemetry and lifecycle edges (not structural history)
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

-- Each write is a distinct observation. The current-edge table above may
-- merge disjoint observations and therefore cannot answer a time-window query.
CREATE TABLE IF NOT EXISTS topology_edge_observations (
    id BIGSERIAL PRIMARY KEY,
    edge_id TEXT NOT NULL,
    source TEXT NOT NULL,
    target TEXT NOT NULL,
    relation TEXT NOT NULL,
    confidence DOUBLE PRECISION NOT NULL,
    provenance TEXT[] NOT NULL DEFAULT '{}',
    first_seen TIMESTAMPTZ NOT NULL,
    last_seen TIMESTAMPTZ NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_edge_observations_window ON topology_edge_observations (last_seen, first_seen);

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

`

// Migrate runs the schema DDL to create all required tables and indexes.
// This materialized-snapshot model requires a fresh external schema; it does
// not convert a prior accumulated-topology installation.
func Migrate(ctx context.Context, conn *Conn) error {
	if _, err := conn.pool.Exec(ctx, schemaDDL); err != nil {
		return fmt.Errorf("postgresql migrate: %w", err)
	}
	return nil
}
