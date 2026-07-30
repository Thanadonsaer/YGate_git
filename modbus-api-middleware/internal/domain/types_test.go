package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReadingJSONContract(t *testing.T) {
	b, err := json.Marshal(Reading{GatewayID: "G1", DevDn: "D1", PlantCode: "P1", DevTypeID: 1, Model: "SUN2000", CollectTime: 123, RegisterAddressMap: map[string]float64{
		"40084": 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, required := range []string{`"gatewayId"`, `"devDn"`, `"plantCode"`, `"devTypeId"`, `"model"`, `"collectTime"`, `"registerAddressMap"`, `"40084":1`} {
		if !strings.Contains(s, required) {
			t.Fatalf("missing %s in %s", required, s)
		}
	}
	if strings.Contains(s, "dataItemMap") || strings.Contains(s, "canonicalKey") {
		t.Fatalf("legacy mapping leaked into v2 reading: %s", s)
	}
	for _, dropped := range []string{"rawValue", "quality", "dataType", "functionCode", `"registerAddress":`} {
		if strings.Contains(s, dropped) {
			t.Fatalf("slim v2 payload must not carry per-entry %s: %s", dropped, s)
		}
	}
}
