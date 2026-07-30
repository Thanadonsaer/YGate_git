package normalizer

import (
	"fmt"
	"strings"
)

var aliases = map[string]string{
	"active_power": "active_power", "inst_kw": "active_power", "emt.activepw": "active_power", "active power": "active_power",
	"reactive_power": "reactive_power", "reactive power": "reactive_power", "power_factor": "power_factor", "power factor": "power_factor",
	"elec_freq": "elec_freq", "frequency": "elec_freq", "day_cap": "day_cap", "total_cap": "total_cap", "run_state": "run_state", "inverter_state": "inverter_state",
	"temperature": "temperature", "cabinet temperature": "temperature", "pv_temperature": "pv_temperature", "wind_speed": "wind_speed", "wind_direction": "wind_direction", "radiant_line": "radiant_line", "radiant_total": "radiant_total",
	"a_u": "a_u", "b_u": "b_u", "c_u": "c_u", "ab_u": "ab_u", "bc_u": "bc_u", "ca_u": "ca_u", "a_i": "a_i", "b_i": "b_i", "c_i": "c_i",
	"phase_a_voltage": "a_u", "phase_b_voltage": "b_u", "phase_c_voltage": "c_u", "phase_a_current": "a_i", "phase_b_current": "b_i", "phase_c_current": "c_i",
	"active_cap": "active_cap", "reverse_active_cap": "reverse_active_cap", "import_active_energy": "reverse_active_cap", "export_active_energy": "active_cap",
}

func Key(value string) (string, error) {
	k := strings.ToLower(strings.TrimSpace(value))
	if canonical, ok := aliases[k]; ok {
		return canonical, nil
	}
	k = strings.NewReplacer(" ", "_", "-", "_").Replace(k)
	if canonical, ok := aliases[k]; ok {
		return canonical, nil
	}
	return "", fmt.Errorf("unknown canonical KPI %q", value)
}
func Unit(value float64, source, canonical string) (float64, error) {
	s, c := strings.ToLower(strings.TrimSpace(source)), strings.ToLower(strings.TrimSpace(canonical))
	if s == c || s == "" || c == "" {
		return value, nil
	}
	if (s == "w" && c == "kw") || (s == "var" && c == "kvar") || (s == "wh" && c == "kwh") || (s == "varh" && c == "kvarh") {
		return value / 1000, nil
	}
	return 0, fmt.Errorf("unsupported unit conversion %s to %s", source, canonical)
}
