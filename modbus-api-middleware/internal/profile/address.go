package profile

import (
	"fmt"
	"strings"

	"chpp/modbus-api-middleware/internal/domain"
)

type ResolvedAddress struct {
	FunctionCode int
	Start        uint16
	Quantity     uint16
}

func CanonicalAddressMode(addressMode string) (string, error) {
	mode := strings.ToUpper(strings.TrimSpace(addressMode))
	switch mode {
	case "", "ZERO_BASED", "OFFSET_0", "ZERO_BASED_OFFSET":
		return "ZERO_BASED", nil
	case "ONE_BASED", "OFFSET_1", "ONE_BASED_OFFSET":
		return "ONE_BASED", nil
	case "FULL_NOTATION", "FULL", "MODBUS_NOTATION":
		return "FULL_NOTATION", nil
	case "VENDOR_RAW", "RAW", "DIRECT", "MODBUS_RAW":
		return "VENDOR_RAW", nil
	case "SMA":
		return "SMA", nil
	case "REGISTER_30001", "INPUT_30001", "INPUT_REGISTER_30001":
		return "REGISTER_30001", nil
	case "REGISTER_40001", "HOLDING_40001", "HOLDING_REGISTER_40001":
		return "REGISTER_40001", nil
	default:
		return "", fmt.Errorf("unsupported address mode %q", addressMode)
	}
}

func ResolveModbusAddress(addressMode string, d domain.RegisterDefinition) (ResolvedAddress, error) {
	mode, err := CanonicalAddressMode(addressMode)
	if err != nil {
		return ResolvedAddress{}, err
	}
	fc, start, quantity := d.FunctionCode, d.RegisterAddress, d.Length
	if quantity < 1 || quantity > 125 {
		return ResolvedAddress{}, fmt.Errorf("%s has invalid length %d", d.Key, quantity)
	}
	switch mode {
	case "ZERO_BASED", "VENDOR_RAW", "SMA":
	case "ONE_BASED":
		start--
	case "FULL_NOTATION":
		switch {
		case start >= 30000 && start < 40000:
			fc, start = 3, start-30000
		case start >= 40000 && start < 50000:
			fc, start = 4, start-40000
		default:
			return ResolvedAddress{}, fmt.Errorf("register %d is not 3xxxx/4xxxx full notation", d.RegisterAddress)
		}
	case "REGISTER_40001":
		start -= 40001
	case "REGISTER_30001":
		start -= 30001
	}
	if fc != 3 && fc != 4 {
		return ResolvedAddress{}, fmt.Errorf("%s has unsupported function code %d", d.Key, fc)
	}
	if start < 0 || start > 65535 || start+quantity > 65536 {
		return ResolvedAddress{}, fmt.Errorf("register %d out of range for %s", d.RegisterAddress, mode)
	}
	return ResolvedAddress{FunctionCode: fc, Start: uint16(start), Quantity: uint16(quantity)}, nil
}
