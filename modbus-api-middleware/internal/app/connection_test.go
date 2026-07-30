package app

import (
	"encoding/binary"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/modbus"
	"chpp/modbus-api-middleware/internal/store"
)

func TestPollConnectionUsesTableAddressFields(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		request := make([]byte, 12)
		if _, e = conn.Read(request); e != nil {
			return
		}
		response := make([]byte, 13)
		copy(response[:2], request[:2])
		binary.BigEndian.PutUint16(response[4:6], 7)
		response[6] = request[6]
		response[7] = request[7]
		response[8] = 4
		binary.BigEndian.PutUint16(response[9:11], 42000)
		binary.BigEndian.PutUint16(response[11:13], 123)
		_, _ = conn.Write(response)
	}()

	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	brand, err := st.SaveBrand(domain.Brand{BrandName: "ABB"})
	if err != nil {
		t.Fatal(err)
	}
	set, err := st.SaveDeviceSet(domain.DeviceSet{
		BrandID:  brand.BrandID,
		DevType:  "Inverter",
		DevModel: "PVS100",
		Addresses: []domain.Address{
			{FunctionCode: 3, Register: 0, Description: "Active power", Factor: .001, DataType: "USHORT"},
			{FunctionCode: 3, Register: 1, Description: "Collect DSP data", Factor: 1, DataType: "USHORT"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	connection, err := st.SaveConnection(domain.ConnectionConfig{ConnectionName: "AICA-INV-01", Host: "127.0.0.1", Port: port, DeviceSetID: set.DeviceSetID})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: st, Client: &modbus.Client{Timeout: time.Second}}
	reading, measurements, err := svc.PollConnection(fmtID(connection.ConnectionID))
	if err != nil {
		t.Fatal(err)
	}
	if reading.DevDn != "AICA-INV-01" || reading.PlantCode != "AICA" || reading.DevTypeID != 1 {
		t.Fatalf("reading identity=%+v", reading)
	}
	if got := reading.RegisterAddressMap["0"]; got != 42000 {
		t.Fatalf("register 0=%v want 42000", got)
	}
	if got := reading.RegisterAddressMap["1"]; got != 123 {
		t.Fatalf("register 1=%v want 123", got)
	}
	if len(measurements) != 2 || measurements[0].RawValue != 42000 || measurements[0].Quality != "GOOD" || measurements[1].RegisterAddress != 1 {
		t.Fatalf("measurements=%+v", measurements)
	}
	if queued, e := svc.Enqueue(reading, "MOXA-AICA-01"); e != nil || !queued {
		t.Fatalf("enqueue=%v err=%v", queued, e)
	}
	events, e := st.Ready(1)
	if e != nil || len(events) != 1 || !strings.Contains(events[0].Payload, `"gatewayId":"MOXA-AICA-01"`) {
		t.Fatalf("events=%+v err=%v", events, e)
	}
	offline := reading
	offline.CollectTime++
	offline.RegisterAddressMap = map[string]float64{"0": 0}
	if queued, e := svc.Enqueue(offline, "MOXA-AICA-01"); e != nil || !queued {
		t.Fatalf("offline enqueue=%v err=%v", queued, e)
	}
}

func TestPollSMAContinuesWhenOneAddressIsUnsupported(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for i := 0; i < 2; i++ {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			request := make([]byte, 12)
			if _, e = io.ReadFull(conn, request); e != nil {
				conn.Close()
				return
			}
			if i == 0 {
				response := make([]byte, 9)
				copy(response[:2], request[:2])
				binary.BigEndian.PutUint16(response[4:6], 3)
				response[6] = request[6]
				response[7], response[8] = request[7]|0x80, 2
				_, _ = conn.Write(response)
			} else {
				response := make([]byte, 13)
				copy(response[:2], request[:2])
				binary.BigEndian.PutUint16(response[4:6], 7)
				response[6], response[7], response[8] = request[6], request[7], 4
				binary.BigEndian.PutUint16(response[11:13], 123)
				_, _ = conn.Write(response)
			}
			conn.Close()
		}
	}()

	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	brand, _ := st.SaveBrand(domain.Brand{BrandName: "SMA"})
	set, err := st.SaveDeviceSet(domain.DeviceSet{BrandID: brand.BrandID, DevType: "Inverter", DevModel: "Sunny Central", AddressMode: "SMA", Addresses: []domain.Address{
		{FunctionCode: 3, Register: 30057, Description: "Serial number", CanonicalKey: "serial_number", Factor: 1, DataType: "SMA_UINT32"},
		{FunctionCode: 3, Register: 30193, Description: "System time", CanonicalKey: "system_time", Factor: 1, DataType: "SMA_UINT32"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	connection, err := st.SaveConnection(domain.ConnectionConfig{ConnectionName: "SMA-01", Host: "127.0.0.1", Port: port, UnitID: 43, DeviceSetID: set.DeviceSetID})
	if err != nil {
		t.Fatal(err)
	}
	reading, measurements, err := (&Service{Store: st, Client: &modbus.Client{Timeout: time.Second}}).PollConnection(fmtID(connection.ConnectionID))
	if err != nil || len(measurements) != 1 || measurements[0].RawValue != 123 {
		t.Fatalf("reading=%+v measurements=%+v err=%v", reading, measurements, err)
	}
}

func TestPollConnectionFallsBackPerAddressAndKeepsFCAndUnitID(t *testing.T) {
	type request struct{ fc, start, quantity, unit int }
	requests := make(chan request, 4)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for i := 0; i < 4; i++ {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			packet := make([]byte, 12)
			if _, e = io.ReadFull(conn, packet); e != nil {
				conn.Close()
				return
			}
			requests <- request{int(packet[7]), int(binary.BigEndian.Uint16(packet[8:10])), int(binary.BigEndian.Uint16(packet[10:12])), int(packet[6])}
			var pdu []byte
			if i == 0 {
				pdu = []byte{packet[7] | 0x80, 2}
			} else {
				pdu = []byte{packet[7], 2, 0, byte(10 + i)}
			}
			response := make([]byte, 7+len(pdu))
			copy(response[:2], packet[:2])
			binary.BigEndian.PutUint16(response[4:6], uint16(1+len(pdu)))
			response[6] = packet[6]
			copy(response[7:], pdu)
			_, _ = conn.Write(response)
			conn.Close()
		}
	}()

	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	brand, _ := st.SaveBrand(domain.Brand{BrandName: "Generic"})
	set, err := st.SaveDeviceSet(domain.DeviceSet{BrandID: brand.BrandID, DevType: "Inverter", DevModel: "Fallback", AddressMode: "VENDOR_RAW", MaxBlockSize: 30, Addresses: []domain.Address{
		{FunctionCode: 3, Register: 100, Description: "Active power", Factor: 1, DataType: "USHORT"},
		{FunctionCode: 3, Register: 101, Description: "Reactive power", Factor: 1, DataType: "USHORT"},
		{FunctionCode: 4, Register: 200, Description: "Power factor", Factor: 1, DataType: "USHORT"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	connection, err := st.SaveConnection(domain.ConnectionConfig{ConnectionName: "GEN-01", Host: "127.0.0.1", Port: port, UnitID: 43, DeviceSetID: set.DeviceSetID})
	if err != nil {
		t.Fatal(err)
	}
	reading, measurements, err := (&Service{Store: st, Client: &modbus.Client{Timeout: time.Second}}).PollConnection(fmtID(connection.ConnectionID))
	if err != nil || len(measurements) != 3 {
		t.Fatalf("reading=%+v measurements=%+v err=%v", reading, measurements, err)
	}
	want := []request{{3, 100, 2, 43}, {3, 100, 1, 43}, {3, 101, 1, 43}, {4, 200, 1, 43}}
	for i, expected := range want {
		if got := <-requests; got != expected {
			t.Fatalf("request %d=%+v want %+v", i, got, expected)
		}
	}
	logs, err := st.PollLogs(connection.ConnectionID, 10)
	if err != nil || len(logs) != 1 || logs[0].Status != "WARN" || !strings.Contains(logs[0].Detail, `"startAddress":100`) {
		t.Fatalf("logs=%+v err=%v", logs, err)
	}
}

func TestPollConnectionHints40001AddressMode(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		packet := make([]byte, 12)
		if _, e = io.ReadFull(conn, packet); e != nil {
			return
		}
		response := make([]byte, 9)
		copy(response[:2], packet[:2])
		binary.BigEndian.PutUint16(response[4:6], 3)
		response[6] = packet[6]
		response[7], response[8] = packet[7]|0x80, 3
		_, _ = conn.Write(response)
	}()

	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	brand, _ := st.SaveBrand(domain.Brand{BrandName: "Generic"})
	set, err := st.SaveDeviceSet(domain.DeviceSet{BrandID: brand.BrandID, DevType: "Inverter", DevModel: "Real Plant", AddressMode: "VENDOR_RAW", Addresses: []domain.Address{
		{FunctionCode: 3, Register: 41441, Description: "Active power", Factor: 1, DataType: "USHORT"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	connection, err := st.SaveConnection(domain.ConnectionConfig{ConnectionName: "REAL-INV-01", Host: "127.0.0.1", Port: port, UnitID: 2, DeviceSetID: set.DeviceSetID})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = (&Service{Store: st, Client: &modbus.Client{Timeout: time.Second}}).PollConnection(fmtID(connection.ConnectionID))
	if err == nil || !strings.Contains(err.Error(), "fc=03 start=41441 quantity=1 unit=2") {
		t.Fatalf("err=%v", err)
	}
	logs, err := st.PollLogs(connection.ConnectionID, 10)
	if err != nil || len(logs) != 1 || !strings.Contains(logs[0].Detail, "REGISTER_40001") {
		t.Fatalf("logs=%+v err=%v", logs, err)
	}
}
func fmtID(v int64) string {
	var b [20]byte
	i := len(b)
	for {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
		if v == 0 {
			return string(b[i:])
		}
	}
}
