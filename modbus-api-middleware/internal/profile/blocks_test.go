package profile

import (
	"chpp/modbus-api-middleware/internal/domain"
	"testing"
)

func TestBuildBlocks(t *testing.T) {
	p := domain.RegisterProfile{AddressMode: "ZERO_BASED", DefaultFunctionCode: 3, Registers: []domain.RegisterDefinition{{Key: "a", RegisterAddress: 7000, Length: 2, Enabled: true}, {Key: "b", RegisterAddress: 7002, Length: 2, Enabled: true}, {Key: "c", RegisterAddress: 7010, Length: 2, Enabled: true}}}
	b, err := BuildBlocks(p)
	if err != nil || len(b) != 2 || b[0].Start != 7000 || b[0].Quantity != 4 || b[1].Start != 7010 {
		t.Fatalf("blocks=%+v err=%v", b, err)
	}
}
