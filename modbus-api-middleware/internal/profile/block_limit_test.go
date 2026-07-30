package profile

import (
	"testing"

	"chpp/modbus-api-middleware/internal/domain"
)

func TestBuildBlocksHonorsModbus125RegisterLimit(t *testing.T) {
	registers := make([]domain.RegisterDefinition, 126)
	for i := range registers {
		registers[i] = domain.RegisterDefinition{Key: "k", RegisterAddress: i, FunctionCode: 3, Length: 1, Enabled: true}
	}
	blocks, err := BuildBlocks(domain.RegisterProfile{AddressMode: "ZERO_BASED", MaxBlockSize: 125, Registers: registers})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || blocks[0].Quantity != 125 || blocks[1].Quantity != 1 {
		t.Fatalf("blocks=%+v", blocks)
	}
}
