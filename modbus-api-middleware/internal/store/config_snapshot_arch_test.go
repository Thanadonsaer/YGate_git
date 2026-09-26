//go:build mips || mipsle

package store

import (
	"path/filepath"
	"strings"
	"testing"

	"chpp/modbus-api-middleware/internal/domain"
)

// mipsValidSnapshot mirrors config_history_test.go's validSnapshot() (the
// SQLite build's equivalent test), including an UnitID left unset with only
// the legacy SlaveID present -- the exact shape the platform push that
// exposed this bug sends.
func mipsValidSnapshot() domain.ConfigSnapshot {
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
			SlaveID: 45, DeviceSetID: 10, PlantCode: "VT1", Enabled: true,
		}},
	}
}

func TestApplyConfigSnapshotPromotesSlaveIDToUnitID(t *testing.T) {
	st, err := OpenNormalized(filepath.Join(t.TempDir(), "test.store"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err = st.ApplyConfigSnapshot(1, mipsValidSnapshot()); err != nil {
		t.Fatalf("ApplyConfigSnapshot() err=%v", err)
	}
	conn, err := st.Connection(100)
	if err != nil {
		t.Fatalf("Connection(100) err=%v", err)
	}
	// Root-cause regression: unitId 0 is the Modbus broadcast address, which
	// a real device never answers a read on. A pushed connection with only
	// slaveId set must still poll on that unit id.
	if conn.UnitID != 45 {
		t.Fatalf("UnitID=%d want 45 (promoted from SlaveID)", conn.UnitID)
	}
}

func TestApplyConfigSnapshotDanglingDeviceSetIDFailsAndLeavesStateUntouched(t *testing.T) {
	st, err := OpenNormalized(filepath.Join(t.TempDir(), "test.store"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err = st.ApplyConfigSnapshot(1, mipsValidSnapshot()); err != nil {
		t.Fatalf("baseline ApplyConfigSnapshot() err=%v", err)
	}
	bad := mipsValidSnapshot()
	bad.Connections[0].DeviceSetID = 999
	err = st.ApplyConfigSnapshot(2, bad)
	if err == nil || !strings.Contains(err.Error(), "unknown deviceSetId") {
		t.Fatalf("ApplyConfigSnapshot() with dangling deviceSetId err=%v, want unknown deviceSetId", err)
	}
	// State from the last good apply must survive a rejected push untouched,
	// same as the SQLite build's rolled-back transaction.
	conn, err := st.Connection(100)
	if err != nil || conn.UnitID != 45 {
		t.Fatalf("Connection(100)=%+v err=%v, want UnitID=45 preserved", conn, err)
	}
	version, err := st.CurrentConfigVersion()
	if err != nil || version != 1 {
		t.Fatalf("CurrentConfigVersion()=%d err=%v, want 1", version, err)
	}
}
