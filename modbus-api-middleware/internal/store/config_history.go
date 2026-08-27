//go:build !mips && !mipsle

package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"chpp/modbus-api-middleware/internal/domain"
)

const configHistorySchema = `
CREATE TABLE IF NOT EXISTS config_history (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 version INTEGER NOT NULL,
 status TEXT NOT NULL,
 reason TEXT NOT NULL DEFAULT '',
 snapshot TEXT NOT NULL DEFAULT '',
 applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);`

func (s *Store) ensureConfigHistory() error {
	_, err := s.DB.Exec(configHistorySchema)
	return err
}

// CurrentConfigVersion returns the highest version ever successfully
// applied locally, or 0 if none has. Sent as appliedVersion in the
// realtime client's hello so the platform only pushes when it is ahead.
func (s *Store) CurrentConfigVersion() (int64, error) {
	if err := s.ensureConfigHistory(); err != nil {
		return 0, err
	}
	var version sql.NullInt64
	if err := s.DB.QueryRow(`SELECT MAX(version) FROM config_history WHERE status='APPLIED'`).Scan(&version); err != nil {
		return 0, err
	}
	return version.Int64, nil
}

// ApplyConfigSnapshot replaces the entire local configuration (brands,
// device sets, addresses, connections, plants) with snapshot inside a
// single transaction, reusing the same normalize/validate helpers local
// edits already go through (normalizeDeviceSet/normalizeAddress/
// validateAddress/normalizeConnection). Any validation or write failure
// rolls the whole transaction back -- SQLite is left exactly as it was --
// and is recorded as a FAILED config_history row instead. The caller must
// only hot-swap the in-memory cache after this returns nil.
//
// IDs inside snapshot (brandId, deviceSetId, addressId, connectionId) are
// written as the actual local primary keys (SQLite allows explicit values
// on an INTEGER PRIMARY KEY column), not left to autoincrement. They're
// deterministic, collision-free wire IDs derived from the platform's own
// UUIDs (see wireID in platform-api's middleware_config.go) -- preserving
// them keeps a Device's connectionId stable across every push, which the
// platform relies on to route command.request{connectionId} (Test
// Connection / Test Read) to the right row after a config push. Letting
// SQLite reassign fresh autoincrement IDs on every apply used to break that
// silently: the platform kept sending the wireID it always had, but the
// local row now had a different con_id, so every command failed with
// "connection not found" the moment a config had ever been re-applied.
func (s *Store) ApplyConfigSnapshot(version int64, snapshot domain.ConfigSnapshot) error {
	if err := s.ensureConfigHistory(); err != nil {
		return err
	}
	applyErr := s.applyConfigSnapshotTx(snapshot)
	status, reason := "APPLIED", ""
	if applyErr != nil {
		status, reason = "FAILED", applyErr.Error()
	}
	raw, _ := json.Marshal(snapshot)
	if _, err := s.DB.Exec(`INSERT INTO config_history(version,status,reason,snapshot) VALUES(?,?,?,?)`, version, status, reason, string(raw)); err != nil {
		return err
	}
	return applyErr
}

func (s *Store) applyConfigSnapshotTx(snapshot domain.ConfigSnapshot) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range []string{"addresses", "connections", "device_sets", "brands", "plants"} {
		if _, err = tx.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}

	brandIDs := map[int64]bool{}
	for _, b := range snapshot.Brands {
		name := strings.TrimSpace(b.BrandName)
		if name == "" {
			return fmt.Errorf("brand: brandName is required")
		}
		if _, err = tx.Exec(`INSERT INTO brands(brand_id,brand_name,brand_description) VALUES(?,?,?)`, b.BrandID, name, strings.TrimSpace(b.BrandDescription)); err != nil {
			return fmt.Errorf("brand %q: %w", name, err)
		}
		brandIDs[b.BrandID] = true
	}

	deviceSetIDs := map[int64]bool{}
	for _, ds := range snapshot.DeviceSets {
		normalized := normalizeDeviceSet(ds)
		if !brandIDs[ds.BrandID] {
			return fmt.Errorf("device set %q: unknown brandId %d", normalized.DevModel, ds.BrandID)
		}
		if strings.TrimSpace(normalized.DevType) == "" || strings.TrimSpace(normalized.DevModel) == "" {
			return fmt.Errorf("device set: devType and devModel are required")
		}
		if !oneOf(normalized.ByteOrder, "BIG_ENDIAN", "LITTLE_ENDIAN") || !oneOf(normalized.WordOrder, "HIGH_LOW", "LOW_HIGH") || normalized.MaxBlockSize < 1 || normalized.MaxBlockSize > 125 {
			return fmt.Errorf("device set %q: invalid byteOrder/wordOrder/maxBlockSize", normalized.DevModel)
		}
		if len(ds.Addresses) == 0 {
			return fmt.Errorf("device set %q: at least one address is required", normalized.DevModel)
		}
		if _, err = tx.Exec(`INSERT INTO device_sets(dev_set_id,brand_id,dev_type_id,dev_type,dev_model,address_mode,byte_order,word_order,max_block_size) VALUES(?,?,?,?,?,?,?,?,?)`,
			ds.DeviceSetID, ds.BrandID, normalized.DevTypeID, normalized.DevType, normalized.DevModel, normalized.AddressMode, normalized.ByteOrder, normalized.WordOrder, normalized.MaxBlockSize); err != nil {
			return fmt.Errorf("device set %q: %w", normalized.DevModel, err)
		}
		deviceSetIDs[ds.DeviceSetID] = true

		for i, addr := range ds.Addresses {
			a, err := normalizeAddress(addr, normalized.AddressMode)
			if err != nil {
				return fmt.Errorf("device set %q address %d: %w", normalized.DevModel, i+1, err)
			}
			if err = validateAddress(a, normalized.AddressMode); err != nil {
				return fmt.Errorf("device set %q address %d: %w", normalized.DevModel, i+1, err)
			}
			if _, err = tx.Exec(`INSERT INTO addresses(address_id,dev_set_id,address_fc,address_register,address_description,canonical_key,source_tag,address_factor,address_offset,address_data_type,address_length,word_order,source_unit,canonical_unit,address_remark,enabled) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				addr.AddressID, ds.DeviceSetID, a.FunctionCode, a.Register, a.Description, a.CanonicalKey, a.SourceTag, a.Factor, a.Offset, a.DataType, a.Length, a.WordOrder, a.SourceUnit, a.CanonicalUnit, a.Remark, a.Enabled); err != nil {
				return fmt.Errorf("device set %q address %d: %w", normalized.DevModel, i+1, err)
			}
		}
	}

	for _, p := range snapshot.Plants {
		code := strings.ToUpper(strings.TrimSpace(p.PlantCode))
		name := strings.TrimSpace(p.PlantName)
		if code == "" || name == "" {
			return fmt.Errorf("plant: plantCode and plantName are required")
		}
		if _, err = tx.Exec(`INSERT INTO plants(plant_code,plant_name) VALUES(?,?) ON CONFLICT(plant_code) DO UPDATE SET plant_name=excluded.plant_name`, code, name); err != nil {
			return fmt.Errorf("plant %q: %w", code, err)
		}
	}

	for _, conn := range snapshot.Connections {
		normalized := normalizeConnection(conn)
		if !deviceSetIDs[conn.DeviceSetID] {
			return fmt.Errorf("connection %q: unknown deviceSetId %d", normalized.ConnectionName, conn.DeviceSetID)
		}
		if strings.TrimSpace(normalized.ConnectionName) == "" || strings.TrimSpace(normalized.Host) == "" || normalized.Port < 1 || normalized.Port > 65535 {
			return fmt.Errorf("connection %q: connectionName, host and valid port are required", normalized.ConnectionName)
		}
		if _, err = tx.Exec(`INSERT INTO connections(con_id,con_name,con_host,con_port,unit_id,con_dev_set,dev_dn,device_name,plant_code,plant_name,enabled) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			conn.ConnectionID, normalized.ConnectionName, normalized.Host, normalized.Port, normalized.UnitID, conn.DeviceSetID, normalized.DevDn, normalized.DeviceName, normalized.PlantCode, normalized.PlantName, normalized.Enabled); err != nil {
			return fmt.Errorf("connection %q: %w", normalized.ConnectionName, err)
		}
	}

	return tx.Commit()
}
