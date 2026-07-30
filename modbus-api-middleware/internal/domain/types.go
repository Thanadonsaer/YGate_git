package domain

type RegisterDefinition struct {
	Key             string  `json:"key"`
	SourceTag       string  `json:"sourceTag"`
	DataType        string  `json:"dataType"`
	SourceUnit      string  `json:"sourceUnit"`
	CanonicalUnit   string  `json:"canonicalUnit"`
	RegisterAddress int     `json:"registerAddress"`
	FunctionCode    int     `json:"functionCode"`
	Length          int     `json:"length"`
	Scale           float64 `json:"scale"`
	Offset          float64 `json:"offset"`
	WordOrder       string  `json:"wordOrder,omitempty"`
	Enabled         bool    `json:"enabled"`
}

type RegisterProfile struct {
	ProfileID           string               `json:"profileId"`
	ProfileName         string               `json:"profileName"`
	AddressMode         string               `json:"addressMode"`
	ByteOrder           string               `json:"byteOrder"`
	WordOrder           string               `json:"wordOrder"`
	MaxBlockSize        int                  `json:"maxBlockSize,omitempty"`
	DevTypeID           int                  `json:"devTypeId"`
	DefaultFunctionCode int                  `json:"defaultFunctionCode"`
	Enabled             bool                 `json:"enabled"`
	Registers           []RegisterDefinition `json:"registers"`
}

type Connection struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	UnitID int    `json:"unitId"`
}

type Device struct {
	DeviceID          string     `json:"deviceId"`
	DeviceName        string     `json:"deviceName"`
	PlantCode         string     `json:"plantCode"`
	PlantName         string     `json:"plantName"`
	RegisterProfileID string     `json:"registerProfileId"`
	DevTypeID         int        `json:"devTypeId"`
	Enabled           bool       `json:"enabled"`
	Connection        Connection `json:"connection"`
}

type RegisterSample struct {
	FunctionCode    int     `json:"functionCode"`
	RegisterAddress int     `json:"registerAddress"`
	DataType        string  `json:"dataType"`
	RawValue        float64 `json:"rawValue"`
	Quality         string  `json:"quality"`
}

// RegisterAddressMap is keyed by register address only (e.g. "40072"), value
// is the decoded raw register value. Function code, source data type and
// quality are not sent per reading: function code is fixed per profile at
// config time (VENDOR_RAW address mode, always FC03/FC04 as configured — see
// profile.ResolveModbusAddress), data type and scaling are owned by the
// Central Platform's Register Metadata, and quality is always GOOD in
// practice since failed reads are excluded from the map entirely (see
// decodeEntry in app/service.go, which only ever writes on success).
// ponytail: dropping the function-code prefix means two profiles that mix
// FC03 and FC04 on the same register address would collide in this map; no
// configured device set does that today (all addresses use address_mode
// VENDOR_RAW with FC03). Re-add an "fc:register" key if that ever changes.
type Reading struct {
	GatewayID          string             `json:"gatewayId"`
	DevDn              string             `json:"devDn"`
	DevName            string             `json:"devName,omitempty"`
	PlantCode          string             `json:"plantCode"`
	PlantName          string             `json:"plantName,omitempty"`
	DevTypeID          int                `json:"devTypeId"`
	Model              string             `json:"model,omitempty"`
	CollectTime        int64              `json:"collectTime"`
	RegisterAddressMap map[string]float64 `json:"registerAddressMap"`
}

type Measurement = RegisterSample
