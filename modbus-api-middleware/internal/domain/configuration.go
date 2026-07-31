package domain

import "encoding/json"

type Brand struct {
	BrandID           int64   `json:"brandId"`
	BrandName         string  `json:"brandName"`
	BrandDescription  string  `json:"brandDescription,omitempty"`
	BrandDevSetIDList []int64 `json:"brandDevSetList,omitempty"`
}

type Plant struct {
	PlantID   int64  `json:"plantId"`
	PlantCode string `json:"plantCode"`
	PlantName string `json:"plantName"`
}

type Address struct {
	AddressID     int64   `json:"addressId"`
	DeviceSetID   int64   `json:"deviceSetId"`
	FunctionCode  int     `json:"functionCode"`
	Register      int     `json:"register"`
	Description   string  `json:"description"`
	CanonicalKey  string  `json:"canonicalKey,omitempty"`
	SourceTag     string  `json:"sourceTag,omitempty"`
	Factor        float64 `json:"factor"`
	Offset        float64 `json:"offset,omitempty"`
	DataType      string  `json:"dataType"`
	Length        int     `json:"length,omitempty"`
	WordOrder     string  `json:"wordOrder,omitempty"`
	SourceUnit    string  `json:"sourceUnit,omitempty"`
	CanonicalUnit string  `json:"canonicalUnit,omitempty"`
	Remark        string  `json:"remark,omitempty"`
	Enabled       bool    `json:"enabled,omitempty"`
	EnabledSet    bool    `json:"-"`
}

func (a *Address) UnmarshalJSON(data []byte) error {
	type alias Address
	v := struct {
		*alias
		Enabled *bool `json:"enabled"`
	}{alias: (*alias)(a)}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if v.Enabled != nil {
		a.Enabled, a.EnabledSet = *v.Enabled, true
	}
	return nil
}

type DeviceSet struct {
	DeviceSetID   int64     `json:"deviceSetId"`
	BrandID       int64     `json:"brandId,omitempty"`
	BrandName     string    `json:"brandName,omitempty"`
	DevTypeID     int       `json:"devTypeId,omitempty"`
	DevType       string    `json:"devType"`
	DevModel      string    `json:"devModel"`
	AddressIDList []int64   `json:"addressIdList,omitempty"`
	AddressMode   string    `json:"addressMode,omitempty"`
	ByteOrder     string    `json:"byteOrder,omitempty"`
	WordOrder     string    `json:"wordOrder,omitempty"`
	MaxBlockSize  int       `json:"maxBlockSize,omitempty"`
	Addresses     []Address `json:"addresses"`
}

// ConfigSnapshot is the full-replace config payload exchanged with the
// Central Platform over the realtime channel: pushed down as
// config.snapshot and reported back up during onboarding. Field shapes and
// JSON names match platform-api's MiddlewareConfigSnapshot exactly, so no
// translation step is needed on either side.
type ConfigSnapshot struct {
	Version     int64              `json:"version"`
	Brands      []Brand            `json:"brands"`
	DeviceSets  []DeviceSet        `json:"deviceSets"`
	Connections []ConnectionConfig `json:"connections"`
	Plants      []Plant            `json:"plants"`
}

type ConnectionConfig struct {
	ConnectionID   int64  `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	UnitID         int    `json:"unitId,omitempty"`
	SlaveID        int    `json:"slaveId,omitempty"`
	DeviceSetID    int64  `json:"deviceSetId"`
	DeviceSetName  string `json:"deviceSetName,omitempty"`
	DevTypeID      int    `json:"devTypeId,omitempty"`
	DevDn          string `json:"devDn,omitempty"`
	DeviceName     string `json:"deviceName,omitempty"`
	PlantCode      string `json:"plantCode,omitempty"`
	PlantName      string `json:"plantName,omitempty"`
	Enabled        bool   `json:"enabled"`
}
