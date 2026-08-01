package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open connects to the shared PostgreSQL instance. auth-service does not run
// migrations itself: the database and its schema_migrations ledger are owned
// by platform-api (see services/platform-api/internal/database/database.go),
// which every service depends on having applied migrations already. This is
// a deliberate deviation from a byte-for-byte copy of platform-api's
// database.go -- Go's //go:embed directive cannot reference
// "../platform-api/internal/database/migrations" from this module, and
// duplicating the migration *.sql files into this module would violate the
// "one shared migration set" rule the design spec calls for.
func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("configure PostgreSQL pool: %w", err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}
	return pool, nil
}
