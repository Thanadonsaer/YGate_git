package modbus

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"
)

type ProbeInfo struct {
	Protocol       string `json:"protocol"`
	Transport      string `json:"transport"`
	FunctionCodes  []int  `json:"functionCodes"`
	RegisterFamily string `json:"registerFamily"`
	Detail         string `json:"detail"`
}
type Client struct {
	Timeout     time.Duration
	transaction atomic.Uint32
}

func (c *Client) Read(host string, port, unitID, functionCode int, address, quantity uint16) ([]byte, time.Duration, error) {
	if functionCode != 3 && functionCode != 4 {
		return nil, 0, fmt.Errorf("unsupported function code %d", functionCode)
	}
	if quantity == 0 || quantity > 125 {
		return nil, 0, fmt.Errorf("quantity must be 1..125")
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}
	started := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprint(port)), timeout)
	if err != nil {
		return nil, time.Since(started), err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	tx := uint16(c.transaction.Add(1))
	req := make([]byte, 12)
	binary.BigEndian.PutUint16(req[0:2], tx)
	binary.BigEndian.PutUint16(req[4:6], 6)
	req[6] = byte(unitID)
	req[7] = byte(functionCode)
	binary.BigEndian.PutUint16(req[8:10], address)
	binary.BigEndian.PutUint16(req[10:12], quantity)
	if _, err = conn.Write(req); err != nil {
		return nil, time.Since(started), err
	}
	header := make([]byte, 7)
	if _, err = io.ReadFull(conn, header); err != nil {
		return nil, time.Since(started), err
	}
	if binary.BigEndian.Uint16(header[:2]) != tx {
		return nil, time.Since(started), fmt.Errorf("transaction id mismatch")
	}
	length := int(binary.BigEndian.Uint16(header[4:6])) - 1
	if length < 2 || length > 252 {
		return nil, time.Since(started), fmt.Errorf("invalid response length")
	}
	pdu := make([]byte, length)
	if _, err = io.ReadFull(conn, pdu); err != nil {
		return nil, time.Since(started), err
	}
	if pdu[0]&0x80 != 0 {
		return nil, time.Since(started), fmt.Errorf("modbus exception %d", pdu[1])
	}
	if pdu[0] != byte(functionCode) {
		return nil, time.Since(started), fmt.Errorf("function code mismatch")
	}
	if int(pdu[1]) != len(pdu)-2 {
		return nil, time.Since(started), fmt.Errorf("invalid byte count")
	}
	return pdu[2:], time.Since(started), nil
}

func (c *Client) Probe(host string, port, unitID int) (time.Duration, error) {
	_, latency, err := c.ProbeDetails(host, port, unitID)
	return latency, err
}

func (c *Client) ProbeDetails(host string, port, unitID int) (ProbeInfo, time.Duration, error) {
	info := ProbeInfo{Protocol: "Modbus TCP", Transport: "TCP"}
	var lastLatency time.Duration
	var details []string
	for _, fc := range []int{3, 4} {
		pdu, latency, err := c.probeFunction(host, port, unitID, fc)
		lastLatency = latency
		if err != nil {
			details = append(details, fmt.Sprintf("FC%02d no response", fc))
			continue
		}
		if pdu[0]&0x80 != 0 {
			code := 0
			if len(pdu) > 1 {
				code = int(pdu[1])
			}
			details = append(details, fmt.Sprintf("FC%02d exception %d", fc, code))
			if code != 1 {
				info.FunctionCodes = append(info.FunctionCodes, fc)
			}
			continue
		}
		info.FunctionCodes = append(info.FunctionCodes, fc)
		details = append(details, fmt.Sprintf("FC%02d normal", fc))
	}
	info.RegisterFamily = registerFamily(info.FunctionCodes)
	info.Detail = joinDetails(details)
	if len(info.FunctionCodes) == 0 {
		return info, lastLatency, fmt.Errorf("no Modbus FC03/FC04 response")
	}
	return info, lastLatency, nil
}

func (c *Client) probeFunction(host string, port, unitID, functionCode int) ([]byte, time.Duration, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}
	started := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprint(port)), timeout)
	if err != nil {
		return nil, time.Since(started), err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	tx := uint16(c.transaction.Add(1))
	req := make([]byte, 12)
	binary.BigEndian.PutUint16(req[0:2], tx)
	binary.BigEndian.PutUint16(req[4:6], 6)
	req[6] = byte(unitID)
	req[7] = byte(functionCode)
	binary.BigEndian.PutUint16(req[10:12], 1)
	if _, err = conn.Write(req); err != nil {
		return nil, time.Since(started), err
	}
	header := make([]byte, 7)
	if _, err = io.ReadFull(conn, header); err != nil {
		return nil, time.Since(started), err
	}
	if binary.BigEndian.Uint16(header[:2]) != tx {
		return nil, time.Since(started), fmt.Errorf("transaction id mismatch")
	}
	length := int(binary.BigEndian.Uint16(header[4:6])) - 1
	if length < 2 || length > 252 {
		return nil, time.Since(started), fmt.Errorf("invalid response length")
	}
	pdu := make([]byte, length)
	if _, err = io.ReadFull(conn, pdu); err != nil {
		return nil, time.Since(started), err
	}
	if pdu[0] != byte(functionCode) && pdu[0] != byte(functionCode|0x80) {
		return nil, time.Since(started), fmt.Errorf("function code mismatch")
	}
	return pdu, time.Since(started), nil
}

func registerFamily(functions []int) string {
	has3, has4 := false, false
	for _, fc := range functions {
		has3 = has3 || fc == 3
		has4 = has4 || fc == 4
	}
	switch {
	case has3 && has4:
		return "Holding/Input registers (4x/3x)"
	case has3:
		return "Holding registers (4x / FC03)"
	case has4:
		return "Input registers (3x / FC04)"
	default:
		return "unknown"
	}
}

func joinDetails(values []string) string {
	out := ""
	for _, value := range values {
		if out != "" {
			out += "; "
		}
		out += value
	}
	return out
}
