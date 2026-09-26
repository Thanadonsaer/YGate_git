//go:build !mips && !mipsle

package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

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
	snapshot, err := normalizeConfigSnapshot(snapshot)
	if err != nil {
		return err
	}
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
	for _, b := range snapshot.Brands {
		if _, err = tx.Exec(`INSERT INTO brands(brand_id,brand_name,brand_description) VALUES(?,?,?)`, b.BrandID, b.BrandName, b.BrandDescription); err != nil {
			return fmt.Errorf("brand %q: %w", b.BrandName, err)
		}
	}
	for _, ds := range snapshot.DeviceSets {
		if _, err = tx.Exec(`INSERT INTO device_sets(dev_set_id,brand_id,dev_type_id,dev_type,dev_model,address_mode,byte_order,word_order,max_block_size) VALUES(?,?,?,?,?,?,?,?,?)`,
			ds.DeviceSetID, ds.BrandID, ds.DevTypeID, ds.DevType, ds.DevModel, ds.AddressMode, ds.ByteOrder, ds.WordOrder, ds.MaxBlockSize); err != nil {
			return fmt.Errorf("device set %q: %w", ds.DevModel, err)
		}
		for i, a := range ds.Addresses {
			if _, err = tx.Exec(`INSERT INTO addresses(address_id,dev_set_id,address_fc,address_register,address_description,canonical_key,source_tag,address_factor,address_offset,address_data_type,address_length,word_order,source_unit,canonical_unit,address_remark,enabled) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				a.AddressID, ds.DeviceSetID, a.FunctionCode, a.Register, a.Description, a.CanonicalKey, a.SourceTag, a.Factor, a.Offset, a.DataType, a.Length, a.WordOrder, a.SourceUnit, a.CanonicalUnit, a.Remark, a.Enabled); err != nil {
				return fmt.Errorf("device set %q address %d: %w", ds.DevModel, i+1, err)
			}
		}
	}
	for _, p := range snapshot.Plants {
		if _, err = tx.Exec(`INSERT INTO plants(plant_code,plant_name) VALUES(?,?) ON CONFLICT(plant_code) DO UPDATE SET plant_name=excluded.plant_name`, p.PlantCode, p.PlantName); err != nil {
			return fmt.Errorf("plant %q: %w", p.PlantCode, err)
		}
	}
	for _, c := range snapshot.Connections {
		if _, err = tx.Exec(`INSERT INTO connections(con_id,con_name,con_host,con_port,unit_id,con_dev_set,dev_dn,device_name,plant_code,plant_name,enabled) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			c.ConnectionID, c.ConnectionName, c.Host, c.Port, c.UnitID, c.DeviceSetID, c.DevDn, c.DeviceName, c.PlantCode, c.PlantName, c.Enabled); err != nil {
			return fmt.Errorf("connection %q: %w", c.ConnectionName, err)
		}
	}
	return tx.Commit()
}
