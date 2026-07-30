package normalizer

import "testing"

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{"Inst_kW": "active_power", "Watts": "active_power", "Operating State": "inverter_state", "%Command Watt Limit": "active_power_adjustment", "EMT.ActivePW": "active_power", "Power Factor": "power_factor", "Grid frequency": "elec_freq", "Energy yield of current day": "day_cap", "Total energy yield": "total_cap", "Device status": "inverter_state", "PV1 voltage": "pv1_voltage", "Alarm_1": "alarm_1"} {
		got, err := Key(in)
		if err != nil || got != want {
			t.Fatalf("%s=%s,%v", in, got, err)
		}
	}
	for _, tt := range []struct {
		value             float64
		source, canonical string
		want              float64
	}{{1000, "W", "kW", 1}, {2000, "Var", "kVAr", 2}, {3000, "Wh", "kWh", 3}, {4, "kW", "kW", 4}} {
		got, err := Unit(tt.value, tt.source, tt.canonical)
		if err != nil || got != tt.want {
			t.Fatalf("Unit=%v,%v", got, err)
		}
	}
	if _, err := Key("invented_kpi"); err == nil {
		t.Fatal("unknown KPI accepted")
	}
}
