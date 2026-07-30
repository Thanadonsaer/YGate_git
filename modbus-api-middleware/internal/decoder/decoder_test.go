package decoder

import (
	"math"
	"testing"
)

func TestDecode(t *testing.T) {
	tests := []struct {
		name, kind, words string
		regs              []uint16
		want              float64
	}{
		{"int16", "INT16", "HIGH_LOW", []uint16{0xffff}, -1},
		{"uint16", "UINT16", "HIGH_LOW", []uint16{65535}, 65535},
		{"int32", "INT32", "HIGH_LOW", []uint16{0xffff, 0xfffe}, -2},
		{"uint32 low first", "UINT32", "LOW_HIGH", []uint16{2, 1}, 65538},
		{"float32", "FLOAT32", "HIGH_LOW", []uint16{0x4148, 0}, 12.5},
		{"SMA U64", "U64", "HIGH_LOW", []uint16{0, 0, 1, 2}, 65538},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Decode(tt.regs, tt.kind, tt.words)
			if err != nil || math.Abs(got-tt.want) > .0001 {
				t.Fatalf("got %v err=%v want %v", got, err, tt.want)
			}
		})
	}
	if got, err := Decode([]uint16{0xffff, 0xffff}, "U32", "HIGH_LOW"); err != nil || !math.IsNaN(got) {
		t.Fatalf("SMA NaN sentinel: got %v err=%v", got, err)
	}
}
