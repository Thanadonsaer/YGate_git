package ingestion

import (
	"testing"
	"time"
)

func TestValidateReadingNormalizesLegacyIdentity(t *testing.T) {
	now := time.Now().UTC()
	reading, err := validateReading(Reading{
		PlantCode: " ne=49712672 ", DevDn: " INV-1 ", DevTypeID: 1,
		CollectTime: now.Add(-time.Hour).UnixMilli(), DataItemMap: map[string]float64{"active_power": 12.5},
	}, "Gateway A", now)
	if err != nil || reading.PlantCode != "NE=49712672" || reading.PlantName != "NE=49712672" || reading.DevName != "INV-1" || reading.GatewayID != "Gateway A" || reading.Model != "DEV_TYPE_1" {
		t.Fatalf("reading=%+v err=%v", reading, err)
	}
}

func TestValidateReadingRejectsFutureTimestamp(t *testing.T) {
	now := time.Now().UTC()
	_, err := validateReading(Reading{PlantCode: "P1", DevDn: "D1", DevTypeID: 1, CollectTime: now.Add(11 * time.Minute).UnixMilli(), DataItemMap: map[string]float64{"x": 1}}, "G1", now)
	if err == nil {
		t.Fatal("expected future timestamp error")
	}
}
