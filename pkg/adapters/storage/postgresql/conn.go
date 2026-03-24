// Package postgresql implements persistence store interfaces using PostgreSQL.
// It uses github.com/jackc/pgx/v5 for connection pooling and query execution.
package postgresql

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Conn wraps a pgx connection pool and provides convenience methods.
type Conn struct {
	pool *pgxpool.Pool
}

// NewConn creates a new connection pool from the given DSN string.
func NewConn(ctx context.Context, dsn string) (*Conn, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgresql connect: %w", err)
	}
	return &Conn{pool: pool}, nil
}

// Ping verifies the database connection is alive.
func (c *Conn) Ping(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

// Close closes the connection pool.
func (c *Conn) Close() error {
	c.pool.Close()
	return nil
}
