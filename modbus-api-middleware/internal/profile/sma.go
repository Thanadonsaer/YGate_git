package profile

import "chpp/modbus-api-middleware/internal/domain"

const SMADeviceModel = "Sunny Central SC/HE via SC-COM"

// SMADeviceSet is the register profile used by the working sma.py reader.
// SMA addresses are passed to Modbus unchanged (for example 30057 stays 30057).
func SMADeviceSet(brandID int64) domain.DeviceSet {
	return domain.DeviceSet{
		BrandID: brandID, DevTypeID: 1, DevType: "Inverter", DevModel: SMADeviceModel,
		AddressMode: "SMA", ByteOrder: "BIG_ENDIAN", WordOrder: "HIGH_LOW", MaxBlockSize: 30,
		Addresses: []domain.Address{
			smaAddress(30057, 2, "U32", "Serial number", "serial_number", 1, "", "", ""),
			smaAddress(30193, 2, "U32", "System time (UTC)", "system_time", 1, "s", "s", "Unix epoch UTC"),
			smaAddress(30197, 2, "U32", "Event ID (current)", "event_id", 1, "", "", ""),
			smaAddress(30199, 2, "U32", "Time until grid connection", "time_until_grid_connection", 1, "s", "s", ""),
			smaAddress(30211, 2, "U32", "Recommended action", "recommended_action", 1, "", "", "336=manufacturer, 337=installer, 338=invalid"),
			smaAddress(30217, 2, "U32", "Grid contactor", "grid_contactor", 1, "", "", "51=closed, 311=open"),
			smaAddress(30225, 2, "U32", "Insulation resistance", "insulation_resistance", 1, "ohm", "ohm", ""),
			smaAddress(30231, 2, "U32", "Max permanent active power", "rated_power", 1, "W", "kW", ""),
			smaAddress(30233, 2, "U32", "Active power limitation (Pmax)", "active_power_adjustment", 1, "W", "kW", ""),
			smaAddress(30241, 2, "U32", "Operating state", "inverter_state", 1, "", "", "309=operation, 381=stop, 455=warning, 1392=error"),
			smaAddress(30243, 2, "U32", "Error", "error_code", 1, "", "", "267=inverter, 1395=DC, 1396=AC grid"),
			smaAddress(30247, 2, "U32", "Complete event number", "complete_event_number", 1, "", "", ""),
			smaAddress(30257, 2, "U32", "DC switch in cabinet", "dc_switch", 1, "", "", "51=closed, 311=open"),
			smaAddress(30513, 4, "U64", "Total yield", "total_cap", 1, "Wh", "kWh", ""),
			smaAddress(30517, 4, "U64", "Daily yield", "day_cap", 1, "Wh", "kWh", ""),
			smaAddress(30521, 4, "U64", "Operating time", "operating_time", 1, "s", "s", ""),
			smaAddress(30525, 4, "U64", "Feed-in time", "feed_in_time", 1, "s", "s", ""),
			smaAddress(30769, 2, "S32", "DC current input (Ipv)", "dc_current_input", .001, "A", "A", ""),
			smaAddress(30771, 2, "S32", "DC voltage input (Vpv)", "dc_voltage_input", .01, "V", "V", ""),
			smaAddress(30773, 2, "S32", "DC power input (Ppv)", "input_power", 1, "W", "kW", ""),
			smaAddress(30775, 2, "S32", "AC active power (Pac)", "active_power", 1, "W", "kW", ""),
			smaAddress(30789, 2, "U32", "Grid voltage L1-L2 (AB)", "ab_u", .01, "V", "V", ""),
			smaAddress(30791, 2, "U32", "Grid voltage L2-L3 (BC)", "bc_u", .01, "V", "V", ""),
			smaAddress(30793, 2, "U32", "Grid voltage L3-L1 (CA)", "ca_u", .01, "V", "V", ""),
			smaAddress(30795, 2, "U32", "Grid current AC (Iac)", "grid_current", .001, "A", "A", ""),
			smaAddress(30797, 2, "U32", "Grid current L1", "a_i", .001, "A", "A", ""),
			smaAddress(30799, 2, "U32", "Grid current L2", "b_i", .001, "A", "A", ""),
			smaAddress(30801, 2, "U32", "Grid current L3", "c_i", .001, "A", "A", ""),
			smaAddress(30803, 2, "U32", "Power frequency (Fac)", "elec_freq", .01, "Hz", "Hz", ""),
			smaAddress(30805, 2, "S32", "Reactive power (Qac)", "reactive_power", .01, "var", "kvar", ""),
			smaAddress(30813, 2, "S32", "Apparent power (Sac)", "apparent_power", 1, "VA", "kVA", ""),
			smaAddress(30821, 2, "U32", "Displacement power factor (PF)", "power_factor", .01, "", "", ""),
			smaAddress(30823, 2, "U32", "Excitation type of cos phi", "excitation_type", 1, "", "", "1041=capacitive, 1042=inductive"),
			smaAddress(30841, 2, "U32", "AC voltage (avg all phases)", "ac_voltage_average", .01, "V", "V", ""),
			smaAddress(34109, 2, "S32", "Heat sink temperature", "temperature", .1, "C", "C", ""),
			smaAddress(34113, 2, "S32", "Interior temperature 1", "interior_temperature", .1, "C", "C", ""),
		},
	}
}

func smaAddress(register, length int, dataType, description, key string, factor float64, sourceUnit, canonicalUnit, remark string) domain.Address {
	return domain.Address{FunctionCode: 3, Register: register, Description: description, CanonicalKey: key, SourceTag: description, Factor: factor, DataType: dataType, Length: length, SourceUnit: sourceUnit, CanonicalUnit: canonicalUnit, Remark: remark, Enabled: true, EnabledSet: true}
}
