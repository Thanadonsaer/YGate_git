package decoder

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// CanonicalTypes is the data type list offered in the UI, named to match the
// Huawei Solar Inverter Modbus Interface Definitions register tables (U16,
// I16, U32, I32; U64 and FLOAT32 cover vendors like SMA that need them).
var CanonicalTypes = []string{"U16", "I16", "U32", "I32", "U64", "FLOAT32"}

// NormalizeDataType maps a configured dataType (canonical or a legacy vendor
// alias already stored in existing device sets) to the canonical name used
// internally. Centralized here so Decode, register-length inference and the
// import path agree on the same aliases instead of drifting independently.
func NormalizeDataType(dataType string) string {
	switch strings.ToUpper(strings.TrimSpace(dataType)) {
	case "SHORT", "INT16":
		return "I16"
	case "USHORT", "UINT16":
		return "U16"
	case "LONG", "SLONG", "INT32", "S32", "SW_INT", "SMA_INT32":
		return "I32"
	case "ULONG", "DWORD", "UINT32", "SW_UINT", "SMA_UINT32":
		return "U32"
	case "UINT64", "SMA_UINT64":
		return "U64"
	case "FLOAT", "SW_FLOAT":
		return "FLOAT32"
	default:
		return strings.ToUpper(strings.TrimSpace(dataType))
	}
}

// RegisterCount returns how many 16-bit registers a (possibly legacy) data
// type occupies, or 0 if unknown.
func RegisterCount(dataType string) int {
	switch NormalizeDataType(dataType) {
	case "U16", "I16":
		return 1
	case "U32", "I32", "FLOAT32":
		return 2
	case "U64":
		return 4
	default:
		return 0
	}
}

func Decode(registers []uint16, dataType, wordOrder string) (float64, error) {
	if len(registers) == 0 {
		return 0, fmt.Errorf("no registers")
	}
	swapWords := strings.EqualFold(wordOrder, "LOW_HIGH") || strings.HasPrefix(strings.ToUpper(strings.TrimSpace(dataType)), "SW_")
	switch NormalizeDataType(dataType) {
	case "I16":
		return float64(int16(registers[0])), nil
	case "U16":
		return float64(registers[0]), nil
	case "I32", "U32", "FLOAT32":
		if len(registers) < 2 {
			return 0, fmt.Errorf("%s needs 2 registers", dataType)
		}
		a, b := registers[0], registers[1]
		if swapWords {
			a, b = b, a
		}
		u := uint32(a)<<16 | uint32(b)
		switch NormalizeDataType(dataType) {
		case "I32":
			if u == 0x80000000 {
				return math.NaN(), nil
			}
			return float64(int32(u)), nil
		case "U32":
			if u == 0xffffffff {
				return math.NaN(), nil
			}
			return float64(u), nil
		default:
			return float64(math.Float32frombits(u)), nil
		}
	case "U64":
		if len(registers) < 4 {
			return 0, fmt.Errorf("%s needs 4 registers", dataType)
		}
		words := append([]uint16(nil), registers[:4]...)
		if strings.EqualFold(wordOrder, "LOW_HIGH") {
			for i, j := 0, len(words)-1; i < j; i, j = i+1, j-1 {
				words[i], words[j] = words[j], words[i]
			}
		}
		var u uint64
		for _, word := range words {
			u = u<<16 | uint64(word)
		}
		if u == ^uint64(0) {
			return math.NaN(), nil
		}
		return float64(u), nil
	default:
		return 0, fmt.Errorf("unsupported data type %q", dataType)
	}
}

func BytesToRegisters(data []byte, byteOrder string) ([]uint16, error) {
	if len(data)%2 != 0 {
		return nil, fmt.Errorf("odd register byte count")
	}
	result := make([]uint16, len(data)/2)
	for i := range result {
		pair := data[i*2 : i*2+2]
		if strings.EqualFold(byteOrder, "LITTLE_ENDIAN") {
			result[i] = binary.LittleEndian.Uint16(pair)
		} else {
			result[i] = binary.BigEndian.Uint16(pair)
		}
	}
	return result, nil
}
