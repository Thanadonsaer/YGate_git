package ingestion

import "testing"

func TestRegisterAlarmTransitionsCloseOldCodeAndOpenNewCode(t *testing.T) {
	open := []registerAlarmSignal{{MappingSourceID: "a", DisplayValue: "Model A"}}
	current := []registerAlarmSignal{{MappingSourceID: "b", DisplayValue: "Model B"}}
	closed, opened := registerAlarmTransitions(open, current)
	if len(closed) != 1 || closed[0].MappingSourceID != "a" || len(opened) != 1 || opened[0].MappingSourceID != "b" {
		t.Fatalf("closed=%+v opened=%+v", closed, opened)
	}
}

func TestRegisterAlarmTransitionsKeepRepeatedBitOpen(t *testing.T) {
	open := []registerAlarmSignal{{MappingSourceID: "bit-0"}}
	current := []registerAlarmSignal{{MappingSourceID: "bit-0"}, {MappingSourceID: "bit-1"}}
	closed, opened := registerAlarmTransitions(open, current)
	if len(closed) != 0 || len(opened) != 1 || opened[0].MappingSourceID != "bit-1" {
		t.Fatalf("closed=%+v opened=%+v", closed, opened)
	}
}
