package app

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"chpp/modbus-api-middleware/internal/configcache"
	"chpp/modbus-api-middleware/internal/decoder"
	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/modbus"
	"chpp/modbus-api-middleware/internal/profile"
	"chpp/modbus-api-middleware/internal/store"
)

type Service struct {
	Store  *store.Store
	Client *modbus.Client
	Cache  *configcache.Cache

	// Guards lastEnqueued and idleHeartbeat. Enqueue is driven by the poll
	// sweep, but the realtime command channel can trigger an out-of-band poll
	// on the same Service, so the two can overlap.
	idleMu        sync.Mutex
	lastEnqueued  map[string]enqueuedReading
	idleHeartbeat time.Duration
}

type enqueuedReading struct {
	values string
	at     time.Time
}

// DefaultIdleHeartbeat is the longest a device may go without a stored reading
// while its register values are not moving, when the gateway has no configured
// value of its own. Operators set the real one per gateway on the Middleware
// Gateways page; it arrives as GatewayConfig.IdleHeartbeatSeconds and is
// applied by SetIdleHeartbeat at the start of each poll sweep.
//
// A reading whose registers read exactly the same as the last one stored for
// that device is dropped instead of enqueued, which is most of what a solar
// site produces: every inverter sits at a frozen zero all night, and at a 5
// minute poll that is ~150 identical rows per device per night carried all the
// way into Postgres. Dropping them outright would make a healthy-but-idle
// device indistinguishable from a dead one, though -- the dashboard and SCADA
// status both key off "when did this device last report" -- so one reading
// still goes through this often no matter what, as a heartbeat.
//
// The trend charts join across the resulting gaps (see buildPath in
// apps/web/app/components/charts/time-series-chart.tsx): an absent sample means
// "unchanged", so a straight line between heartbeats is the accurate picture.
//
// Whatever it is set to, it needs to sit comfortably under whatever staleness
// window the platform treats as offline.
const DefaultIdleHeartbeat = 30 * time.Minute

// SetIdleHeartbeat applies the gateway's configured heartbeat. Called once per
// poll sweep from the config the poller already loads, so the per-device
// Enqueue path stays off the database. Zero or out-of-range falls back to
// DefaultIdleHeartbeat.
func (s *Service) SetIdleHeartbeat(seconds int) {
	s.idleMu.Lock()
	defer s.idleMu.Unlock()
	if seconds < 60 || seconds > 86400 {
		s.idleHeartbeat = 0
		return
	}
	s.idleHeartbeat = time.Duration(seconds) * time.Second
}

// heartbeat must be called with idleMu held.
func (s *Service) heartbeat() time.Duration {
	if s.idleHeartbeat <= 0 {
		return DefaultIdleHeartbeat
	}
	return s.idleHeartbeat
}

func (s *Service) PollConnection(value string) (domain.Reading, []domain.Measurement, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return domain.Reading{}, nil, fmt.Errorf("invalid connection id")
	}
	connection, err := s.localConnection(id)
	if err != nil {
		return domain.Reading{}, nil, err
	}
	set, err := s.localDeviceSet(connection.DeviceSetID)
	if err != nil {
		return domain.Reading{}, nil, err
	}
	registers := make([]domain.RegisterDefinition, 0, len(set.Addresses))
	for _, a := range set.Addresses {
		key := fmt.Sprintf("%d:%d", a.FunctionCode, a.Register)
		registers = append(registers, domain.RegisterDefinition{Key: key, SourceTag: first(a.SourceTag, a.Description), DataType: decoder.NormalizeDataType(a.DataType), SourceUnit: a.SourceUnit, CanonicalUnit: a.CanonicalUnit, RegisterAddress: a.Register, FunctionCode: a.FunctionCode, Length: a.Length, Scale: a.Factor, Offset: a.Offset, WordOrder: a.WordOrder, Enabled: a.Enabled})
	}
	p := domain.RegisterProfile{ProfileID: fmt.Sprint(set.DeviceSetID), ProfileName: set.BrandName + " " + set.DevModel, AddressMode: first(set.AddressMode, "ZERO_BASED"), ByteOrder: first(set.ByteOrder, "BIG_ENDIAN"), WordOrder: first(set.WordOrder, "HIGH_LOW"), MaxBlockSize: set.MaxBlockSize, DevTypeID: set.DevTypeID, Registers: registers, Enabled: true}
	blocks, err := profile.BuildBlocks(p)
	if err != nil {
		return domain.Reading{}, nil, err
	}
	if len(blocks) == 0 {
		return domain.Reading{}, nil, fmt.Errorf("device set has no address")
	}
	cycle := time.Now().UTC()
	values := map[string]float64{}
	measurements := []domain.Measurement{}
	var lastReadErr error
	type readFailure struct {
		FunctionCode int    `json:"functionCode"`
		Start        uint16 `json:"startAddress"`
		Quantity     uint16 `json:"quantity"`
		UnitID       int    `json:"unitId"`
		Error        string `json:"error"`
		Hint         string `json:"hint,omitempty"`
	}
	failures := []readFailure{}
	appendFailure := func(fc int, start, quantity uint16, err error) {
		failures = append(failures, readFailure{fc, start, quantity, connection.UnitID, err.Error(), modbusAddressHint(fc, start, err)})
	}
	readRegisters := func(fc int, start, quantity uint16) ([]uint16, error) {
		data, _, e := s.Client.Read(connection.Host, connection.Port, connection.UnitID, fc, start, quantity)
		if e != nil {
			return nil, fmt.Errorf("fc=%02d start=%d quantity=%d unit=%d: %w", fc, start, quantity, connection.UnitID, e)
		}
		regs, e := decoder.BytesToRegisters(data, p.ByteOrder)
		if e != nil {
			return nil, fmt.Errorf("fc=%02d start=%d quantity=%d unit=%d: %w", fc, start, quantity, connection.UnitID, e)
		}
		return regs, nil
	}
	decodeEntry := func(entry profile.Entry, regs []uint16, base uint16) {
		start := int(entry.Offset - base)
		def := entry.Definition
		if start < 0 || start+def.Length > len(regs) {
			lastReadErr = fmt.Errorf("%s: incomplete register response", def.SourceTag)
			appendFailure(def.FunctionCode, entry.Offset, uint16(def.Length), lastReadErr)
			return
		}
		raw, e := decoder.Decode(regs[start:start+def.Length], def.DataType, first(def.WordOrder, p.WordOrder))
		if e != nil {
			lastReadErr = fmt.Errorf("%s: %w", def.SourceTag, e)
			appendFailure(def.FunctionCode, entry.Offset, uint16(def.Length), lastReadErr)
			return
		}
		if math.IsNaN(raw) {
			lastReadErr = fmt.Errorf("%s: sentinel/NaN skipped", def.SourceTag)
			appendFailure(def.FunctionCode, entry.Offset, uint16(def.Length), lastReadErr)
			return
		}
		key := fmt.Sprintf("%d", def.RegisterAddress)
		sample := domain.RegisterSample{FunctionCode: def.FunctionCode, RegisterAddress: def.RegisterAddress, DataType: def.DataType, RawValue: raw, Quality: "GOOD"}
		// The Middleware sends the decoded register value unchanged.
		// Factor/Offset belongs to Platform Register Metadata so historical
		// Raw readings can be recalculated after configuration changes.
		values[key] = raw
		measurements = append(measurements, sample)
	}
	for _, b := range blocks {
		regs, e := readRegisters(b.FunctionCode, b.Start, b.Quantity)
		if e == nil {
			for _, entry := range b.Entries {
				decodeEntry(entry, regs, b.Start)
			}
			continue
		}
		lastReadErr = e
		appendFailure(b.FunctionCode, b.Start, b.Quantity, e)
		if len(b.Entries) == 1 && b.Quantity == uint16(b.Entries[0].Definition.Length) {
			continue
		}
		for _, entry := range b.Entries {
			quantity := uint16(entry.Definition.Length)
			regs, e = readRegisters(entry.Definition.FunctionCode, entry.Offset, quantity)
			if e != nil {
				lastReadErr = e
				appendFailure(entry.Definition.FunctionCode, entry.Offset, quantity, e)
				continue
			}
			decodeEntry(entry, regs, entry.Offset)
		}
	}
	if len(failures) > 0 {
		detail, _ := json.Marshal(map[string]any{"connectionId": connection.ConnectionID, "unitId": connection.UnitID, "failures": failures})
		s.logPoll(connection.ConnectionID, connection.ConnectionName, "WARN", fmt.Sprintf("%d Modbus read warning(s)", len(failures)), string(detail))
	}
	if len(measurements) == 0 {
		if lastReadErr != nil {
			return domain.Reading{}, nil, lastReadErr
		}
		return domain.Reading{}, nil, fmt.Errorf("device returned no data")
	}
	return domain.Reading{DevDn: connection.DevDn, DevName: connection.DeviceName, PlantCode: connection.PlantCode, PlantName: connection.PlantName, DevTypeID: set.DevTypeID, Model: set.DevModel, CollectTime: cycle.UnixMilli(), RegisterAddressMap: values}, measurements, nil
}

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func modbusAddressHint(fc int, start uint16, err error) string {
	if err == nil || !strings.Contains(err.Error(), "modbus exception") {
		return ""
	}
	if fc == 3 && start >= 40001 && start < 50000 {
		return "start address looks like 4xxxx notation; try Address Mode REGISTER_40001 for FC03 holding registers"
	}
	if fc == 4 && start >= 30001 && start < 40000 {
		return "start address looks like 3xxxx notation; try Address Mode REGISTER_30001 for FC04 input registers"
	}
	return ""
}

func PrepareReadingForAPI(r domain.Reading, gatewayID string) (domain.Reading, string, error) {
	gatewayID = strings.TrimSpace(gatewayID)
	r.GatewayID = gatewayID
	if gatewayID == "" || r.DevDn == "" || r.PlantCode == "" || r.DevTypeID <= 0 || r.CollectTime <= 0 || len(r.RegisterAddressMap) == 0 {
		return r, "", fmt.Errorf("incomplete API payload")
	}
	key := fmt.Sprintf("%s|%s|%d|%s|%d", gatewayID, r.PlantCode, r.DevTypeID, r.DevDn, r.CollectTime)
	return r, key, nil
}

// localConnection is deliberately backed by the Middleware SQLite store.
// Local Test/Read must use the configuration edited on this machine, not a
// potentially stale in-memory/realtime snapshot.
func (s *Service) localConnection(id int64) (domain.ConnectionConfig, error) {
	if s.Store != nil {
		connection, err := s.Store.Connection(id)
		if err == nil {
			return connection, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return domain.ConnectionConfig{}, fmt.Errorf("load local connection: %w", err)
		}
		return domain.ConnectionConfig{}, fmt.Errorf("connection not found")
	}
	if s.Cache != nil {
		if connection, ok := s.Cache.Load().Connections[id]; ok {
			return connection, nil
		}
	}
	return domain.ConnectionConfig{}, fmt.Errorf("connection not found")
}

func (s *Service) localDeviceSet(id int64) (domain.DeviceSet, error) {
	if s.Store != nil {
		set, err := s.Store.DeviceSet(id)
		if err == nil {
			return set, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return domain.DeviceSet{}, fmt.Errorf("load local device set: %w", err)
		}
		return domain.DeviceSet{}, fmt.Errorf("device set not found")
	}
	if s.Cache != nil {
		if set, ok := s.Cache.Load().DeviceSets[id]; ok {
			return set, nil
		}
	}
	return domain.DeviceSet{}, fmt.Errorf("device set not found")
}

// ProbeConnection runs a live Modbus connect-test against connectionID.
// Shared by the local-UI connect-test route (internal/web/server.go) and
// the remote connectTest command relayed over the realtime channel
// (internal/realtimeclient), so there is one probe implementation instead
// of two copies.
func (s *Service) ProbeConnection(connectionID int64) (modbus.ProbeInfo, time.Duration, domain.ConnectionConfig, error) {
	connection, err := s.localConnection(connectionID)
	if err != nil {
		return modbus.ProbeInfo{}, 0, domain.ConnectionConfig{}, err
	}
	client := s.Client
	if client == nil {
		client = &modbus.Client{Timeout: 3 * time.Second}
	}
	info, latency, err := client.ProbeDetails(connection.Host, connection.Port, connection.UnitID)
	return info, latency, connection, err
}

// Enqueue stores a reading in the outbox for delivery, unless it repeats the
// values already stored for that device and no heartbeat is due (see
// the gateway's IdleHeartbeatSeconds). Returns whether a row was created.
func (s *Service) Enqueue(r domain.Reading, gatewayID string) (bool, error) {
	r, key, err := PrepareReadingForAPI(r, gatewayID)
	if err != nil {
		return false, err
	}
	if s.skipUnchanged(r) {
		return false, nil
	}
	b, _ := json.Marshal(r)
	sum := sha256.Sum256(b)
	created, err := s.Store.Enqueue(key, hex.EncodeToString(sum[:]), r)
	if err != nil || !created {
		// The row was rejected or already present, so nothing new is stored --
		// forget the reading again so the next poll is compared against what
		// is genuinely in the outbox, not against a row that never landed.
		s.forgetEnqueued(r)
	}
	return created, err
}

// deviceKey identifies the device a reading belongs to, without its timestamp.
func deviceKey(r domain.Reading) string {
	return fmt.Sprintf("%s|%s|%d|%s", r.GatewayID, r.PlantCode, r.DevTypeID, r.DevDn)
}

// valuesHash fingerprints just the register values. encoding/json sorts map
// keys, so the same readings always produce the same hash.
func valuesHash(r domain.Reading) string {
	b, _ := json.Marshal(r.RegisterAddressMap)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// skipUnchanged reports whether this reading repeats the last one enqueued for
// its device and is not yet due a heartbeat. When it returns false it records
// the reading as the new baseline, so the heartbeat clock only advances on
// readings that are actually stored -- resetting it on every skipped poll would
// mean the heartbeat never fires at all.
//
// ponytail: the baseline is in memory, so the first poll after a restart always
// stores. That is the safe direction (one extra row, never a missed change) and
// it keeps the poll path off the database. Persist it in gateway_config if
// restarts ever get frequent enough to matter.
func (s *Service) skipUnchanged(r domain.Reading) bool {
	observed := time.UnixMilli(r.CollectTime)
	values := valuesHash(r)
	device := deviceKey(r)

	s.idleMu.Lock()
	defer s.idleMu.Unlock()
	// A reading older than the baseline is out of order, so its elapsed time is
	// meaningless; store it and let the platform's idempotency key sort it out.
	if last, seen := s.lastEnqueued[device]; seen && last.values == values &&
		!observed.Before(last.at) && observed.Sub(last.at) < s.heartbeat() {
		return true
	}
	if s.lastEnqueued == nil {
		s.lastEnqueued = map[string]enqueuedReading{}
	}
	s.lastEnqueued[device] = enqueuedReading{values: values, at: observed}
	return false
}

func (s *Service) forgetEnqueued(r domain.Reading) {
	s.idleMu.Lock()
	defer s.idleMu.Unlock()
	delete(s.lastEnqueued, deviceKey(r))
}
