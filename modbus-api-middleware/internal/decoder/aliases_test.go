package decoder

import "testing"

func TestConfiguredDataTypeAliases(t *testing.T) {
	short, err := Decode([]uint16{0xffff}, "SHORT", "HIGH_LOW")
	if err != nil || short != -1 {
		t.Fatalf("SHORT=%v,%v", short, err)
	}
	swapped, err := Decode([]uint16{2, 1}, "SW_INT", "HIGH_LOW")
	if err != nil || swapped != 65538 {
		t.Fatalf("SW_INT=%v,%v", swapped, err)
	}
	floating, err := Decode([]uint16{0x4148, 0}, "FLOAT", "HIGH_LOW")
	if err != nil || floating != 12.5 {
		t.Fatalf("FLOAT=%v,%v", floating, err)
	}
	long, err := Decode([]uint16{0xffff, 0xfffe}, "LONG", "HIGH_LOW")
	if err != nil || long != -2 {
		t.Fatalf("LONG=%v,%v", long, err)
	}
	ulong, err := Decode([]uint16{0x0001, 0x0002}, "ULONG", "HIGH_LOW")
	if err != nil || ulong != 65538 {
		t.Fatalf("ULONG=%v,%v", ulong, err)
	}
}
