package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/store"
)

func TestImportDeviceSetAddressCSV(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	csv := "brand_name,dev_type,dev_model,address_fc,register,description,factor,data_type,remark\n" +
		"Huawei,Inverter,SUN2000,03,32080,Active power,0.001,SW_INT,\n" +
		"Huawei,Inverter,SUN2000,03,32002,Collect DSP data,1,SHORT,\n"
	body, _ := json.Marshal(csvImportRequest{CSV: csv})
	req := httptest.NewRequest(http.MethodPost, "/api/import-device-set-address", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	(&Server{Store: st}).FullHandler().ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	sets, err := st.DeviceSets()
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 1 || len(sets[0].Addresses) != 2 {
		t.Fatalf("sets=%+v", sets)
	}
	if sets[0].Addresses[0].Register != 2002 || sets[0].Addresses[1].Register != 2080 {
		t.Fatalf("addresses not normalized/sorted=%+v", sets[0].Addresses)
	}
}

func TestImportExtendedModbusConfiguration(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	csv := "brand_name,dev_type,dev_model,address_mode,byte_order,word_order,max_block_size,address_fc,register,description,canonical_key,source_tag,factor,offset,data_type,address_length,address_word_order,source_unit,canonical_unit,enabled,remark\n" +
		"SMA,Inverter,Sunny Central,SMA,BIG_ENDIAN,HIGH_LOW,20,03,30057,Serial number,serial_number,Serial number,1,2,U32,2,LOW_HIGH, W , kW ,false,test\n"
	summary, err := (&Server{Store: st}).importCSV(csv)
	if err != nil || summary.DeviceSets != 1 || summary.Addresses != 1 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	sets, err := st.DeviceSets()
	if err != nil || len(sets) != 1 {
		t.Fatalf("sets=%+v err=%v", sets, err)
	}
	set, address := sets[0], sets[0].Addresses[0]
	if set.AddressMode != "SMA" || set.MaxBlockSize != 20 || address.Register != 30057 || address.Length != 2 || address.WordOrder != "LOW_HIGH" || address.Offset != 2 || address.SourceUnit != "W" || address.CanonicalUnit != "kW" || address.Enabled {
		t.Fatalf("set=%+v address=%+v", set, address)
	}
}

func TestImportAcceptsExcelStyleHeadersAndState(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	csv := "brand name,dev type,dev model,address mode,address fc,Register,Description,Factor,Data Type,State\n" +
		"ABB,Inverter,PVS100,RAW,03,40084,Watts,0.01,SHORT,\"use\"\n" +
		"ABB,Inverter,PVS100,RAW,03,40110,EVENT_1,1,LONG,\"not use\"\n"
	summary, err := (&Server{Store: st}).importCSV(csv)
	if err != nil || summary.DeviceSets != 1 || summary.Addresses != 2 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	sets, err := st.DeviceSets()
	if err != nil || len(sets) != 1 {
		t.Fatalf("sets=%+v err=%v", sets, err)
	}
	if sets[0].AddressMode != "VENDOR_RAW" || sets[0].Addresses[0].CanonicalKey != "3:40084" || !sets[0].Addresses[0].Enabled || sets[0].Addresses[1].DataType != "LONG" || sets[0].Addresses[1].Enabled {
		t.Fatalf("sets=%+v", sets)
	}
}

func TestImportDefaultsBlankAddressFCToReadHoldingRegister(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	csv := "brand_name,dev_type,dev_model,address_mode,address_fc,register,description,canonical_key,factor,data_type,enabled\n" +
		"Huawei,Inverter,SUN2000,VENDOR_RAW,,40196,Active power adjustment,active_power_adjustment,1,SHORT,true\n"
	summary, err := (&Server{Store: st}).importCSV(csv)
	if err != nil || summary.Addresses != 1 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	sets, err := st.DeviceSets()
	if err != nil || len(sets) != 1 {
		t.Fatalf("sets=%+v err=%v", sets, err)
	}
	if sets[0].Addresses[0].FunctionCode != 3 || sets[0].Addresses[0].Register != 40196 {
		t.Fatalf("address=%+v", sets[0].Addresses[0])
	}
}
func TestDeviceSetAddressTemplateIncludesUnitColumns(t *testing.T) {
	res := httptest.NewRecorder()
	(&Server{}).templateCSV(res, httptest.NewRequest(http.MethodGet, "/template/device-set-address.csv", nil))
	rows, err := connectionRows(res.Body.String())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"brand_name", "dev_type", "dev_model", "address_mode", "address_fc", "register", "description", "canonical_key", "factor", "data_type", "source_unit", "canonical_unit", "enabled", "remark"}
	if res.Code != http.StatusOK || len(rows) != 11 || len(rows[0]) != len(want) {
		t.Fatalf("status=%d rows=%d header=%v", res.Code, len(rows), rows[0])
	}
	for i := range want {
		if rows[0][i] != want[i] {
			t.Fatalf("header=%v", rows[0])
		}
	}
}
func TestImportConnectionsRequiresExistingPlant(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	brand, _ := st.SaveBrand(domain.Brand{BrandName: "Huawei"})
	set, err := st.SaveDeviceSet(domain.DeviceSet{BrandID: brand.BrandID, DevType: "Inverter", DevModel: "SUN2000", Addresses: []domain.Address{{FunctionCode: 3, Register: 80, Description: "Active power", Factor: 1, DataType: "SHORT"}}})
	if err != nil {
		t.Fatal(err)
	}
	csv := "connection_name,plant_code,device_code,device_name,brand_name,dev_type,dev_model,host,port,unit_id,enabled\n" +
		"VT1-INV-01,VT1,VT1-INV-01,Inverter 01,Huawei,Inverter,SUN2000,192.168.1.200,502,1,true\n"
	server := &Server{Store: st}
	if _, err = server.importConnectionCSV(csv); err == nil {
		t.Fatal("expected missing plant error")
	}
	if _, err = st.SavePlant(domain.Plant{PlantCode: "VT1", PlantName: "CHPP VT1"}); err != nil {
		t.Fatal(err)
	}
	templateReq := httptest.NewRequest(http.MethodGet, "/template/devices.csv?plantCode=VT1", nil)
	templateRes := httptest.NewRecorder()
	server.deviceTemplateCSV(templateRes, templateReq)
	rows, err := connectionRows(templateRes.Body.String())
	if err != nil || templateRes.Code != http.StatusOK || len(rows) != 2 || rows[0][1] != "device_code" {
		t.Fatalf("invalid CSV template: status=%d rows=%v err=%v", templateRes.Code, rows, err)
	}
	summary, err := server.importConnectionCSV(csv)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Connections != 1 {
		t.Fatalf("summary=%+v", summary)
	}
	scopedCSV := "connection_name,device_code,device_name,brand_name,dev_type,dev_model,host,port,unit_id,enabled\n" +
		"VT1-INV-02,VT1-INV-02,Inverter 02,Huawei,Inverter,SUN2000-100KTL-M1,192.168.1.201,502,2,true\n"
	summary, err = server.importConnectionCSV(scopedCSV, "VT1")
	if err != nil || summary.Connections != 1 {
		t.Fatalf("scoped import summary=%+v err=%v", summary, err)
	}
	idCSV := "connection_name,device_code,device_name,device_set_id,host,port,unit_id,enabled\n" +
		fmt.Sprintf("VT1-INV-03,VT1-INV-03,Inverter 03,%d,192.168.1.202,502,3,true\n", set.DeviceSetID)
	summary, err = server.importConnectionCSV(idCSV, "VT1")
	if err != nil || summary.Connections != 1 {
		t.Fatalf("device_set_id import summary=%+v err=%v", summary, err)
	}
	connection, err := st.Connection(1)
	if err != nil {
		t.Fatal(err)
	}
	if connection.DeviceSetID != set.DeviceSetID || connection.PlantName != "CHPP VT1" || connection.DevDn != "VT1-INV-01" {
		t.Fatalf("connection=%+v", connection)
	}
	plants, _ := st.Plants()
	if _, err = st.SavePlant(domain.Plant{PlantID: plants[0].PlantID, PlantCode: "VT2", PlantName: "CHPP VT2"}); err != nil {
		t.Fatal(err)
	}
	connection, _ = st.Connection(connection.ConnectionID)
	if connection.PlantCode != "VT2" || connection.PlantName != "CHPP VT2" {
		t.Fatalf("updated plant not applied to connection: %+v", connection)
	}
}
