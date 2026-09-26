package core

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// packages/api-contracts/fixtures/middleware-config-snapshot.json is the
// config.snapshot wire shape both modules test against (ADR-0005; ADR-0001
// forbids sharing Go code). The Middleware decodes it strictly and applies
// it; here every field must survive a round trip through the Platform's
// types, so a rename or drop on either side fails a test.
func TestMiddlewareConfigSnapshotMatchesSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("../../../../packages/api-contracts/fixtures/middleware-config-snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot MiddlewareConfigSnapshot
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	if err = json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("Platform snapshot drifted from the shared fixture:\nwant %s\ngot  %s", raw, encoded)
	}
}
