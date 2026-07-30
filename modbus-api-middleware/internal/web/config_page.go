package web

import "net/http"

const configHTML = `<!doctype html>
<html lang="th">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Gateway API Config</title>
<style>
:root{--ink:#15242b;--muted:#6b7c84;--line:#dbe5e8;--bg:#f4f7f8;--green:#149466;--red:#bd4b52}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font:14px/1.45 "Segoe UI",Tahoma,sans-serif}.wrap{max-width:960px;margin:28px auto;padding:0 18px}.card{background:#fff;border:1px solid var(--line);border-radius:12px;box-shadow:0 10px 28px rgba(17,47,56,.08);overflow:hidden;margin-bottom:14px}.head{padding:20px 22px;border-bottom:1px solid var(--line);display:flex;justify-content:space-between;gap:12px;align-items:flex-start}.head h1{margin:0;font-size:22px}.head p{margin:5px 0 0;color:var(--muted)}.body{padding:22px}.grid{display:grid;grid-template-columns:1fr 1fr;gap:16px}.field.full{grid-column:1/-1}label{display:block;font-size:11px;font-weight:800;color:#516871;margin-bottom:6px}input{width:100%;height:42px;border:1px solid #d3dfe3;border-radius:8px;padding:0 12px;font:inherit}.check{height:42px;display:flex;align-items:center;gap:8px;margin:0;color:var(--ink);font-size:13px}.check input{width:auto;height:auto}input:focus{outline:0;border-color:#197f92;box-shadow:0 0 0 3px #197f9216}.actions{border-top:1px solid var(--line);padding-top:18px;margin-top:20px;display:flex;justify-content:space-between;gap:10px;align-items:center}.btn{border:0;border-radius:8px;min-height:40px;padding:0 16px;font-weight:800;cursor:pointer}.primary{background:linear-gradient(135deg,#18a66f,#117b58);color:#fff}.secondary{background:#edf3f4;color:#435f67;text-decoration:none;display:inline-flex;align-items:center}.status{font-size:12px;color:var(--muted)}.ok{color:var(--green)}.err{color:var(--red)}.detail{display:grid;grid-template-columns:160px 1fr;gap:8px 12px}.detail b{color:#516871}.mono{font-family:Consolas,monospace;font-size:12px;word-break:break-all}@media(max-width:700px){.grid,.detail{grid-template-columns:1fr}.head{display:block}.secondary{margin-top:8px}}
</style>
</head>
<body>
<main class="wrap">
<form class="card" id="form">
<div class="head"><div><h1>Gateway API Config</h1><p>ตั้งค่าปลายทาง Backend ที่ middleware จะส่ง readings ไปหา</p></div><div><a class="btn secondary" href="/">กลับหน้า Address</a> <a class="btn secondary" href="/logs">API Logs</a></div></div>
<div class="body">
<div class="grid">
<div class="field"><label>gateway_id</label><input id="gateway" placeholder="MOXA-VT1-01"></div>
<div class="field"><label>api_key</label><input id="key" type="password" placeholder="chpp_xxx"></div>
<div class="field"><label>API polling</label><label class="check"><input id="enabled" type="checkbox"> Enable API Polling</label></div>
<div class="field"><label>API poll interval seconds</label><input id="interval" type="number" min="1" max="3600" value="5"></div>
<div class="field"><label>send timeout seconds</label><input id="timeout" type="number" min="1" max="300" value="10"></div>
<div class="field full"><label>API endpoint</label><input id="endpoint" type="url" placeholder="http://192.168.1.108:44440/api/v2/ingestion/register-readings"></div>
</div>
<div class="actions"><span class="status" id="status">กำลังโหลด...</span><button class="btn primary">บันทึก Config</button></div>
</div>
</form>
<section class="card"><div class="head"><div><h1>Connection Detail</h1><p>รายละเอียด config ปัจจุบันและสถานะส่ง API ล่าสุด</p></div><a class="btn secondary" href="/template/device-set-address.csv">Export Template</a></div><div class="body"><div class="detail" id="detail"></div></div></section>
</main>
<script>
const $=id=>document.getElementById(id);
function esc(v){const d=document.createElement('div');d.textContent=v??'';return d.innerHTML}
function mask(v){if(!v)return '-';return v.length<=8?'••••':v.slice(0,4)+'••••'+v.slice(-4)}
function dt(ms){return ms?new Date(ms).toLocaleString():'-'}
async function api(path,options){const r=await fetch(path,options);const text=await r.text();let body;try{body=text?JSON.parse(text):null}catch{body=null}if(!r.ok)throw new Error(body?.error||text||'HTTP '+r.status);return body?.data??body}
function status(text,ok=true){$('status').textContent=text;$('status').className='status '+(ok?'ok':'err')}
function detail(c,log){$('detail').innerHTML='<b>Gateway</b><div>'+esc(c.gatewayId||'-')+'</div><b>Endpoint</b><div class="mono">'+esc(c.endpoint||'-')+'</div><b>API Key</b><div class="mono">'+esc(mask(c.apiKey))+'</div><b>API Polling</b><div class="'+(c.apiPollingEnabled?'ok':'err')+'">'+(c.apiPollingEnabled?'Enabled':'Disabled')+'</div><b>API Poll Interval</b><div>'+esc(c.sendIntervalSeconds||5)+' sec</div><b>Timeout</b><div>'+esc(c.sendTimeoutSeconds||10)+' sec</div><b>Last API Send</b><div>'+esc(log?(log.status+' / HTTP '+(log.lastHttpStatus||'-')+' / '+dt(log.deliveredAt||log.createdAt)):'ยังไม่มี log')+'</div><b>Last Response</b><div class="mono">'+esc(log?.lastResponse||log?.lastError||'-')+'</div>'}
async function load(){try{const c=await api('/api/gateway-config');const logs=await api('/api/delivery-logs?limit=1');$('gateway').value=c.gatewayId||'';$('endpoint').value=c.endpoint||'';$('key').value=c.apiKey||'';$('enabled').checked=!!c.apiPollingEnabled;$('interval').value=c.sendIntervalSeconds||5;$('timeout').value=c.sendTimeoutSeconds||10;status(!c.endpoint?'ยังไม่ได้ตั้งค่า endpoint':(c.apiPollingEnabled?'API Polling เปิดอยู่':'API Polling ปิดอยู่'),!!(c.endpoint&&c.apiPollingEnabled));detail(c,logs[0])}catch(e){status(e.message,false)}}
$('form').onsubmit=async e=>{e.preventDefault();try{const c=await api('/api/gateway-config',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({gatewayId:$('gateway').value.trim(),endpoint:$('endpoint').value.trim(),apiKey:$('key').value.trim(),apiPollingEnabled:$('enabled').checked,sendIntervalSeconds:+$('interval').value||5,sendTimeoutSeconds:+$('timeout').value||10})});status('บันทึกแล้ว: '+(c.endpoint||'ไม่มี endpoint'),true);await load()}catch(e){status(e.message,false)}};
load();
</script>
</body>
</html>`

func (s *Server) configPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(configHTML))
}
