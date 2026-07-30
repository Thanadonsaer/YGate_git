package web

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
)

const logsHTML = `<!doctype html>
<html lang="th"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>API Send Logs</title>
<style>body{margin:0;background:#f4f7f8;color:#15242b;font:14px/1.45 "Segoe UI",Tahoma,sans-serif}.wrap{max-width:1180px;margin:28px auto;padding:0 18px}.card{background:#fff;border:1px solid #dbe5e8;border-radius:12px;box-shadow:0 10px 28px rgba(17,47,56,.08);overflow:hidden}.head{padding:20px 22px;border-bottom:1px solid #dbe5e8;display:flex;justify-content:space-between;gap:12px}.head h1{margin:0;font-size:22px}.head p{margin:4px 0 0;color:#6b7c84}.btn{border:0;border-radius:8px;min-height:38px;padding:0 14px;font-weight:800;background:#edf3f4;color:#435f67;text-decoration:none;display:inline-flex;align-items:center;cursor:pointer}.body{padding:18px;overflow:auto}table{width:100%;border-collapse:collapse;min-width:940px}th,td{text-align:left;border-bottom:1px solid #edf2f3;padding:9px;font-size:12px;vertical-align:top}th{color:#516871;background:#f6fafb}.mono{font-family:Consolas,monospace;word-break:break-all}.ok{color:#149466;font-weight:800}.bad{color:#bd4b52;font-weight:800}</style></head>
<body><main class="wrap"><section class="card"><div class="head"><div><h1>API Send Logs</h1><p>ประวัติ outbox ในการส่ง readings ขึ้น Backend API</p></div><div><button class="btn" onclick="load()">Refresh</button> <button class="btn" onclick="exportLogs()">Export CSV</button> <button class="btn" onclick="clearLogs()">Clear Logs</button> <a class="btn" href="/config">Config</a> <a class="btn" href="/">กลับหน้า Address</a></div></div><div class="body"><table><thead><tr><th>ID</th><th>Status</th><th>HTTP</th><th>Attempts</th><th>Created</th><th>Delivered</th><th>Error</th><th>Response</th><th>Idempotency Key</th><th>Detail</th></tr></thead><tbody id="rows"><tr><td colspan="10">Loading...</td></tr></tbody></table></div></section></main><dialog id="detail"><div class="head"><div><h1 id="detail-title">Response</h1><p>รายละเอียด response/error ล่าสุด</p></div><button class="btn" onclick="detail.close()">ปิด</button></div><div class="body"><pre class="mono" id="detail-body"></pre></div></dialog>
<script>
const rows=document.getElementById('rows'),detail=document.getElementById('detail');let logs=[];function esc(v){const d=document.createElement('div');d.textContent=v??'';return d.innerHTML}function dt(ms){return ms?new Date(ms).toLocaleString():'-'}function short(v){v=String(v||'');return v.length>90?v.slice(0,90)+'...':v}function cell(v){return '"'+String(v??'').replaceAll('"','""')+'"'}function download(name,lines){if(!logs.length)return alert('ไม่มี log ให้ export');const a=document.createElement('a');a.href=URL.createObjectURL(new Blob([lines.map(r=>r.map(cell).join(',')).join('\r\n')],{type:'text/csv;charset=utf-8'}));a.download=name;a.click();URL.revokeObjectURL(a.href)}function exportLogs(){download('api-send-logs.csv', [['id','status','http','attempts','created','delivered','error','response','idempotency_key'],...logs.map(x=>[x.id,x.status,x.lastHttpStatus||'',x.attempts,dt(x.createdAt),dt(x.deliveredAt),x.lastError||'',x.lastResponse||'',x.idempotencyKey||''])])}async function clearLogs(){if(!confirm('ล้าง API Send Logs ที่ส่งจบแล้ว? Pending/Retrying จะยังไม่ถูกลบ'))return;await fetch('/api/delivery-logs',{method:'DELETE'});await load()}function openDetail(i){const x=logs[i];document.getElementById('detail-title').textContent='Response #'+x.id;document.getElementById('detail-body').textContent=x.lastResponse||x.lastError||'-';detail.showModal()}
async function load(){const r=await fetch('/api/delivery-logs?limit=200');const body=await r.json();logs=body.data||[];rows.innerHTML=logs.length?logs.map((x,i)=>'<tr><td>'+x.id+'</td><td class="'+(x.status==='DELIVERED'?'ok':'bad')+'">'+esc(x.status)+'</td><td>'+(x.lastHttpStatus||'-')+'</td><td>'+x.attempts+'</td><td>'+dt(x.createdAt)+'</td><td>'+dt(x.deliveredAt)+'</td><td class="mono">'+esc(short(x.lastError||'-'))+'</td><td class="mono">'+esc(short(x.lastResponse||'-'))+'</td><td class="mono">'+esc(x.idempotencyKey)+'</td><td><button class="btn" onclick="openDetail('+i+')">Detail</button></td></tr>').join(''):'<tr><td colspan="10">ยังไม่มี log</td></tr>'}
load();setInterval(load,5000);
</script></body></html>`

func (s *Server) logsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(logsHTML))
}

func (s *Server) brandPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/brands/"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	brands, err := s.Store.Brands()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sets, err := s.Store.DeviceSets()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := fmt.Sprintf("Brand %d", id)
	for _, b := range brands {
		if b.BrandID == id {
			name = b.BrandName
			break
		}
	}
	var body strings.Builder
	for _, set := range sets {
		if set.BrandID != id {
			continue
		}
		fmt.Fprintf(&body, `<section class="card"><div class="head"><div><h2>%s • %s</h2><p>dev_set %d / %d address</p></div></div><div class="body"><table><thead><tr><th>FC</th><th>Register</th><th>Description</th><th>Factor</th><th>Data Type</th><th>Remark</th></tr></thead><tbody>`, html.EscapeString(set.DevType), html.EscapeString(set.DevModel), set.DeviceSetID, len(set.Addresses))
		for _, a := range set.Addresses {
			fmt.Fprintf(&body, `<tr><td>%02d</td><td>%d</td><td>%s</td><td>%g</td><td>%s</td><td>%s</td></tr>`, a.FunctionCode, a.Register, html.EscapeString(a.Description), a.Factor, html.EscapeString(a.DataType), html.EscapeString(a.Remark))
		}
		body.WriteString(`</tbody></table></div></section>`)
	}
	if body.Len() == 0 {
		body.WriteString(`<section class="card"><div class="body">ยังไม่มี Device Set ใน Brand นี้</div></section>`)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><html lang="th"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s Devices</title><style>body{margin:0;background:#f4f7f8;color:#15242b;font:14px/1.45 "Segoe UI",Tahoma,sans-serif}.wrap{max-width:1180px;margin:28px auto;padding:0 18px}.card{background:#fff;border:1px solid #dbe5e8;border-radius:12px;box-shadow:0 10px 28px rgba(17,47,56,.08);overflow:hidden;margin-bottom:14px}.head{padding:18px 20px;border-bottom:1px solid #dbe5e8;display:flex;justify-content:space-between;gap:12px}.head h1,.head h2{margin:0}.head p{margin:4px 0 0;color:#6b7c84}.btn{border:0;border-radius:8px;min-height:38px;padding:0 14px;font-weight:800;background:#edf3f4;color:#435f67;text-decoration:none;display:inline-flex;align-items:center}.body{padding:18px;overflow:auto}table{width:100%%;border-collapse:collapse;min-width:760px}th,td{text-align:left;border-bottom:1px solid #edf2f3;padding:9px;font-size:12px}th{color:#516871;background:#f6fafb}</style></head><body><main class="wrap"><section class="card"><div class="head"><div><h1>%s</h1><p>Device Set และ Address ภายใต้ Brand นี้</p></div><a class="btn" href="/">กลับหน้า Address</a></div></section>%s</main></body></html>`, html.EscapeString(name), html.EscapeString(name), body.String())
}

func (s *Server) templateCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF")
	cw := csv.NewWriter(&buf)
	_ = cw.Write([]string{"brand_name", "dev_type", "dev_model", "address_mode", "address_fc", "register", "description", "canonical_key", "factor", "data_type", "source_unit", "canonical_unit", "enabled", "remark"})
	_ = cw.Write([]string{"ABB", "Inverter", "PVS100", "VENDOR_RAW", "03", "40084", "Watts", "active_power", "0.01", "SHORT", "kW", "kW", "true", "Address.xlsx State=use"})
	_ = cw.Write([]string{"ABB", "Inverter", "PVS100", "VENDOR_RAW", "03", "40108", "Operating State", "inverter_state", "1", "USHORT", "", "", "true", "Address.xlsx State=use"})
	_ = cw.Write([]string{"ABB", "Inverter", "PVS100", "VENDOR_RAW", "03", "40233", "%Command Watt Limit", "active_power_adjustment", "0.1", "USHORT", "%", "%", "true", "Readback value; Modbus write control is separate"})
	_ = cw.Write([]string{"ABB", "Inverter", "PVS50", "VENDOR_RAW", "03", "40084", "Watts", "active_power", "0.01", "SHORT", "kW", "kW", "true", "Address.xlsx State=use"})
	_ = cw.Write([]string{"ABB", "Inverter", "PVS50", "VENDOR_RAW", "03", "40108", "Operating State", "inverter_state", "1", "USHORT", "", "", "true", "Address.xlsx State=use"})
	_ = cw.Write([]string{"ABB", "Inverter", "PVS50", "VENDOR_RAW", "03", "40233", "%Command Watt Limit", "active_power_adjustment", "0.1", "USHORT", "%", "%", "true", "Readback value; Modbus write control is separate"})
	_ = cw.Write([]string{"Huawei", "Inverter", "SUN2000", "VENDOR_RAW", "03", "32080", "Active power", "active_power", "0.001", "INT32", "kW", "kW", "true", "Address.xlsx says SW_INT; use SW_INT only if field test proves low-word-first"})
	_ = cw.Write([]string{"Huawei", "Inverter", "SUN2000", "VENDOR_RAW", "03", "32089", "Device status", "inverter_state", "1", "USHORT", "", "", "true", "Use unsigned so 0xA000 remains 40960"})
	_ = cw.Write([]string{"Huawei", "Inverter", "SUN2000", "VENDOR_RAW", "03", "32114", "Energy yield of current day", "day_cap", "0.01", "UINT32", "kWh", "kWh", "true", "Address.xlsx says SW_UINT; use SW_UINT only if field test proves low-word-first"})
	_ = cw.Write([]string{"Huawei", "Inverter", "SUN2000", "VENDOR_RAW", "03", "35302", "Power_adjustment", "active_power_adjustment", "0.1", "SHORT", "%", "%", "true", "Readback value; Modbus write control is separate"})
	cw.Flush()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="device-set-address-template.csv"`)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) connectionTemplateCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF")
	cw := csv.NewWriter(&buf)
	_ = cw.Write([]string{"connection_name", "plant_code", "device_code", "device_name", "device_set_id", "brand_name", "dev_type", "dev_model", "host", "port", "unit_id", "enabled"})
	_ = cw.Write([]string{"VT1-INV-01", "VT1", "VT1-INV-01", "Inverter 01", "", "Huawei", "Inverter", "SUN2000-100KTL-M1", "192.168.1.200", "502", "1", "true"})
	cw.Flush()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="connections-template.csv"`)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) deviceTemplateCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	plantCode := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("plantCode")))
	if plantCode == "" {
		http.Error(w, "plantCode is required", http.StatusBadRequest)
		return
	}
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF")
	cw := csv.NewWriter(&buf)
	_ = cw.Write([]string{"connection_name", "device_code", "device_name", "device_set_id", "brand_name", "dev_type", "dev_model", "host", "port", "unit_id", "enabled"})
	_ = cw.Write([]string{plantCode + "-INV-01", plantCode + "-INV-01", "Inverter 01", "", "Huawei", "Inverter", "SUN2000-100KTL-M1", "192.168.1.200", "502", "1", "true"})
	cw.Flush()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="devices-%s-template.csv"`, plantCode))
	_, _ = w.Write(buf.Bytes())
}
