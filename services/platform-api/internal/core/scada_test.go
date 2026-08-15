package core

import (
	"math"
	"testing"
)

func TestValidateScadaDesign(t *testing.T) {
	minimum, maximum, onValue := 0.0, 100.0, 1.0
	binding := ScadaTelemetryBinding{DeviceID: "00000000-0000-4000-8000-000000000001", PointKey: "activePower", Unit: "kW", Decimals: 1}
	valid := ScadaDesign{
		Version: 1,
		Nodes: []ScadaNode{
			{ID: "source", Type: "equipment", Position: ScadaPosition{X: 10, Y: 20}, Data: ScadaNodeData{Label: "Inverter", EquipmentKind: "inverter"}},
			{ID: "power", Type: "metric", Position: ScadaPosition{X: 200, Y: 20}, Data: ScadaNodeData{Label: "Active power", Binding: &binding, DisplayType: "gauge", MinValue: &minimum, MaxValue: &maximum}},
			{ID: "shape", Type: "shape", Position: ScadaPosition{X: 10, Y: 120}, Data: ScadaNodeData{Label: "Bus", ShapeKind: "rectangle"}, Width: 200, Height: 60},
			{ID: "section", Type: "section", Position: ScadaPosition{X: 10, Y: 200}, Data: ScadaNodeData{Label: "Inverter group"}, Width: 400, Height: 220},
			{ID: "led", Type: "led", Position: ScadaPosition{X: 250, Y: 120}, Data: ScadaNodeData{Label: "Running", Binding: &binding, OnValue: &onValue}},
			{ID: "clock", Type: "clock", Position: ScadaPosition{X: 400, Y: 20}, Data: ScadaNodeData{Label: "Plant time", Timezone: "Asia/Bangkok"}},
			{ID: "image", Type: "image", Position: ScadaPosition{X: 500, Y: 120}, Data: ScadaNodeData{Label: "Reference", ImageURL: "https://example.com/plant.png"}, Width: 240, Height: 160},
			{ID: "table", Type: "table", Position: ScadaPosition{X: 600, Y: 20}, Data: ScadaNodeData{Label: "Measurements", Items: []ScadaDataItem{{Label: "Power", Binding: binding}}}},
			{ID: "alarms", Type: "alarms", Position: ScadaPosition{X: 800, Y: 20}, Data: ScadaNodeData{Label: "Thresholds", Items: []ScadaDataItem{{Label: "Power", Binding: binding, MinAlarm: &minimum, MaxAlarm: &maximum}}}},
			{ID: "ticker", Type: "ticker", Position: ScadaPosition{X: 10, Y: 450}, Data: ScadaNodeData{Label: "Message", Text: "Plant operating normally"}},
			{ID: "device-summary", Type: "device-summary", Position: ScadaPosition{X: 800, Y: 200}, Data: ScadaNodeData{Label: "All parameters", DeviceID: binding.DeviceID}, Width: 300, Height: 240},
		},
		Edges:    []ScadaEdge{{ID: "flow", Source: "source", Target: "power", Type: "smoothstep"}},
		Viewport: ScadaViewport{Zoom: 1},
	}
	if err := ValidateScadaDesign(valid); err != nil {
		t.Fatalf("valid design rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ScadaDesign)
	}{
		{name: "duplicate node", mutate: func(d *ScadaDesign) { d.Nodes[1].ID = d.Nodes[0].ID }},
		{name: "missing metric binding", mutate: func(d *ScadaDesign) { d.Nodes[1].Data.Binding = nil }},
		{name: "edge target missing", mutate: func(d *ScadaDesign) { d.Edges[0].Target = "missing" }},
		{name: "non finite position", mutate: func(d *ScadaDesign) { d.Nodes[0].Position.X = math.NaN() }},
		{name: "unsafe image URL", mutate: func(d *ScadaDesign) { d.Nodes[6].Data.ImageURL = "javascript:alert(1)" }},
		{name: "missing table rows", mutate: func(d *ScadaDesign) { d.Nodes[7].Data.Items = nil }},
		{name: "invalid clock timezone", mutate: func(d *ScadaDesign) { d.Nodes[5].Data.Timezone = "Mars/Olympus" }},
		{name: "oversized node", mutate: func(d *ScadaDesign) { d.Nodes[2].Width = 3000 }},
		{name: "invalid node color", mutate: func(d *ScadaDesign) { d.Nodes[2].Data.BackgroundColor = "red" }},
		{name: "orphaned parent", mutate: func(d *ScadaDesign) { d.Nodes[2].ParentID = "missing" }},
		{name: "missing device-summary deviceId", mutate: func(d *ScadaDesign) { d.Nodes[10].Data.DeviceID = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copy := valid
			copy.Nodes = append([]ScadaNode(nil), valid.Nodes...)
			copy.Edges = append([]ScadaEdge(nil), valid.Edges...)
			test.mutate(&copy)
			if ValidateScadaDesign(copy) == nil {
				t.Fatal("invalid design accepted")
			}
		})
	}
}
