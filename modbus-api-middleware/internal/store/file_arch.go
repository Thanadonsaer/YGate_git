//go:build mips || mipsle

package store

import (
	"chpp/modbus-api-middleware/internal/decoder"
	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/profile"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// The MIPS build uses a small JSON document instead of SQLite.  It deliberately
// keeps the Store API identical so the middleware, web UI and config transfer
// code do not need an RUT906-specific path.
type Store struct {
	path  string
	mu    sync.Mutex
	state fileState
}
type OutboxEvent struct {
	ID       int64
	Payload  string
	Attempts int
}
type fileState struct {
	FormatVersion    int                               `json:"formatVersion"`
	NextID           int64                             `json:"nextId"`
	GatewayConfig    domain.GatewayConfig              `json:"gatewayConfig"`
	GatewayConfigSet bool                              `json:"gatewayConfigSet"`
	Brands           []domain.Brand                    `json:"brands"`
	DeviceSets       []domain.DeviceSet                `json:"deviceSets"`
	Plants           []domain.Plant                    `json:"plants"`
	Connections      []domain.ConnectionConfig         `json:"connections"`
	Profiles         map[string]domain.RegisterProfile `json:"profiles"`
	Devices          map[string]domain.Device          `json:"devices"`
	Outbox           []fileOutbox                      `json:"outbox"`
	PollLogs         []domain.PollLog                  `json:"pollLogs"`
	History          []fileHistory                     `json:"history"`
}
type fileOutbox struct {
	ID                            int64
	Key, Hash, Payload, Status    string
	Attempts, HTTPStatus          int
	NextRetry, Created, Delivered int64
	LastError, LastResponse       string
}
type fileHistory struct {
	Version                  int64
	Status, Reason, Snapshot string
	AppliedAt                int64
}

func Open(path string) (*Store, error)           { return openNormalized(path) }
func OpenNormalized(path string) (*Store, error) { return openNormalized(path) }
func openNormalized(path string) (*Store, error) {
	if strings.HasSuffix(strings.ToLower(path), ".db") {
		return nil, errors.New("SQLite middleware.db is not supported on MIPS; use middleware.store or export/import settings")
	}
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(b, &s.state); err != nil {
			return nil, fmt.Errorf("read middleware store: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if s.state.FormatVersion == 0 {
		s.state.FormatVersion = 1
	}
	if s.state.NextID < 1 {
		s.state.NextID = 1
	}
	if s.state.Profiles == nil {
		s.state.Profiles = map[string]domain.RegisterProfile{}
	}
	if s.state.Devices == nil {
		s.state.Devices = map[string]domain.Device{}
	}
	return s, nil
}
func (s *Store) Close() error { return nil }
func (s *Store) saveLocked() error {
	dir := filepath.Dir(s.path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
func (s *Store) nextID() int64 { id := s.state.NextID; s.state.NextID++; return id }
func (s *Store) SaveProfile(v domain.RegisterProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Profiles[v.ProfileID] = v
	return s.saveLocked()
}
func (s *Store) SaveDevice(v domain.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Devices[v.DeviceID] = v
	return s.saveLocked()
}
func (s *Store) Profile(id string) (domain.RegisterProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.state.Profiles[id]
	if !ok {
		return v, os.ErrNotExist
	}
	return v, nil
}
func (s *Store) Device(id string) (domain.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.state.Devices[id]
	if !ok {
		return v, os.ErrNotExist
	}
	return v, nil
}

func (s *Store) SaveBrand(v domain.Brand) (domain.Brand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.BrandName = strings.TrimSpace(v.BrandName)
	v.BrandDescription = strings.TrimSpace(v.BrandDescription)
	if v.BrandName == "" {
		return v, fmt.Errorf("brandName is required")
	}
	for i := range s.state.Brands {
		if s.state.Brands[i].BrandName == v.BrandName && v.BrandID == 0 {
			v.BrandID = s.state.Brands[i].BrandID
		}
		if s.state.Brands[i].BrandID == v.BrandID && v.BrandID != 0 {
			s.state.Brands[i] = v
			v.BrandDevSetIDList = s.brandSets(v.BrandID)
			return v, s.saveLocked()
		}
	}
	v.BrandID = s.nextID()
	s.state.Brands = append(s.state.Brands, v)
	v.BrandDevSetIDList = s.brandSets(v.BrandID)
	return v, s.saveLocked()
}
func (s *Store) brandSets(id int64) []int64 {
	var r []int64
	for _, v := range s.state.DeviceSets {
		if v.BrandID == id {
			r = append(r, v.DeviceSetID)
		}
	}
	return r
}
func (s *Store) Brands() ([]domain.Brand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := append([]domain.Brand(nil), s.state.Brands...)
	sort.Slice(r, func(i, j int) bool { return r[i].BrandName < r[j].BrandName })
	for i := range r {
		r[i].BrandDevSetIDList = s.brandSets(r[i].BrandID)
	}
	return r, nil
}
func normalizeDeviceSet(v domain.DeviceSet) domain.DeviceSet {
	v.DevType = strings.TrimSpace(v.DevType)
	v.DevModel = strings.TrimSpace(v.DevModel)
	if v.DevType == "" && v.DevTypeID > 0 {
		v.DevType = devTypeName(v.DevTypeID)
	}
	if v.DevTypeID <= 0 {
		v.DevTypeID = devTypeID(v.DevType)
	}
	if v.AddressMode == "" {
		v.AddressMode = "ZERO_BASED"
	}
	if m, e := profile.CanonicalAddressMode(v.AddressMode); e == nil {
		v.AddressMode = m
	} else {
		v.AddressMode = strings.ToUpper(strings.TrimSpace(v.AddressMode))
	}
	if v.ByteOrder == "" {
		v.ByteOrder = "BIG_ENDIAN"
	}
	if v.WordOrder == "" {
		v.WordOrder = "HIGH_LOW"
	}
	v.ByteOrder = strings.ToUpper(strings.TrimSpace(v.ByteOrder))
	v.WordOrder = strings.ToUpper(strings.TrimSpace(v.WordOrder))
	if v.MaxBlockSize < 1 {
		v.MaxBlockSize = 30
	}
	return v
}
func normalizeAddress(a domain.Address, mode string) (domain.Address, error) {
	a.Description = strings.TrimSpace(a.Description)
	a.DataType = strings.ToUpper(strings.TrimSpace(a.DataType))
	a.SourceUnit = strings.TrimSpace(a.SourceUnit)
	a.CanonicalUnit = strings.TrimSpace(a.CanonicalUnit)
	a.Remark = strings.TrimSpace(a.Remark)
	if mode == "ZERO_BASED" {
		a.FunctionCode, a.Register = normalizeRegister(a.FunctionCode, a.Register)
	}
	if a.Factor == 0 {
		a.Factor = 1
	}
	if strings.TrimSpace(a.CanonicalKey) == "" {
		a.CanonicalKey = fmt.Sprintf("%d:%d", a.FunctionCode, a.Register)
	}
	if a.SourceTag == "" {
		a.SourceTag = a.Description
	}
	if a.Length == 0 {
		a.Length = decoder.RegisterCount(a.DataType)
	}
	a.WordOrder = strings.ToUpper(strings.TrimSpace(a.WordOrder))
	if !a.EnabledSet {
		a.Enabled = true
	}
	return a, nil
}
func normalizeRegister(fc, r int) (int, int) {
	switch {
	case r >= 30000 && r < 40000:
		return 3, r - 30000
	case r >= 40000 && r < 50000:
		return 4, r - 40000
	default:
		return fc, r
	}
}
func oneOf(v string, a ...string) bool {
	for _, x := range a {
		if v == x {
			return true
		}
	}
	return false
}
func validateAddress(a domain.Address, mode string) error {
	if a.FunctionCode != 3 && a.FunctionCode != 4 {
		return fmt.Errorf("functionCode must be 3 or 4")
	}
	if a.Register < 0 || a.Register > 65535 {
		return fmt.Errorf("register must be 0..65535")
	}
	if a.Description == "" || a.DataType == "" {
		return fmt.Errorf("description and dataType are required")
	}
	if decoder.RegisterCount(a.DataType) == 0 {
		return fmt.Errorf("unsupported dataType %q", a.DataType)
	}
	if a.Length < 1 || a.Length > 4 {
		return fmt.Errorf("length must be 1..4")
	}
	if a.WordOrder != "" && !oneOf(a.WordOrder, "HIGH_LOW", "LOW_HIGH") {
		return fmt.Errorf("wordOrder must be HIGH_LOW or LOW_HIGH")
	}
	_, err := profile.ResolveModbusAddress(mode, domain.RegisterDefinition{Key: a.Description, FunctionCode: a.FunctionCode, RegisterAddress: a.Register, Length: a.Length})
	return err
}
func addressIDList(a []domain.Address) []int64 {
	r := make([]int64, 0, len(a))
	for _, v := range a {
		r = append(r, v.AddressID)
	}
	return r
}
func (s *Store) SaveDeviceSet(v domain.DeviceSet) (domain.DeviceSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v = normalizeDeviceSet(v)
	if v.BrandID == 0 || v.DevType == "" || v.DevModel == "" {
		return v, fmt.Errorf("brandId, devType and devModel are required")
	}
	if !oneOf(v.ByteOrder, "BIG_ENDIAN", "LITTLE_ENDIAN") || !oneOf(v.WordOrder, "HIGH_LOW", "LOW_HIGH") {
		return v, fmt.Errorf("invalid byteOrder or wordOrder")
	}
	if v.MaxBlockSize < 1 || v.MaxBlockSize > 125 {
		return v, fmt.Errorf("maxBlockSize must be 1..125")
	}
	if len(v.Addresses) == 0 {
		return v, fmt.Errorf("at least one address is required")
	}
	if v.DeviceSetID == 0 {
		v.DeviceSetID = s.nextID()
	}
	for i := range v.Addresses {
		a, e := normalizeAddress(v.Addresses[i], v.AddressMode)
		if e != nil {
			return v, e
		}
		a.DeviceSetID = v.DeviceSetID
		if e = validateAddress(a, v.AddressMode); e != nil {
			return v, e
		}
		if a.AddressID == 0 {
			a.AddressID = s.nextID()
		}
		v.Addresses[i] = a
	}
	v.AddressIDList = addressIDList(v.Addresses)
	found := false
	for i := range s.state.DeviceSets {
		if s.state.DeviceSets[i].DeviceSetID == v.DeviceSetID {
			s.state.DeviceSets[i] = v
			found = true
		}
	}
	if !found {
		s.state.DeviceSets = append(s.state.DeviceSets, v)
	}
	return v, s.saveLocked()
}
func (s *Store) DeviceSets() ([]domain.DeviceSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := append([]domain.DeviceSet(nil), s.state.DeviceSets...)
	for i := range r {
		r[i].Addresses = append([]domain.Address(nil), r[i].Addresses...)
		r[i].AddressIDList = addressIDList(r[i].Addresses)
	}
	return r, nil
}
func (s *Store) DeviceSet(id int64) (domain.DeviceSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.state.DeviceSets {
		if v.DeviceSetID == id {
			return v, nil
		}
	}
	return domain.DeviceSet{}, os.ErrNotExist
}
func (s *Store) Addresses(id int64) ([]domain.Address, error) {
	v, e := s.DeviceSet(id)
	return v.Addresses, e
}

func normalizeConnection(v domain.ConnectionConfig) domain.ConnectionConfig {
	v.ConnectionName = strings.TrimSpace(v.ConnectionName)
	v.Host = strings.TrimSpace(v.Host)
	if v.UnitID <= 0 && v.SlaveID > 0 {
		v.UnitID = v.SlaveID
	}
	if v.UnitID <= 0 {
		v.UnitID = 1
	}
	v.SlaveID = v.UnitID
	if v.DevDn == "" {
		v.DevDn = v.ConnectionName
	}
	if v.DeviceName == "" {
		v.DeviceName = v.ConnectionName
	}
	if v.PlantCode == "" {
		v.PlantCode = plantFromConnectionName(v.ConnectionName)
	}
	if v.PlantName == "" {
		v.PlantName = v.PlantCode
	}
	if v.ConnectionID == 0 {
		v.Enabled = true
	}
	return v
}
func plantFromConnectionName(v string) string {
	h, _, ok := strings.Cut(strings.TrimSpace(v), "-")
	if !ok || strings.TrimSpace(h) == "" {
		return "DEFAULT"
	}
	return strings.ToUpper(strings.TrimSpace(h))
}
func devTypeID(v string) int {
	v = strings.ToLower(v)
	if strings.Contains(v, "grid") || strings.Contains(v, "meter") {
		return 17
	}
	return 1
}
func devTypeName(id int) string {
	if id == 17 {
		return "Grid-Meter"
	}
	return "Inverter"
}
func (s *Store) SaveConnection(v domain.ConnectionConfig) (domain.ConnectionConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v = normalizeConnection(v)
	if v.ConnectionName == "" || v.Host == "" || v.Port < 1 || v.Port > 65535 || v.UnitID < 0 || v.UnitID > 255 || v.DeviceSetID == 0 {
		return v, fmt.Errorf("connectionName, host, valid port/unitId and deviceSetId are required")
	}
	if v.ConnectionID == 0 {
		v.ConnectionID = s.nextID()
		s.state.Connections = append(s.state.Connections, v)
	} else {
		ok := false
		for i := range s.state.Connections {
			if s.state.Connections[i].ConnectionID == v.ConnectionID {
				s.state.Connections[i] = v
				ok = true
			}
		}
		if !ok {
			return v, fmt.Errorf("connection not found")
		}
	}
	return v, s.saveLocked()
}
func (s *Store) enrich(v domain.ConnectionConfig) domain.ConnectionConfig {
	for _, d := range s.state.DeviceSets {
		if d.DeviceSetID == v.DeviceSetID {
			v.DeviceSetName = firstBrand(s.state.Brands, d.BrandID) + " " + d.DevModel
			v.DevTypeID = d.DevTypeID
		}
	}
	return normalizeConnection(v)
}
func firstBrand(bs []domain.Brand, id int64) string {
	for _, b := range bs {
		if b.BrandID == id {
			return b.BrandName
		}
	}
	return ""
}
func (s *Store) Connections() ([]domain.ConnectionConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := append([]domain.ConnectionConfig(nil), s.state.Connections...)
	for i := range r {
		r[i] = s.enrich(r[i])
	}
	sort.Slice(r, func(i, j int) bool { return r[i].ConnectionName < r[j].ConnectionName })
	return r, nil
}
func (s *Store) Connection(id int64) (domain.ConnectionConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.state.Connections {
		if v.ConnectionID == id {
			return s.enrich(v), nil
		}
	}
	return domain.ConnectionConfig{}, os.ErrNotExist
}

func (s *Store) DeleteBrand(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.state.DeviceSets {
		if d.BrandID == id {
			return fmt.Errorf("brand is used by device set")
		}
	}
	for i, b := range s.state.Brands {
		if b.BrandID == id {
			s.state.Brands = append(s.state.Brands[:i], s.state.Brands[i+1:]...)
			return s.saveLocked()
		}
	}
	return nil
}
func (s *Store) DeleteDeviceSet(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.state.Connections {
		if c.DeviceSetID == id {
			return fmt.Errorf("device set is used by connection")
		}
	}
	for i, d := range s.state.DeviceSets {
		if d.DeviceSetID == id {
			s.state.DeviceSets = append(s.state.DeviceSets[:i], s.state.DeviceSets[i+1:]...)
			return s.saveLocked()
		}
	}
	return nil
}
func (s *Store) DeleteConnection(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.state.Connections {
		if c.ConnectionID == id {
			s.state.Connections = append(s.state.Connections[:i], s.state.Connections[i+1:]...)
			return s.saveLocked()
		}
	}
	return nil
}
func (s *Store) SetConnectionEnabled(id int64, e bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Connections {
		if s.state.Connections[i].ConnectionID == id {
			s.state.Connections[i].Enabled = e
			return s.saveLocked()
		}
	}
	return fmt.Errorf("connection not found")
}
func (s *Store) ConnectionsWithStatus() ([]domain.ConnectionConfig, error) { return s.Connections() }
func (s *Store) EnabledConnections() ([]domain.ConnectionConfig, error) {
	r, e := s.Connections()
	if e != nil {
		return nil, e
	}
	o := r[:0]
	for _, v := range r {
		if v.Enabled {
			o = append(o, v)
		}
	}
	return o, nil
}

func (s *Store) SavePlant(v domain.Plant) (domain.Plant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.PlantCode = strings.TrimSpace(v.PlantCode)
	v.PlantName = strings.TrimSpace(v.PlantName)
	if v.PlantCode == "" || v.PlantName == "" {
		return v, fmt.Errorf("plantCode and plantName are required")
	}
	if v.PlantID == 0 {
		v.PlantID = s.nextID()
		s.state.Plants = append(s.state.Plants, v)
	} else {
		for i := range s.state.Plants {
			if s.state.Plants[i].PlantID == v.PlantID {
				s.state.Plants[i] = v
			}
		}
	}
	return v, s.saveLocked()
}
func (s *Store) Plants() ([]domain.Plant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.Plant(nil), s.state.Plants...), nil
}
func (s *Store) PlantByCode(c string) (domain.Plant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.state.Plants {
		if v.PlantCode == c {
			return v, nil
		}
	}
	return domain.Plant{}, os.ErrNotExist
}
func (s *Store) DeletePlant(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, v := range s.state.Plants {
		if v.PlantID == id {
			s.state.Plants = append(s.state.Plants[:i], s.state.Plants[i+1:]...)
			return s.saveLocked()
		}
	}
	return nil
}

func (s *Store) GatewayConfig() (domain.GatewayConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.state.GatewayConfigSet {
		// A new file store has no row yet. SQLite's gateway config path also
		// starts from an empty/default value so the service can boot and the
		// Web UI can save the configuration.
		return domain.GatewayConfig{}, nil
	}
	return s.state.GatewayConfig, nil
}
func (s *Store) SaveGatewayConfig(v domain.GatewayConfig) (domain.GatewayConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(v.Endpoint) != "" {
		u, e := url.Parse(strings.TrimSpace(v.Endpoint))
		if e != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
			return v, fmt.Errorf("endpoint must be a valid http/https URL")
		}
	}
	if v.SendIntervalSeconds < 1 {
		v.SendIntervalSeconds = 5
	}
	if v.SendTimeoutSeconds < 1 {
		v.SendTimeoutSeconds = 10
	}
	if v.IdleHeartbeatSeconds < 60 {
		v.IdleHeartbeatSeconds = 1800
	}
	s.state.GatewayConfig = v
	s.state.GatewayConfigSet = true
	return v, s.saveLocked()
}

func (s *Store) Enqueue(key, hash string, r domain.Reading) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.state.Outbox {
		if v.Key == key {
			return false, nil
		}
	}
	b, e := json.Marshal(r)
	if e != nil {
		return false, e
	}
	s.state.Outbox = append(s.state.Outbox, fileOutbox{ID: s.nextID(), Key: key, Hash: hash, Payload: string(b), Status: "PENDING", Created: time.Now().UnixMilli()})
	return true, s.saveLocked()
}
func (s *Store) Ready(limit int) ([]OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	var r []OutboxEvent
	for _, v := range s.state.Outbox {
		if (v.Status == "PENDING" || v.Status == "RETRYING") && v.NextRetry <= now {
			r = append(r, OutboxEvent{v.ID, v.Payload, v.Attempts})
			if len(r) >= limit {
				break
			}
		}
	}
	return r, nil
}
func (s *Store) Delivered(ids []int64) error { return s.DeliveredWithResponse(ids, 0, "") }
func (s *Store) DeliveredWithResponse(ids []int64, status int, response string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Outbox {
		for _, id := range ids {
			if s.state.Outbox[i].ID == id {
				s.state.Outbox[i].Status = "DELIVERED"
				s.state.Outbox[i].HTTPStatus = status
				s.state.Outbox[i].LastResponse = response
				s.state.Outbox[i].Delivered = time.Now().UnixMilli()
			}
		}
	}
	return s.saveLocked()
}
func (s *Store) Failed(ids []int64, status int, msg string, retry bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Outbox {
		for _, id := range ids {
			if s.state.Outbox[i].ID == id {
				v := &s.state.Outbox[i]
				v.Attempts++
				v.HTTPStatus = status
				v.LastError = msg
				v.LastResponse = msg
				if retry {
					v.Status = "RETRYING"
					v.NextRetry = time.Now().Add(time.Second * time.Duration(1<<min(v.Attempts-1, 8))).UnixMilli()
				} else {
					v.Status = "DEAD_LETTER"
				}
			}
		}
	}
	return s.saveLocked()
}
func (s *Store) DeliveryLogs(limit int) ([]domain.DeliveryLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	r := []domain.DeliveryLog{}
	for i := len(s.state.Outbox) - 1; i >= 0 && len(r) < limit; i-- {
		v := s.state.Outbox[i]
		r = append(r, domain.DeliveryLog{ID: v.ID, IdempotencyKey: v.Key, Status: v.Status, Attempts: v.Attempts, LastHTTPStatus: v.HTTPStatus, LastError: v.LastError, LastResponse: v.LastResponse, CreatedAt: v.Created, DeliveredAt: v.Delivered})
	}
	return r, nil
}
func (s *Store) ClearDeliveryLogs() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keep []fileOutbox
	var n int64
	for _, v := range s.state.Outbox {
		if v.Status == "DELIVERED" || v.Status == "DEAD_LETTER" {
			n++
		} else {
			keep = append(keep, v)
		}
	}
	s.state.Outbox = keep
	return n, s.saveLocked()
}
func (s *Store) SavePollLog(v domain.PollLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ID == 0 {
		v.ID = s.nextID()
	}
	if v.CreatedAt == 0 {
		v.CreatedAt = time.Now().UnixMilli()
	}
	s.state.PollLogs = append(s.state.PollLogs, v)
	return s.saveLocked()
}
func (s *Store) PollLogs(cid int64, limit int) ([]domain.PollLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	r := []domain.PollLog{}
	for i := len(s.state.PollLogs) - 1; i >= 0 && len(r) < limit; i-- {
		if cid == 0 || s.state.PollLogs[i].ConnectionID == cid {
			r = append(r, s.state.PollLogs[i])
		}
	}
	return r, nil
}
func (s *Store) CleanupOld(ret time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cut := time.Now().Add(-ret).UnixMilli()
	var n int64
	var o []fileOutbox
	for _, v := range s.state.Outbox {
		if (v.Status == "DELIVERED" && v.Delivered < cut) || (v.Status == "DEAD_LETTER" && v.Created < cut) {
			n++
		} else {
			o = append(o, v)
		}
	}
	s.state.Outbox = o
	var p []domain.PollLog
	for _, v := range s.state.PollLogs {
		if v.CreatedAt < cut {
			n++
		} else {
			p = append(p, v)
		}
	}
	s.state.PollLogs = p
	return n, s.saveLocked()
}
func (s *Store) CurrentConfigVersion() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v int64
	for _, h := range s.state.History {
		if h.Status == "APPLIED" && h.Version > v {
			v = h.Version
		}
	}
	return v, nil
}
func (s *Store) ApplyConfigSnapshot(version int64, snap domain.ConfigSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap.Version = version
	s.state.Brands = snap.Brands
	s.state.DeviceSets = snap.DeviceSets
	s.state.Connections = snap.Connections
	s.state.Plants = snap.Plants
	for _, b := range s.state.Brands {
		if b.BrandID >= s.state.NextID {
			s.state.NextID = b.BrandID + 1
		}
	}
	for _, d := range s.state.DeviceSets {
		if d.DeviceSetID >= s.state.NextID {
			s.state.NextID = d.DeviceSetID + 1
		}
	}
	for _, c := range s.state.Connections {
		if c.ConnectionID >= s.state.NextID {
			s.state.NextID = c.ConnectionID + 1
		}
	}
	b, _ := json.Marshal(snap)
	s.state.History = append(s.state.History, fileHistory{Version: version, Status: "APPLIED", Snapshot: string(b), AppliedAt: time.Now().UnixMilli()})
	return s.saveLocked()
}

func first(v ...string) string {
	for _, x := range v {
		if strings.TrimSpace(x) != "" {
			return strings.TrimSpace(x)
		}
	}
	return ""
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
