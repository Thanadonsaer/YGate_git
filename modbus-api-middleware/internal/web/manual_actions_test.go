package web

import (
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"chpp/modbus-api-middleware/internal/app"
	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/modbus"
	"chpp/modbus-api-middleware/internal/store"
)

func TestReadNowDoesNotEnqueueAPI(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	brand, _ := st.SaveBrand(domain.Brand{BrandName: "Test"})
	set, err := st.SaveDeviceSet(domain.DeviceSet{BrandID: brand.BrandID, DevType: "Inverter", DevModel: "SIM", Addresses: []domain.Address{{FunctionCode: 3, Register: 0, Description: "Active power", CanonicalKey: "active_power", Factor: 1, DataType: "SHORT"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.SavePlant(domain.Plant{PlantCode: "VT1", PlantName: "VT1"}); err != nil {
		t.Fatal(err)
	}
	addr, stop := readServer(t)
	defer stop()
	host, portText, _ := net.SplitHostPort(addr)
	port, _ := net.LookupPort("tcp", portText)
	conn, err := st.SaveConnection(domain.ConnectionConfig{ConnectionName: "VT1-INV-01", Host: host, Port: port, UnitID: 1, DeviceSetID: set.DeviceSetID, PlantCode: "VT1", DevDn: "VT1-INV-01", DeviceName: "INV 01"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, App: &app.Service{Store: st, Client: &modbus.Client{Timeout: time.Second}}}
	res := httptest.NewRecorder()
	s.readNow(res, httptest.NewRequest(http.MethodPost, "/api/read-now/1", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	events, err := st.Ready(10)
	if err != nil || len(events) != 0 {
		t.Fatalf("readNow queued events=%d err=%v connection=%+v", len(events), err, conn)
	}
}

func readServer(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				req := make([]byte, 12)
				if _, err := io.ReadFull(conn, req); err != nil {
					return
				}
				pdu := []byte{req[7], 2, 0, 42}
				res := make([]byte, 7+len(pdu))
				copy(res[:2], req[:2])
				binary.BigEndian.PutUint16(res[4:6], uint16(1+len(pdu)))
				res[6] = req[6]
				copy(res[7:], pdu)
				_, _ = conn.Write(res)
			}()
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}
