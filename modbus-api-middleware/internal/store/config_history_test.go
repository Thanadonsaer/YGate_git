//go:build !mips && !mipsle

package store

import (
	"path/filepath"
	"strings"
	"testing"

	"chpp/modbus-api-middleware/internal/domain"
)

func validSnapshot() domain.ConfigSnapshot {
	return domain.ConfigSnapshot{
		Version: 1,
		Brands:  []domain.Brand{{BrandID: 1, BrandName: "Huawei"}},
		DeviceSets: []domain.DeviceSet{{
			DeviceSetID: 10, BrandID: 1, DevType: "Inverter", DevModel: "SUN2000",
			ByteOrder: "BIG_ENDIAN", WordOrder: "HIGH_LOW", MaxBlockSize: 30,
			Addresses: []domain.Address{{FunctionCode: 3, Register: 32080, Description: "Active power", Factor: 1, DataType: "U32"}},
		}},
		Plants: []domain.Plant{{PlantCode: "VT1", PlantName: "VT1"}},
		Connections: []domain.ConnectionConfig{{
			ConnectionID: 100, ConnectionName: "VT1-INV-01", Host: "127.0.0.1", Port: 502,
			DeviceSetID: 10, PlantCode: "VT1", Enabled: true,
		}},
	}
}

func TestApplyConfigSnapshotValidSnapshotIsAppliedAndVisible(t *testing.T) {
	st, err := OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err = st.ApplyConfigSnapshot(1, validSnapshot()); err != nil {
		t.Fatalf("ApplyConfigSnapshot() err=%v", err)
	}

	version, err := st.CurrentConfigVersion()
	if err != nil || version != 1 {
		t.Fatalf("CurrentConfigVersion()=%d err=%v want 1", version, err)
	}

	brands, err := st.Brands()
	if err != nil || len(brands) != 1 || brands[0].BrandName != "Huawei" {
		t.Fatalf("Brands()=%+v err=%v", brands, err)
	}
	sets, err := st.DeviceSets()
	if err != nil || len(sets) != 1 || sets[0].DevModel != "SUN2000" || len(sets[0].Addresses) != 1 {
		t.Fatalf("DeviceSets()=%+v err=%v", sets, err)
	}
	connections, err := st.Connections()
	if err != nil || len(connections) != 1 || connections[0].Host != "127.0.0.1" {
		t.Fatalf("Connections()=%+v err=%v", connections, err)
	}
	// The platform routes command.request{connectionId} (Test Connection /
	// Test Read) using the ConnectionID it sent in the snapshot -- if the
	// locally applied row got a different (autoincrement-assigned) ID, every
	// such command would fail with "connection not found" even though the
	// gateway is online and the connection genuinely exists.
	if connections[0].ConnectionID != 100 {
		t.Fatalf("Connections()[0].ConnectionID=%d, want 100 (must match the pushed snapshot's id, not a locally reassigned one)", connections[0].ConnectionID)
	}
}

func TestApplyConfigSnapshotPreservesConnectionIDAcrossReapply(t *testing.T) {
	st, err := OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err = st.ApplyConfigSnapshot(1, validSnapshot()); err != nil {
		t.Fatalf("first ApplyConfigSnapshot() err=%v", err)
	}
	snapshot2 := validSnapshot()
	snapshot2.Version = 2
	if err = st.ApplyConfigSnapshot(2, snapshot2); err != nil {
		t.Fatalf("second ApplyConfigSnapshot() err=%v", err)
	}
	connections, err := st.Connections()
	if err != nil || len(connections) != 1 || connections[0].ConnectionID != 100 {
		t.Fatalf("Connections() after reapply = %+v err=%v, want ConnectionID=100 unchanged across pushes", connections, err)
	}
}

func TestApplyConfigSnapshotDanglingDeviceSetIDFailsAndLeavesStateUntouched(t *testing.T) {
	st, err := OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Establish a known-good baseline first.
	if err = st.ApplyConfigSnapshot(1, validSnapshot()); err != nil {
		t.Fatalf("baseline ApplyConfigSnapshot() err=%v", err)
	}

	bad := validSnapshot()
	bad.Version = 2
	bad.Connections[0].DeviceSetID = 999999 // does not exist in bad.DeviceSets

	err = st.ApplyConfigSnapshot(2, bad)
	if err == nil {
		t.Fatal("ApplyConfigSnapshot() with dangling deviceSetId did not return an error")
	}
	if !strings.Contains(err.Error(), "unknown deviceSetId") {
		t.Fatalf("err=%v want mention of unknown deviceSetId", err)
	}

	// The rolled-back transaction must leave the previously-applied (v1)
	// state exactly as it was, not partially applied and not empty.
	version, err := st.CurrentConfigVersion()
	if err != nil || version != 1 {
		t.Fatalf("CurrentConfigVersion()=%d err=%v want 1 (rollback must not advance it)", version, err)
	}
	brands, err := st.Brands()
	if err != nil || len(brands) != 1 {
		t.Fatalf("Brands() after failed apply=%+v err=%v, want the v1 baseline untouched", brands, err)
	}
	connections, err := st.Connections()
	if err != nil || len(connections) != 1 {
		t.Fatalf("Connections() after failed apply=%+v err=%v, want the v1 baseline untouched", connections, err)
	}

	// And the failure itself must be recorded.
	var status, reason string
	if scanErr := st.DB.QueryRow(`SELECT status, reason FROM config_history WHERE version=2`).Scan(&status, &reason); scanErr != nil {
		t.Fatalf("config_history row for v2 not found: %v", scanErr)
	}
	if status != "FAILED" || reason == "" {
		t.Fatalf("config_history v2 status=%q reason=%q, want FAILED with a reason", status, reason)
	}
}
