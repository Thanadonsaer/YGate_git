package core

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// mustUUID mirrors platform-api's core package test helper (defined there in
// plants_integration_test.go); roles_test.go/roles_integration_test.go moved
// here from platform-api still need it.
func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	id, err := parseUUID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
