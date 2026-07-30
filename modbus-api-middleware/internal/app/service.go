package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"chpp/modbus-api-middleware/internal/decoder"
	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/modbus"
	"chpp/modbus-api-middleware/internal/profile"
	"chpp/modbus-api-middleware/internal/store"
)

type Service struct {
	Store  *store.Store
	Client *modbus.Client
}

func (s *Service) PollConnection(value string) (domain.Reading, []domain.Measurement, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return domain.Reading{}, nil, fmt.Errorf("invalid connection id")
	}
	connection, err := s.Store.Connection(id)
	if err != nil {
		return domain.Reading{}, nil, err
	}
	set, err := s.Store.DeviceSet(connection.DeviceSetID)
	if err != nil {
		return domain.Reading{}, nil, err
	}
	registers := make([]domain.RegisterDefinition, 0, len(set.Addresses))
	for _, a := range set.Addresses {
		key := fmt.Sprintf("%d:%d", a.FunctionCode, a.Register)
		registers = append(registers, domain.RegisterDefinition{Key: key, SourceTag: first(a.SourceTag, a.Description), DataType: normalizeDataType(a.DataType), SourceUnit: a.SourceUnit, CanonicalUnit: a.CanonicalUnit, RegisterAddress: a.Register, FunctionCode: a.FunctionCode, Length: a.Length, Scale: a.Factor, Offset: a.Offset, WordOrder: a.WordOrder, Enabled: a.Enabled})
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

func normalizeDataType(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "SHORT":
		return "INT16"
	case "USHORT":
		return "UINT16"
	case "FLOAT":
		return "FLOAT32"
	default:
		return strings.ToUpper(strings.TrimSpace(value))
	}
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

func (s *Service) Enqueue(r domain.Reading, gatewayID string) (bool, error) {
	r, key, err := PrepareReadingForAPI(r, gatewayID)
	if err != nil {
		return false, err
	}
	b, _ := json.Marshal(r)
	sum := sha256.Sum256(b)
	return s.Store.Enqueue(key, hex.EncodeToString(sum[:]), r)
}
