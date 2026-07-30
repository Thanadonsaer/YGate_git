package web

import "testing"

func TestNormalizeDiscoveryLimitsCIDRAndPorts(t *testing.T) {
	hosts, ports, unitID, timeout, err := normalizeDiscovery(modbusDiscoveryRequest{CIDR: "192.168.1.0/30", Ports: []int{502, 502, 15034}, TimeoutMS: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 || hosts[0] != "192.168.1.1" || hosts[1] != "192.168.1.2" {
		t.Fatalf("hosts = %#v", hosts)
	}
	if len(ports) != 2 || ports[0] != 502 || ports[1] != 15034 {
		t.Fatalf("ports = %#v", ports)
	}
	if unitID != 1 || timeout.Milliseconds() != 100 {
		t.Fatalf("unitID/timeout = %d/%s", unitID, timeout)
	}
	if _, _, _, _, err = normalizeDiscovery(modbusDiscoveryRequest{CIDR: "192.168.0.0/16", Ports: []int{502}}); err == nil {
		t.Fatal("expected large CIDR error")
	}
}
