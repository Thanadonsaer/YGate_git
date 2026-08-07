package ingestion

import (
	"math"
	"testing"
	"time"
)

func TestValidateRawReadingAcceptsAddressKeyWithoutCanonicalMapping(t *testing.T) {
	now := time.Now().UTC()
	reading, err := validateRawReading(RawReading{
		GatewayID: "G1", PlantCode: " p1 ", DevDn: " D1 ", DevTypeID: 1,
		CollectTime:        now.UnixMilli(),
		RegisterAddressMap: map[string]float64{"40084": 123},
	}, "client", now)
	if err != nil || reading.PlantCode != "P1" || reading.DevDn != "D1" {
		t.Fatalf("reading=%+v err=%v", reading, err)
	}
}

func TestValidateRawReadingRejectsNonNumericAddressKey(t *testing.T) {
	now := time.Now().UTC()
	_, err := validateRawReading(RawReading{
		GatewayID: "G1", PlantCode: "P1", DevDn: "D1", DevTypeID: 1,
		CollectTime:        now.UnixMilli(),
		RegisterAddressMap: map[string]float64{"active_power": 123},
	}, "client", now)
	if err == nil {
		t.Fatal("expected non-numeric address key error")
	}
}

func TestValidateRawReadingRejectsNonFiniteValue(t *testing.T) {
	now := time.Now().UTC()
	_, err := validateRawReading(RawReading{
		GatewayID: "G1", PlantCode: "P1", DevDn: "D1", DevTypeID: 1,
		CollectTime:        now.UnixMilli(),
		RegisterAddressMap: map[string]float64{"40084": math.NaN()},
	}, "client", now)
	if err == nil {
		t.Fatal("expected non-finite value error")
	}
}

func TestValidateRawReadingRejectsFutureTimestamp(t *testing.T) {
	now := time.Now().UTC()
	_, err := validateRawReading(RawReading{
		GatewayID: "G1", PlantCode: "P1", DevDn: "D1", DevTypeID: 1,
		CollectTime:        now.Add(11 * time.Minute).UnixMilli(),
		RegisterAddressMap: map[string]float64{"40084": 1},
	}, "client", now)
	if err == nil {
		t.Fatal("expected future timestamp error")
	}
}

func TestValidateRawReadingFillsIdentityDefaults(t *testing.T) {
	now := time.Now().UTC()
	reading, err := validateRawReading(RawReading{
		PlantCode: " ne=49712672 ", DevDn: " INV-1 ", DevTypeID: 1,
		CollectTime:        now.Add(-time.Hour).UnixMilli(),
		RegisterAddressMap: map[string]float64{"40001": 12.5},
	}, "Gateway A", now)
	if err != nil || reading.PlantCode != "NE=49712672" || reading.PlantName != "NE=49712672" ||
		reading.DevName != "INV-1" || reading.GatewayID != "Gateway A" || reading.Model != "DEV_TYPE_1" {
		t.Fatalf("reading=%+v err=%v", reading, err)
	}
}

func TestMetadataDataTypeUsesSupportedCanonicalTypes(t *testing.T) {
	for source, want := range map[string]string{"BOOL": "boolean", " bit ": "boolean", "INT16": "number", "FLOAT32": "number"} {
		if got := metadataDataType(source); got != want {
			t.Fatalf("metadataDataType(%q)=%q want %q", source, got, want)
		}
	}
}
