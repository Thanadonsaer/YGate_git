package decoder

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

func Decode(registers []uint16, dataType, wordOrder string) (float64, error) {
	if len(registers) == 0 {
		return 0, fmt.Errorf("no registers")
	}
	kind := strings.ToUpper(strings.TrimSpace(dataType))
	switch kind {
	case "SHORT":
		kind = "INT16"
	case "USHORT":
		kind = "UINT16"
	case "LONG", "SLONG":
		kind = "INT32"
	case "ULONG", "DWORD":
		kind = "UINT32"
	case "FLOAT":
		kind = "FLOAT32"
	}
	switch kind {
	case "INT16":
		return float64(int16(registers[0])), nil
	case "UINT16":
		return float64(registers[0]), nil
	case "INT32", "UINT32", "S32", "U32", "FLOAT32", "SW_INT", "SW_UINT", "SW_FLOAT", "SMA_INT32", "SMA_UINT32":
		if len(registers) < 2 {
			return 0, fmt.Errorf("%s needs 2 registers", dataType)
		}
		a, b := registers[0], registers[1]
		if strings.EqualFold(wordOrder, "LOW_HIGH") || strings.HasPrefix(kind, "SW_") {
			a, b = b, a
		}
		u := uint32(a)<<16 | uint32(b)
		switch kind {
		case "S32", "SMA_INT32":
			if u == 0x80000000 {
				return math.NaN(), nil
			}
			return float64(int32(u)), nil
		case "U32", "SMA_UINT32":
			if u == 0xffffffff {
				return math.NaN(), nil
			}
			return float64(u), nil
		case "INT32", "SW_INT":
			return float64(int32(u)), nil
		case "UINT32", "SW_UINT":
			return float64(u), nil
		default:
			return float64(math.Float32frombits(u)), nil
		}
	case "UINT64", "U64", "SMA_UINT64":
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
		if (kind == "U64" || kind == "SMA_UINT64") && u == ^uint64(0) {
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
