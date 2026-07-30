package web

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"chpp/modbus-api-middleware/internal/modbus"
)

type modbusDiscoveryRequest struct {
	CIDR      string `json:"cidr"`
	Ports     []int  `json:"ports"`
	UnitID    int    `json:"unitId"`
	TimeoutMS int    `json:"timeoutMs"`
}

type modbusDiscoveryResult struct {
	Host                 string `json:"host"`
	Port                 int    `json:"port"`
	UnitID               int    `json:"unitId"`
	LatencyMS            int64  `json:"latencyMs"`
	Status               string `json:"status"`
	Protocol             string `json:"protocol,omitempty"`
	Transport            string `json:"transport,omitempty"`
	FunctionCodes        []int  `json:"functionCodes,omitempty"`
	RegisterFamily       string `json:"registerFamily,omitempty"`
	Detail               string `json:"detail,omitempty"`
	ExistingConnectionID int64  `json:"existingConnectionId,omitempty"`
	ExistingPlantCode    string `json:"existingPlantCode,omitempty"`
}

func (s *Server) modbusDiscovery(w http.ResponseWriter, r *http.Request) {
	var req modbusDiscoveryRequest
	if !decode(w, r, &req) {
		return
	}
	hosts, ports, unitID, timeout, err := normalizeDiscovery(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	existing := map[string]modbusDiscoveryResult{}
	connections, _ := s.Store.ConnectionsWithStatus()
	for _, c := range connections {
		existing[fmt.Sprintf("%s:%d", c.Host, c.Port)] = modbusDiscoveryResult{ExistingConnectionID: c.ConnectionID, ExistingPlantCode: c.PlantCode}
	}
	client := &modbus.Client{Timeout: timeout}
	type job struct {
		host string
		port int
	}
	jobs := make(chan job)
	results := make(chan modbusDiscoveryResult)
	var wg sync.WaitGroup
	workers := min(64, len(hosts)*len(ports))
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				info, latency, e := client.ProbeDetails(j.host, j.port, unitID)
				if e != nil {
					continue
				}
				found := modbusDiscoveryResult{Host: j.host, Port: j.port, UnitID: unitID, LatencyMS: latency.Milliseconds(), Status: "MODBUS_OK", Protocol: info.Protocol, Transport: info.Transport, FunctionCodes: info.FunctionCodes, RegisterFamily: info.RegisterFamily, Detail: info.Detail}
				if old := existing[fmt.Sprintf("%s:%d", j.host, j.port)]; old.ExistingConnectionID != 0 {
					found.ExistingConnectionID = old.ExistingConnectionID
					found.ExistingPlantCode = old.ExistingPlantCode
					found.Status = "EXISTING"
				}
				results <- found
			}
		}()
	}
	go func() {
		for _, host := range hosts {
			for _, port := range ports {
				select {
				case <-r.Context().Done():
					close(jobs)
					return
				case jobs <- job{host: host, port: port}:
				}
			}
		}
		close(jobs)
	}()
	go func() {
		wg.Wait()
		close(results)
	}()
	out := []modbusDiscoveryResult{}
	for found := range results {
		out = append(out, found)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Host == out[j].Host {
			return out[i].Port < out[j].Port
		}
		return out[i].Host < out[j].Host
	})
	writeJSON(w, http.StatusOK, out)
}

func normalizeDiscovery(req modbusDiscoveryRequest) ([]string, []int, int, time.Duration, error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(req.CIDR))
	if err != nil || !prefix.Addr().Is4() {
		return nil, nil, 0, 0, fmt.Errorf("valid IPv4 CIDR is required")
	}
	prefix = prefix.Masked()
	addresses := uint64(1) << (32 - prefix.Bits())
	if addresses > 1024 {
		return nil, nil, 0, 0, fmt.Errorf("CIDR supports up to 1024 addresses")
	}
	ports := uniquePorts(req.Ports)
	if len(ports) == 0 || len(ports) > 20 {
		return nil, nil, 0, 0, fmt.Errorf("ports must contain 1..20 valid port(s)")
	}
	unitID := req.UnitID
	if unitID <= 0 {
		unitID = 1
	}
	if unitID > 255 {
		return nil, nil, 0, 0, fmt.Errorf("unitId must be 1..255")
	}
	timeoutMS := req.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 500
	}
	if timeoutMS < 100 {
		timeoutMS = 100
	}
	if timeoutMS > 5000 {
		timeoutMS = 5000
	}
	base := prefix.Addr().As4()
	start := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
	hosts := make([]string, 0, addresses)
	for i := uint64(0); i < addresses; i++ {
		if addresses > 2 && (i == 0 || i == addresses-1) {
			continue
		}
		v := start + uint32(i)
		hosts = append(hosts, net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).String())
	}
	return hosts, ports, unitID, time.Duration(timeoutMS) * time.Millisecond, nil
}

func uniquePorts(values []int) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, p := range values {
		if p < 1 || p > 65535 || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}
