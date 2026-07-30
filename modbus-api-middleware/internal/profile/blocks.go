package profile

import (
	"fmt"
	"sort"

	"chpp/modbus-api-middleware/internal/domain"
)

type Entry struct {
	Definition domain.RegisterDefinition
	Offset     uint16
}
type Block struct {
	FunctionCode    int
	Start, Quantity uint16
	Entries         []Entry
}

func BuildBlocks(p domain.RegisterProfile) ([]Block, error) {
	limit := p.MaxBlockSize
	if limit < 1 {
		limit = 30
	}
	if limit > 125 {
		limit = 125
	}
	entries := []Entry{}
	for _, d := range p.Registers {
		if !d.Enabled {
			continue
		}
		if d.Length < 1 || d.Length > 125 {
			return nil, fmt.Errorf("%s has invalid length", d.Key)
		}
		if d.FunctionCode == 0 {
			d.FunctionCode = p.DefaultFunctionCode
		}
		resolved, err := ResolveModbusAddress(p.AddressMode, d)
		if err != nil {
			return nil, err
		}
		d.FunctionCode = resolved.FunctionCode
		entries = append(entries, Entry{d, resolved.Start})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Definition.FunctionCode != entries[j].Definition.FunctionCode {
			return entries[i].Definition.FunctionCode < entries[j].Definition.FunctionCode
		}
		return entries[i].Offset < entries[j].Offset
	})
	blocks := []Block{}
	for _, e := range entries {
		end := uint32(e.Offset) + uint32(e.Definition.Length)
		if end > 65536 {
			return nil, fmt.Errorf("%s exceeds register range", e.Definition.Key)
		}
		newBlock := len(blocks) == 0
		if !newBlock {
			b := blocks[len(blocks)-1]
			currentEnd := uint32(b.Start) + uint32(b.Quantity)
			newBlock = b.FunctionCode != e.Definition.FunctionCode || uint32(e.Offset) > currentEnd || end-uint32(b.Start) > uint32(limit)
		}
		if newBlock {
			blocks = append(blocks, Block{FunctionCode: e.Definition.FunctionCode, Start: e.Offset, Quantity: uint16(e.Definition.Length), Entries: []Entry{e}})
			continue
		}
		b := &blocks[len(blocks)-1]
		if quantity := end - uint32(b.Start); quantity > uint32(b.Quantity) {
			b.Quantity = uint16(quantity)
		}
		b.Entries = append(b.Entries, e)
	}
	return blocks, nil
}
