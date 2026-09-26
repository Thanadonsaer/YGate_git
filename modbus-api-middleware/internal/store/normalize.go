package store

// Config normalization and validation shared by both store adapters (SQLite
// on amd64/arm, the JSON file store on mips). Everything here is pure: no
// build tag, no I/O, so both adapters apply exactly the same rules and a plain
// `go test` covers them. Keeping one copy per adapter is how the mips store
// once skipped normalizeConnection and polled unit id 0.

import (
	"fmt"
	"strings"

	"chpp/modbus-api-middleware/internal/decoder"
	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/profile"
)

// normalizeConfigSnapshot validates and normalizes a whole pushed snapshot
// without touching any store, so the caller can reject it atomically. The
// returned snapshot is what gets persisted; IDs are kept as sent.
func normalizeConfigSnapshot(snap domain.ConfigSnapshot) (domain.ConfigSnapshot, error) {
	out := snap
	out.Brands = make([]domain.Brand, 0, len(snap.Brands))
	brandIDs := map[int64]bool{}
	for _, b := range snap.Brands {
		b.BrandName = strings.TrimSpace(b.BrandName)
		b.BrandDescription = strings.TrimSpace(b.BrandDescription)
		if b.BrandName == "" {
			return domain.ConfigSnapshot{}, fmt.Errorf("brand: brandName is required")
		}
		out.Brands = append(out.Brands, b)
		brandIDs[b.BrandID] = true
	}
	deviceSetIDs := map[int64]bool{}
	out.DeviceSets = make([]domain.DeviceSet, 0, len(snap.DeviceSets))
	for _, ds := range snap.DeviceSets {
		normalized := normalizeDeviceSet(ds)
		if !brandIDs[ds.BrandID] {
			return domain.ConfigSnapshot{}, fmt.Errorf("device set %q: unknown brandId %d", normalized.DevModel, ds.BrandID)
		}
		if strings.TrimSpace(normalized.DevType) == "" || strings.TrimSpace(normalized.DevModel) == "" {
			return domain.ConfigSnapshot{}, fmt.Errorf("device set: devType and devModel are required")
		}
		if !oneOf(normalized.ByteOrder, "BIG_ENDIAN", "LITTLE_ENDIAN") || !oneOf(normalized.WordOrder, "HIGH_LOW", "LOW_HIGH") || normalized.MaxBlockSize < 1 || normalized.MaxBlockSize > 125 {
			return domain.ConfigSnapshot{}, fmt.Errorf("device set %q: invalid byteOrder/wordOrder/maxBlockSize", normalized.DevModel)
		}
		if len(ds.Addresses) == 0 {
			return domain.ConfigSnapshot{}, fmt.Errorf("device set %q: at least one address is required", normalized.DevModel)
		}
		addresses := make([]domain.Address, len(ds.Addresses))
		for i, addr := range ds.Addresses {
			a, err := normalizeAddress(addr, normalized.AddressMode)
			if err != nil {
				return domain.ConfigSnapshot{}, fmt.Errorf("device set %q address %d: %w", normalized.DevModel, i+1, err)
			}
			if err = validateAddress(a, normalized.AddressMode); err != nil {
				return domain.ConfigSnapshot{}, fmt.Errorf("device set %q address %d: %w", normalized.DevModel, i+1, err)
			}
			a.DeviceSetID = ds.DeviceSetID
			addresses[i] = a
		}
		normalized.Addresses = addresses
		normalized.AddressIDList = addressIDList(addresses)
		out.DeviceSets = append(out.DeviceSets, normalized)
		deviceSetIDs[ds.DeviceSetID] = true
	}
	out.Plants = make([]domain.Plant, 0, len(snap.Plants))
	for _, p := range snap.Plants {
		p.PlantCode = strings.ToUpper(strings.TrimSpace(p.PlantCode))
		p.PlantName = strings.TrimSpace(p.PlantName)
		if p.PlantCode == "" || p.PlantName == "" {
			return domain.ConfigSnapshot{}, fmt.Errorf("plant: plantCode and plantName are required")
		}
		out.Plants = append(out.Plants, p)
	}
	out.Connections = make([]domain.ConnectionConfig, 0, len(snap.Connections))
	for _, c := range snap.Connections {
		normalized := normalizeConnection(c)
		if !deviceSetIDs[c.DeviceSetID] {
			return domain.ConfigSnapshot{}, fmt.Errorf("connection %q: unknown deviceSetId %d", normalized.ConnectionName, c.DeviceSetID)
		}
		if strings.TrimSpace(normalized.ConnectionName) == "" || strings.TrimSpace(normalized.Host) == "" || normalized.Port < 1 || normalized.Port > 65535 {
			return domain.ConfigSnapshot{}, fmt.Errorf("connection %q: connectionName, host and valid port are required", normalized.ConnectionName)
		}
		out.Connections = append(out.Connections, normalized)
	}
	return out, nil
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
	if mode, err := profile.CanonicalAddressMode(v.AddressMode); err == nil {
		v.AddressMode = mode
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

func normalizeAddress(a domain.Address, addressMode string) (domain.Address, error) {
	a.Description = strings.TrimSpace(a.Description)
	a.DataType = strings.ToUpper(strings.TrimSpace(a.DataType))
	a.SourceUnit = strings.TrimSpace(a.SourceUnit)
	a.CanonicalUnit = strings.TrimSpace(a.CanonicalUnit)
	a.Remark = strings.TrimSpace(a.Remark)
	if addressMode == "ZERO_BASED" {
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

func normalizeRegister(fc, register int) (int, int) {
	switch {
	case register >= 30000 && register < 40000:
		return 3, register - 30000
	case register >= 40000 && register < 50000:
		return 4, register - 40000
	default:
		return fc, register
	}
}

func validateAddress(a domain.Address, addressMode string) error {
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
	if _, err := profile.ResolveModbusAddress(addressMode, domain.RegisterDefinition{Key: a.Description, FunctionCode: a.FunctionCode, RegisterAddress: a.Register, Length: a.Length}); err != nil {
		return err
	}
	return nil
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
	if strings.TrimSpace(v.DevDn) == "" {
		v.DevDn = v.ConnectionName
	}
	if strings.TrimSpace(v.DeviceName) == "" {
		v.DeviceName = v.ConnectionName
	}
	if strings.TrimSpace(v.PlantCode) == "" {
		v.PlantCode = plantFromConnectionName(v.ConnectionName)
	}
	if strings.TrimSpace(v.PlantName) == "" {
		v.PlantName = v.PlantCode
	}
	if v.ConnectionID == 0 {
		v.Enabled = true
	}
	return v
}

func plantFromConnectionName(value string) string {
	head, _, ok := strings.Cut(strings.TrimSpace(value), "-")
	if !ok || strings.TrimSpace(head) == "" {
		return "DEFAULT"
	}
	return strings.ToUpper(strings.TrimSpace(head))
}

func devTypeID(value string) int {
	v := strings.ToLower(strings.TrimSpace(value))
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

func addressIDList(addresses []domain.Address) []int64 {
	out := make([]int64, 0, len(addresses))
	for _, a := range addresses {
		out = append(out, a.AddressID)
	}
	return out
}

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
