package core

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestResolveRegisterExactMappingAndNumericFallback(t *testing.T) {
	definition := RegisterDefinition{
		AddressKey: "30070",
		Scale:      1,
		Offset:     0,
		Decimals:   0,
		Mappings: []RegisterValueMapping{
			{MatchValue: int64Ptr(145), DisplayValue: "Model A"},
			{MatchValue: int64Ptr(146), DisplayValue: "Model B"},
		},
	}
	if got := ResolveRegister(definition, 145); got.DisplayValue != "Model A" || got.NumericValue != 145 {
		t.Fatalf("exact mapping = %#v", got)
	}
	if got := ResolveRegister(definition, 147); got.DisplayValue != "147" || got.NumericValue != 147 {
		t.Fatalf("numeric fallback = %#v", got)
	}
}

func int64Ptr(value int64) *int64 { return &value }

func TestResolveRegisterBitmaskReturnsAllActiveMappings(t *testing.T) {
	definition := RegisterDefinition{
		AddressKey: "40001",
		Mappings: []RegisterValueMapping{
			{BitIndex: int32Pointer(pgtype.Int4{Int32: 0, Valid: true}), DisplayValue: "DC overvoltage"},
			{BitIndex: int32Pointer(pgtype.Int4{Int32: 2, Valid: true}), DisplayValue: "AC fault"},
		},
	}
	got := ResolveRegister(definition, 5)
	if got.DisplayValue != "DC overvoltage, AC fault" || len(got.Matches) != 2 {
		t.Fatalf("bitmask mapping = %#v", got)
	}
}
