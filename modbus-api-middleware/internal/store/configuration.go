//go:build !mips && !mipsle

package store

import (
	"fmt"
	"strings"

	"chpp/modbus-api-middleware/internal/domain"
)

const configurationSchema = `
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS brands (
 brand_id INTEGER PRIMARY KEY AUTOINCREMENT,
 brand_name TEXT NOT NULL UNIQUE,
 brand_description TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS device_sets (
 dev_set_id INTEGER PRIMARY KEY AUTOINCREMENT,
 brand_id INTEGER NOT NULL REFERENCES brands(brand_id) ON DELETE RESTRICT,
 dev_type_id INTEGER NOT NULL,
 dev_type TEXT NOT NULL,
 dev_model TEXT NOT NULL,
 address_mode TEXT NOT NULL DEFAULT 'ZERO_BASED',
 byte_order TEXT NOT NULL DEFAULT 'BIG_ENDIAN',
 word_order TEXT NOT NULL DEFAULT 'HIGH_LOW',
 max_block_size INTEGER NOT NULL DEFAULT 30,
 UNIQUE(brand_id, dev_type_id, dev_model)
);
CREATE TABLE IF NOT EXISTS addresses (
 address_id INTEGER PRIMARY KEY AUTOINCREMENT,
 dev_set_id INTEGER NOT NULL REFERENCES device_sets(dev_set_id) ON DELETE CASCADE,
 address_fc INTEGER NOT NULL,
 address_register INTEGER NOT NULL,
 address_description TEXT NOT NULL,
 canonical_key TEXT NOT NULL,
 source_tag TEXT NOT NULL DEFAULT '',
 address_factor REAL NOT NULL DEFAULT 1,
 address_offset REAL NOT NULL DEFAULT 0,
 address_data_type TEXT NOT NULL,
 address_length INTEGER NOT NULL DEFAULT 1,
 word_order TEXT NOT NULL DEFAULT '',
 source_unit TEXT NOT NULL DEFAULT '',
 canonical_unit TEXT NOT NULL DEFAULT '',
 address_remark TEXT NOT NULL DEFAULT '',
 enabled INTEGER NOT NULL DEFAULT 1,
 UNIQUE(dev_set_id, address_fc, address_register, canonical_key)
);
CREATE TABLE IF NOT EXISTS plants (
 plant_id INTEGER PRIMARY KEY AUTOINCREMENT,
 plant_code TEXT NOT NULL UNIQUE,
 plant_name TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS connections (
 con_id INTEGER PRIMARY KEY AUTOINCREMENT,
 con_name TEXT NOT NULL UNIQUE,
 con_host TEXT NOT NULL,
 con_port INTEGER NOT NULL,
 unit_id INTEGER NOT NULL DEFAULT 1,
 con_dev_set INTEGER NOT NULL REFERENCES device_sets(dev_set_id) ON DELETE RESTRICT,
 dev_dn TEXT NOT NULL UNIQUE,
 device_name TEXT NOT NULL DEFAULT '',
 plant_code TEXT NOT NULL,
 plant_name TEXT NOT NULL DEFAULT '',
 poll_interval_seconds INTEGER NOT NULL DEFAULT 10,
 publish_interval_seconds INTEGER NOT NULL DEFAULT 60,
 enabled INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS addresses_dev_set ON addresses(dev_set_id, address_fc, address_register);
CREATE INDEX IF NOT EXISTS connections_dev_set ON connections(con_dev_set);
INSERT OR IGNORE INTO plants(plant_code,plant_name)
 SELECT plant_code,MAX(plant_name) FROM connections WHERE plant_code<>'' GROUP BY plant_code;`

func openNormalized(path string) (*Store, error) {
	s, err := Open(path)
	if err != nil {
		return nil, err
	}
	if _, err = s.DB.Exec(configurationSchema); err != nil {
		s.Close()
		return nil, err
	}
	if err = s.ensureColumn("device_sets", "max_block_size", "INTEGER NOT NULL DEFAULT 30"); err != nil {
		s.Close()
		return nil, err
	}
	for _, column := range []string{"source_unit", "canonical_unit"} {
		if err = s.ensureColumn("addresses", column, "TEXT NOT NULL DEFAULT ''"); err != nil {
			s.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) SaveBrand(v domain.Brand) (domain.Brand, error) {
	v.BrandName = strings.TrimSpace(v.BrandName)
	v.BrandDescription = strings.TrimSpace(v.BrandDescription)
	if v.BrandName == "" {
		return v, fmt.Errorf("brandName is required")
	}
	var err error
	if v.BrandID == 0 {
		err = s.DB.QueryRow(`
INSERT INTO brands(brand_name,brand_description) VALUES(?,?)
ON CONFLICT(brand_name) DO UPDATE SET brand_description=excluded.brand_description
RETURNING brand_id`, v.BrandName, v.BrandDescription).Scan(&v.BrandID)
		if err != nil {
			return v, err
		}
	} else if _, err = s.DB.Exec("UPDATE brands SET brand_name=?,brand_description=? WHERE brand_id=?", v.BrandName, v.BrandDescription, v.BrandID); err != nil {
		return v, err
	}
	v.BrandDevSetIDList, err = s.intList("SELECT dev_set_id FROM device_sets WHERE brand_id=? ORDER BY dev_set_id", v.BrandID)
	return v, err
}

func (s *Store) Brands() ([]domain.Brand, error) {
	rows, err := s.DB.Query("SELECT brand_id,brand_name,brand_description FROM brands ORDER BY brand_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Brand{}
	for rows.Next() {
		var v domain.Brand
		if err = rows.Scan(&v.BrandID, &v.BrandName, &v.BrandDescription); err != nil {
			return nil, err
		}
		v.BrandDevSetIDList, err = s.intList("SELECT dev_set_id FROM device_sets WHERE brand_id=? ORDER BY dev_set_id", v.BrandID)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) SaveDeviceSet(v domain.DeviceSet) (domain.DeviceSet, error) {
	v = normalizeDeviceSet(v)
	if v.BrandID == 0 || strings.TrimSpace(v.DevType) == "" || strings.TrimSpace(v.DevModel) == "" {
		return v, fmt.Errorf("brandId, devType and devModel are required")
	}

	if !oneOf(v.ByteOrder, "BIG_ENDIAN", "LITTLE_ENDIAN") {
		return v, fmt.Errorf("byteOrder must be BIG_ENDIAN or LITTLE_ENDIAN")
	}
	if !oneOf(v.WordOrder, "HIGH_LOW", "LOW_HIGH") {
		return v, fmt.Errorf("wordOrder must be HIGH_LOW or LOW_HIGH")
	}
	if v.MaxBlockSize < 1 || v.MaxBlockSize > 125 {
		return v, fmt.Errorf("maxBlockSize must be 1..125")
	}
	if len(v.Addresses) == 0 {
		return v, fmt.Errorf("at least one address is required")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return v, err
	}
	defer tx.Rollback()
	if v.DeviceSetID == 0 {
		r, e := tx.Exec("INSERT INTO device_sets(brand_id,dev_type_id,dev_type,dev_model,address_mode,byte_order,word_order,max_block_size) VALUES(?,?,?,?,?,?,?,?)", v.BrandID, v.DevTypeID, v.DevType, v.DevModel, v.AddressMode, v.ByteOrder, v.WordOrder, v.MaxBlockSize)
		if e != nil {
			return v, e
		}
		v.DeviceSetID, e = r.LastInsertId()
		if e != nil {
			return v, e
		}
	} else {
		if _, err = tx.Exec("UPDATE device_sets SET brand_id=?,dev_type_id=?,dev_type=?,dev_model=?,address_mode=?,byte_order=?,word_order=?,max_block_size=? WHERE dev_set_id=?", v.BrandID, v.DevTypeID, v.DevType, v.DevModel, v.AddressMode, v.ByteOrder, v.WordOrder, v.MaxBlockSize, v.DeviceSetID); err != nil {
			return v, err
		}
		if _, err = tx.Exec("DELETE FROM addresses WHERE dev_set_id=?", v.DeviceSetID); err != nil {
			return v, err
		}
	}
	for i := range v.Addresses {
		a, e := normalizeAddress(v.Addresses[i], v.AddressMode)
		if e != nil {
			return v, fmt.Errorf("address %d: %w", i+1, e)
		}
		a.DeviceSetID = v.DeviceSetID
		if e = validateAddress(a, v.AddressMode); e != nil {
			return v, fmt.Errorf("address %d: %w", i+1, e)
		}
		r, e := tx.Exec(`INSERT INTO addresses(dev_set_id,address_fc,address_register,address_description,canonical_key,source_tag,address_factor,address_offset,address_data_type,address_length,word_order,source_unit,canonical_unit,address_remark,enabled) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, v.DeviceSetID, a.FunctionCode, a.Register, a.Description, a.CanonicalKey, a.SourceTag, a.Factor, a.Offset, a.DataType, a.Length, a.WordOrder, a.SourceUnit, a.CanonicalUnit, a.Remark, a.Enabled)
		if e != nil {
			return v, e
		}
		a.AddressID, _ = r.LastInsertId()
		v.Addresses[i] = a
	}
	if err = tx.Commit(); err != nil {
		return v, err
	}
	v.AddressIDList = addressIDList(v.Addresses)
	return v, nil
}





func scanDeviceSet(row interface{ Scan(...any) error }) (domain.DeviceSet, error) {
	var v domain.DeviceSet
	err := row.Scan(&v.DeviceSetID, &v.BrandID, &v.BrandName, &v.DevTypeID, &v.DevType, &v.DevModel, &v.AddressMode, &v.ByteOrder, &v.WordOrder, &v.MaxBlockSize)
	return v, err
}

func (s *Store) DeviceSets() ([]domain.DeviceSet, error) {
	rows, err := s.DB.Query(`SELECT d.dev_set_id,d.brand_id,b.brand_name,d.dev_type_id,d.dev_type,d.dev_model,d.address_mode,d.byte_order,d.word_order,d.max_block_size FROM device_sets d JOIN brands b ON b.brand_id=d.brand_id ORDER BY b.brand_name,d.dev_model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.DeviceSet{}
	for rows.Next() {
		v, e := scanDeviceSet(rows)
		if e != nil {
			return nil, e
		}
		v.Addresses, e = s.Addresses(v.DeviceSetID)
		if e != nil {
			return nil, e
		}
		v.AddressIDList = addressIDList(v.Addresses)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) DeviceSet(id int64) (domain.DeviceSet, error) {
	v, err := scanDeviceSet(s.DB.QueryRow(`SELECT d.dev_set_id,d.brand_id,b.brand_name,d.dev_type_id,d.dev_type,d.dev_model,d.address_mode,d.byte_order,d.word_order,d.max_block_size FROM device_sets d JOIN brands b ON b.brand_id=d.brand_id WHERE d.dev_set_id=?`, id))
	if err != nil {
		return v, err
	}
	v.Addresses, err = s.Addresses(id)
	v.AddressIDList = addressIDList(v.Addresses)
	return v, err
}

func (s *Store) Addresses(deviceSetID int64) ([]domain.Address, error) {
	rows, err := s.DB.Query(`SELECT address_id,dev_set_id,address_fc,address_register,address_description,canonical_key,source_tag,address_factor,address_offset,address_data_type,address_length,word_order,source_unit,canonical_unit,address_remark,enabled FROM addresses WHERE dev_set_id=? ORDER BY address_fc,address_register,address_id`, deviceSetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Address{}
	for rows.Next() {
		var a domain.Address
		if err = rows.Scan(&a.AddressID, &a.DeviceSetID, &a.FunctionCode, &a.Register, &a.Description, &a.CanonicalKey, &a.SourceTag, &a.Factor, &a.Offset, &a.DataType, &a.Length, &a.WordOrder, &a.SourceUnit, &a.CanonicalUnit, &a.Remark, &a.Enabled); err != nil {
			return nil, err
		}
		a.EnabledSet = true
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) SaveConnection(v domain.ConnectionConfig) (domain.ConnectionConfig, error) {
	v = normalizeConnection(v)
	if strings.TrimSpace(v.ConnectionName) == "" || strings.TrimSpace(v.Host) == "" || v.Port < 1 || v.Port > 65535 || v.UnitID < 0 || v.UnitID > 255 || v.DeviceSetID == 0 {
		return v, fmt.Errorf("connectionName, host, valid port/unitId and deviceSetId are required")
	}
	if v.ConnectionID == 0 {
		r, err := s.DB.Exec(`INSERT INTO connections(con_name,con_host,con_port,unit_id,con_dev_set,dev_dn,device_name,plant_code,plant_name,enabled) VALUES(?,?,?,?,?,?,?,?,?,?)`, v.ConnectionName, v.Host, v.Port, v.UnitID, v.DeviceSetID, v.DevDn, v.DeviceName, v.PlantCode, v.PlantName, v.Enabled)
		if err != nil {
			return v, err
		}
		v.ConnectionID, err = r.LastInsertId()
		return v, err
	}
	r, err := s.DB.Exec(`UPDATE connections SET con_name=?,con_host=?,con_port=?,unit_id=?,con_dev_set=?,dev_dn=?,device_name=?,plant_code=?,plant_name=?,enabled=? WHERE con_id=?`, v.ConnectionName, v.Host, v.Port, v.UnitID, v.DeviceSetID, v.DevDn, v.DeviceName, v.PlantCode, v.PlantName, v.Enabled, v.ConnectionID)
	if err != nil {
		return v, err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return v, fmt.Errorf("connection not found")
	}
	return v, nil
}


func scanConnection(row interface{ Scan(...any) error }) (domain.ConnectionConfig, error) {
	var v domain.ConnectionConfig
	err := row.Scan(&v.ConnectionID, &v.ConnectionName, &v.Host, &v.Port, &v.UnitID, &v.DeviceSetID, &v.DeviceSetName, &v.DevTypeID, &v.DevDn, &v.DeviceName, &v.PlantCode, &v.PlantName, &v.Enabled)
	return normalizeConnection(v), err
}

const connectionSelect = `SELECT c.con_id,c.con_name,c.con_host,c.con_port,c.unit_id,c.con_dev_set,b.brand_name||' '||d.dev_model,d.dev_type_id,c.dev_dn,c.device_name,c.plant_code,c.plant_name,c.enabled FROM connections c JOIN device_sets d ON d.dev_set_id=c.con_dev_set JOIN brands b ON b.brand_id=d.brand_id`

func (s *Store) Connections() ([]domain.ConnectionConfig, error) {
	rows, err := s.DB.Query(connectionSelect + ` ORDER BY c.con_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ConnectionConfig{}
	for rows.Next() {
		v, e := scanConnection(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Connection(id int64) (domain.ConnectionConfig, error) {
	return scanConnection(s.DB.QueryRow(connectionSelect+` WHERE c.con_id=?`, id))
}

func (s *Store) intList(query string, args ...any) ([]int64, error) {
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var v int64
		if err = rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}






