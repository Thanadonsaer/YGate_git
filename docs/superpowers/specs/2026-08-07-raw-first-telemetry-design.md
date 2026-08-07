# Raw-first Telemetry Design

Middleware sends decoded register values unchanged. Platform persists them in `telemetry.raw_register_reading` and calculates `raw * scale + value_offset` from enabled device/model Register Metadata when reading Latest or History. Existing calculated telemetry remains only as a compatibility fallback when no Raw row exists.

