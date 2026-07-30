package store

import (
	"fmt"
	"strings"

	"chpp/modbus-api-middleware/internal/domain"
)

func (s *Store) SavePlant(v domain.Plant) (domain.Plant, error) {
	v.PlantCode = strings.ToUpper(strings.TrimSpace(v.PlantCode))
	v.PlantName = strings.TrimSpace(v.PlantName)
	if v.PlantCode == "" || v.PlantName == "" {
		return v, fmt.Errorf("plantCode and plantName are required")
	}
	if v.PlantID == 0 {
		err := s.DB.QueryRow(`INSERT INTO plants(plant_code,plant_name) VALUES(?,?) ON CONFLICT(plant_code) DO UPDATE SET plant_name=excluded.plant_name RETURNING plant_id`, v.PlantCode, v.PlantName).Scan(&v.PlantID)
		if err == nil {
			_, err = s.DB.Exec("UPDATE connections SET plant_name=? WHERE plant_code=?", v.PlantName, v.PlantCode)
		}
		return v, err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return v, err
	}
	defer tx.Rollback()
	var oldCode string
	if err = tx.QueryRow("SELECT plant_code FROM plants WHERE plant_id=?", v.PlantID).Scan(&oldCode); err != nil {
		return v, err
	}
	if _, err = tx.Exec("UPDATE plants SET plant_code=?,plant_name=? WHERE plant_id=?", v.PlantCode, v.PlantName, v.PlantID); err != nil {
		return v, err
	}
	if _, err = tx.Exec("UPDATE connections SET plant_code=?,plant_name=? WHERE plant_code=?", v.PlantCode, v.PlantName, oldCode); err != nil {
		return v, err
	}
	return v, tx.Commit()
}

func (s *Store) Plants() ([]domain.Plant, error) {
	rows, err := s.DB.Query("SELECT plant_id,plant_code,plant_name FROM plants ORDER BY plant_code")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Plant{}
	for rows.Next() {
		var v domain.Plant
		if err = rows.Scan(&v.PlantID, &v.PlantCode, &v.PlantName); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) PlantByCode(code string) (domain.Plant, error) {
	var v domain.Plant
	err := s.DB.QueryRow("SELECT plant_id,plant_code,plant_name FROM plants WHERE plant_code=?", strings.ToUpper(strings.TrimSpace(code))).Scan(&v.PlantID, &v.PlantCode, &v.PlantName)
	return v, err
}

func (s *Store) DeletePlant(id int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var code string
	if err = tx.QueryRow("SELECT plant_code FROM plants WHERE plant_id=?", id).Scan(&code); err != nil {
		return fmt.Errorf("plant not found")
	}
	if _, err = tx.Exec("DELETE FROM connections WHERE plant_code=?", code); err != nil {
		return err
	}
	r, err := tx.Exec("DELETE FROM plants WHERE plant_id=?", id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return fmt.Errorf("plant not found")
	}
	return tx.Commit()
}
