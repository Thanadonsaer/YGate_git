package normalizer

func init() {
	for k, v := range map[string]string{
		"active_power_adjustment":     "active_power_adjustment",
		"active power adjustment":     "active_power_adjustment",
		"reactive_power_adjustment":   "reactive_power_adjustment",
		"reactive power adjustment":   "reactive_power_adjustment",
		"inst_kw":                     "active_power",
		"inst kw":                     "active_power",
		"watts":                       "active_power",
		"operating_state":             "inverter_state",
		"operating state":             "inverter_state",
		"%command_watt_limit":         "active_power_adjustment",
		"%command watt limit":         "active_power_adjustment",
		"command_watt_limit":          "active_power_adjustment",
		"command watt limit":          "active_power_adjustment",
		"grid_frequency":              "elec_freq",
		"grid frequency":              "elec_freq",
		"energy_yield_of_current_day": "day_cap",
		"energy yield of current day": "day_cap",
		"total_energy_yield":          "total_cap",
		"total energy yield":          "total_cap",
		"device_status":               "inverter_state",
		"device status":               "inverter_state",
		"cabinet_temperature":         "temperature",
		"cabinet temperature":         "temperature",
		"line_voltage_l1_l2":          "ab_u",
		"line voltage l1-l2":          "ab_u",
		"line_voltage_l2_l3":          "bc_u",
		"line voltage l2-l3":          "bc_u",
		"line_voltage_l3_l1":          "ca_u",
		"line voltage l3-l1":          "ca_u",
	} {
		aliases[k] = v
	}
	for _, k := range []string{
		"rated_power", "collect_dsp_data", "locking", "reduced_co2_emission",
		"shutdown_time", "startup_time", "efficiency", "alarm_1", "alarm_2", "alarm_3",
		"power_adjustment", "input_power",
		"pv1_voltage", "pv1_current", "pv2_voltage", "pv2_current", "pv3_voltage", "pv3_current", "pv4_voltage", "pv4_current",
		"pv5_voltage", "pv5_current", "pv6_voltage", "pv6_current", "pv7_voltage", "pv7_current", "pv8_voltage", "pv8_current",
		"pv9_voltage", "pv9_current", "pv10_voltage", "pv10_current", "pv11_voltage", "pv11_current", "pv12_voltage", "pv12_current",
		"pv13_voltage", "pv13_current", "pv14_voltage", "pv14_current", "pv15_voltage", "pv15_current", "pv16_voltage", "pv16_current",
		"pv17_voltage", "pv17_current", "pv18_voltage", "pv18_current", "pv19_voltage", "pv19_current", "pv20_voltage", "pv20_current",
	} {
		aliases[k] = k
	}
}
