// Package testdb hands an integration test its own throwaway PostgreSQL
// database, migrated and dropped again on cleanup.
//
// Tests that share one database are not independent: they see each other's
// rows, so a query like "count(*) FROM audit_log" silently counts whatever
// the previous test left behind, and two tests inserting the same fixed
// UUID collide on the second one to run. Both had already happened here.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ygate/platform-api/internal/database"
)

// Disposable creates a fresh database on the server named by databaseURL,
// opens it through database.Open (so every migration is applied), and
// registers cleanup that closes the pool and drops the database again.
//
// databaseURL points at any existing database on the target server -- it is
// only used to connect as an admin and to borrow the connection settings;
// nothing is written to it.
func Disposable(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Del("schema")
	query.Set("search_path", "public")
	parsed.RawQuery = query.Encode()
	admin, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}

	// Package test binaries run in parallel, so a timestamp alone can collide.
	var seed [8]byte
	if _, err = rand.Read(seed[:]); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	name := fmt.Sprintf("ygate_test_%s", hex.EncodeToString(seed[:]))
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		admin.Close()
		t.Fatal(err)
	}

	// Registered before the pool below, so cleanup runs in LIFO order: the
	// test's own pool closes first, then the database drops.
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+identifier)
		admin.Close()
	})

	dbURL := *parsed
	dbURL.Path = "/" + name
	pool, err := database.Open(ctx, dbURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
