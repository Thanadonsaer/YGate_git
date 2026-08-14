package core

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type RegisterValueMapping struct {
	MatchValue   *int64 `json:"matchValue,omitempty"`
	BitIndex     *int32 `json:"bitIndex,omitempty"`
	DisplayValue string `json:"displayValue"`
	AlarmState   string `json:"alarmState,omitempty"`
	Severity     string `json:"severity,omitempty"`
}

type RegisterDefinition struct {
	AddressKey string                 `json:"addressKey"`
	Scale      float64                `json:"scale"`
	Offset     float64                `json:"offset"`
	Decimals   int32                  `json:"decimals"`
	Mappings   []RegisterValueMapping `json:"mappings,omitempty"`
}

type ResolvedValueMatch struct {
	Mapping      RegisterValueMapping `json:"mapping"`
	NumericValue float64              `json:"numericValue"`
}

type ResolvedRegister struct {
	AddressKey   string               `json:"addressKey"`
	NumericValue float64              `json:"numericValue"`
	DisplayValue string               `json:"displayValue"`
	Matches      []ResolvedValueMatch `json:"matches,omitempty"`
}

func ResolveRegister(definition RegisterDefinition, raw float64) ResolvedRegister {
	numeric := raw*definition.Scale + definition.Offset
	result := ResolvedRegister{AddressKey: definition.AddressKey, NumericValue: numeric, DisplayValue: formatRegisterNumber(numeric, definition.Decimals)}
	if len(definition.Mappings) == 0 {
		return result
	}

	if isInteger(raw) {
		rawInteger := int64(raw)
		for _, mapping := range definition.Mappings {
			if mapping.MatchValue != nil && *mapping.MatchValue == rawInteger {
				result.Matches = append(result.Matches, ResolvedValueMatch{Mapping: mapping, NumericValue: numeric})
			}
		}
	}
	if len(result.Matches) == 0 && isInteger(raw) && raw >= 0 {
		rawBits := uint64(raw)
		for _, mapping := range definition.Mappings {
			if mapping.BitIndex != nil && *mapping.BitIndex >= 0 && *mapping.BitIndex < 64 && rawBits&(uint64(1)<<uint(*mapping.BitIndex)) != 0 {
				result.Matches = append(result.Matches, ResolvedValueMatch{Mapping: mapping, NumericValue: numeric})
			}
		}
		sort.SliceStable(result.Matches, func(i, j int) bool {
			return *result.Matches[i].Mapping.BitIndex < *result.Matches[j].Mapping.BitIndex
		})
	}
	if len(result.Matches) > 0 {
		labels := make([]string, 0, len(result.Matches))
		for _, match := range result.Matches {
			if strings.TrimSpace(match.Mapping.DisplayValue) != "" {
				labels = append(labels, strings.TrimSpace(match.Mapping.DisplayValue))
			}
		}
		if len(labels) > 0 {
			result.DisplayValue = strings.Join(labels, ", ")
		}
	}
	return result
}

func isInteger(value float64) bool {
	return math.IsNaN(value) == false && math.IsInf(value, 0) == false && math.Trunc(value) == value
}

func formatRegisterNumber(value float64, decimals int32) string {
	if decimals < 0 {
		decimals = 0
	}
	if decimals > 9 {
		decimals = 9
	}
	return strconv.FormatFloat(value, 'f', int(decimals), 64)
}

func formatRegisterNumberForError(value float64) string {
	return fmt.Sprintf("%v", value)
}
