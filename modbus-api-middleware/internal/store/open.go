package store

func OpenNormalized(path string) (*Store, error) {
	s, err := openNormalized(path)
	if err != nil {
		return nil, err
	}
	if _, err = s.DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS addresses_one_value_per_kpi ON addresses(dev_set_id,canonical_key)"); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
