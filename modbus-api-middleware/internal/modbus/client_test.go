package modbus

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestProbeAcceptsNormalAndExceptionResponses(t *testing.T) {
	for _, exception := range []bool{false, true} {
		addr, stop := modbusProbeServer(t, exception)
		t.Cleanup(stop)
		host, portText, _ := net.SplitHostPort(addr)
		port, _ := net.LookupPort("tcp", portText)
		if _, err := (&Client{Timeout: time.Second}).Probe(host, port, 1); err != nil {
			t.Fatalf("Probe(exception=%v) error: %v", exception, err)
		}
	}
}

func modbusProbeServer(t *testing.T, exception bool) (string, func()) {
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
				if _, err = io.ReadFull(conn, req); err != nil {
					return
				}
				fc := req[7]
				pdu := []byte{fc, 2, 0, 1}
				if exception {
					pdu = []byte{fc | 0x80, 2}
				}
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
