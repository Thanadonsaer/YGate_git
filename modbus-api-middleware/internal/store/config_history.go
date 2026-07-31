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
// IDs inside snapshot (brandId, deviceSetId, ...) are treated purely as
// cross-reference keys within this one snapshot, never as literal local
// primary keys: every apply deletes and re-creates every row, letting
// SQLite assign fresh IDs. This sidesteps having to reconcile a
// Central-Platform-invented ID space with the local autoincrement space,
// and matches the "full snapshot, not diff" design -- the platform is
// never trying to patch specific rows, only replace everything.
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

	brandIDs := map[int64]int64{}
	for _, b := range snapshot.Brands {
		name := strings.TrimSpace(b.BrandName)
		if name == "" {
			return fmt.Errorf("brand: brandName is required")
		}
		res, err := tx.Exec(`INSERT INTO brands(brand_name,brand_description) VALUES(?,?)`, name, strings.TrimSpace(b.BrandDescription))
		if err != nil {
			return fmt.Errorf("brand %q: %w", name, err)
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		brandIDs[b.BrandID] = newID
	}

	deviceSetIDs := map[int64]int64{}
	for _, ds := range snapshot.DeviceSets {
		normalized := normalizeDeviceSet(ds)
		brandID, ok := brandIDs[ds.BrandID]
		if !ok {
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
		res, err := tx.Exec(`INSERT INTO device_sets(brand_id,dev_type_id,dev_type,dev_model,address_mode,byte_order,word_order,max_block_size) VALUES(?,?,?,?,?,?,?,?)`,
			brandID, normalized.DevTypeID, normalized.DevType, normalized.DevModel, normalized.AddressMode, normalized.ByteOrder, normalized.WordOrder, normalized.MaxBlockSize)
		if err != nil {
			return fmt.Errorf("device set %q: %w", normalized.DevModel, err)
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		deviceSetIDs[ds.DeviceSetID] = newID

		for i, addr := range ds.Addresses {
			a, err := normalizeAddress(addr, normalized.AddressMode)
			if err != nil {
				return fmt.Errorf("device set %q address %d: %w", normalized.DevModel, i+1, err)
			}
			if err = validateAddress(a, normalized.AddressMode); err != nil {
				return fmt.Errorf("device set %q address %d: %w", normalized.DevModel, i+1, err)
			}
			if _, err = tx.Exec(`INSERT INTO addresses(dev_set_id,address_fc,address_register,address_description,canonical_key,source_tag,address_factor,address_offset,address_data_type,address_length,word_order,source_unit,canonical_unit,address_remark,enabled) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				newID, a.FunctionCode, a.Register, a.Description, a.CanonicalKey, a.SourceTag, a.Factor, a.Offset, a.DataType, a.Length, a.WordOrder, a.SourceUnit, a.CanonicalUnit, a.Remark, a.Enabled); err != nil {
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
		devSetID, ok := deviceSetIDs[conn.DeviceSetID]
		if !ok {
			return fmt.Errorf("connection %q: unknown deviceSetId %d", normalized.ConnectionName, conn.DeviceSetID)
		}
		if strings.TrimSpace(normalized.ConnectionName) == "" || strings.TrimSpace(normalized.Host) == "" || normalized.Port < 1 || normalized.Port > 65535 {
			return fmt.Errorf("connection %q: connectionName, host and valid port are required", normalized.ConnectionName)
		}
		if _, err = tx.Exec(`INSERT INTO connections(con_name,con_host,con_port,unit_id,con_dev_set,dev_dn,device_name,plant_code,plant_name,enabled) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			normalized.ConnectionName, normalized.Host, normalized.Port, normalized.UnitID, devSetID, normalized.DevDn, normalized.DeviceName, normalized.PlantCode, normalized.PlantName, normalized.Enabled); err != nil {
			return fmt.Errorf("connection %q: %w", normalized.ConnectionName, err)
		}
	}

	return tx.Commit()
}
