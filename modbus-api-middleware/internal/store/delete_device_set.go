//go:build !mips && !mipsle

package store

import "fmt"

func (s *Store) DeleteDeviceSet(id int64) error {
	if id == 0 {
		return fmt.Errorf("deviceSetId is required")
	}
	var used int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM connections WHERE con_dev_set=?", id).Scan(&used); err != nil {
		return err
	}
	if used > 0 {
		return fmt.Errorf("device set is used by %d connection(s)", used)
	}
	r, err := s.DB.Exec("DELETE FROM device_sets WHERE dev_set_id=?", id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return fmt.Errorf("device set not found")
	}
	return nil
}
