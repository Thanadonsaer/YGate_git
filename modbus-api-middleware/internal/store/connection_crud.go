package store

import (
	"fmt"

	"chpp/modbus-api-middleware/internal/domain"
)

func (s *Store) DeleteBrand(id int64) error {
	if id == 0 {
		return fmt.Errorf("brandId is required")
	}
	var used int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM device_sets WHERE brand_id=?", id).Scan(&used); err != nil {
		return err
	}
	if used > 0 {
		return fmt.Errorf("brand is used by %d device set(s)", used)
	}
	r, err := s.DB.Exec("DELETE FROM brands WHERE brand_id=?", id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return fmt.Errorf("brand not found")
	}
	return nil
}

func (s *Store) DeleteConnection(id int64) error {
	if id == 0 {
		return fmt.Errorf("connectionId is required")
	}
	r, err := s.DB.Exec("DELETE FROM connections WHERE con_id=?", id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return fmt.Errorf("connection not found")
	}
	return nil
}

func (s *Store) SetConnectionEnabled(id int64, enabled bool) error {
	if id == 0 {
		return fmt.Errorf("connectionId is required")
	}
	r, err := s.DB.Exec("UPDATE connections SET enabled=? WHERE con_id=?", enabled, id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return fmt.Errorf("connection not found")
	}
	return nil
}

func (s *Store) ConnectionsWithStatus() ([]domain.ConnectionConfig, error) {
	rows, err := s.DB.Query(connectionSelect + ` ORDER BY c.con_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ConnectionConfig{}
	for rows.Next() {
		v, e := scanConnectionKeepingEnabled(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) EnabledConnections() ([]domain.ConnectionConfig, error) {
	rows, err := s.DB.Query(connectionSelect + ` WHERE c.enabled=1 ORDER BY c.con_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ConnectionConfig{}
	for rows.Next() {
		v, e := scanConnectionKeepingEnabled(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func scanConnectionKeepingEnabled(row interface{ Scan(...any) error }) (domain.ConnectionConfig, error) {
	var v domain.ConnectionConfig
	err := row.Scan(&v.ConnectionID, &v.ConnectionName, &v.Host, &v.Port, &v.UnitID, &v.DeviceSetID, &v.DeviceSetName, &v.DevTypeID, &v.DevDn, &v.DeviceName, &v.PlantCode, &v.PlantName, &v.Enabled)
	enabled := v.Enabled
	v = normalizeConnection(v)
	v.Enabled = enabled
	return v, err
}
