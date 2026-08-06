package web

import (
	"chpp/modbus-api-middleware/internal/license"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) activatePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(activateHTML))
}
func (s *Server) licenseStatus(w http.ResponseWriter, r *http.Request) {
	status, err := license.CheckFile(s.licensePath(), s.LicensePublicKey)
	if err != nil {
		writeJSON(w, 200, map[string]any{"active": false, "machineId": license.MachineID(), "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"active": true, "customer": status.Payload.Customer, "expiresAt": status.Payload.ExpiresAt, "machineId": status.MachineID})
}
func (s *Server) activateLicense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in) != nil || strings.TrimSpace(in.Token) == "" {
		writeError(w, 400, fmt.Errorf("license token is required"))
		return
	}
	status, err := license.Activate(s.licensePath(), in.Token, s.LicensePublicKey)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"active": true, "customer": status.Payload.Customer, "expiresAt": status.Payload.ExpiresAt, "machineId": status.MachineID})
}
func (s *Server) licensePath() string {
	files := s.licenseFiles()
	if len(files) == 0 {
		return "license.json"
	}
	return files[0]
}

const activateHTML = `<!doctype html><html lang="th"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Activate License</title><style>body{font:14px Arial;background:#f4f7f8}.card{max-width:700px;margin:12vh auto;padding:28px;background:white;border-radius:12px}textarea{width:100%;min-height:150px;box-sizing:border-box;font:12px Consolas}button{padding:11px 16px;margin-top:12px;background:#174f68;color:white;border:0;border-radius:8px;font-weight:bold}.error{color:#b44}.ok{color:#149466}.machine{font-family:Consolas}</style><main class="card"><h1>Activate License</h1><p>Machine ID: <span class="machine" id="m">กำลังโหลด...</span></p><textarea id="t" placeholder="payload.signature"></textarea><br><button onclick="a()">Activate</button> <button onclick="l()">Logout</button><div id="msg"></div></main><script>async function q(){let x=await(await fetch('/api/license/status')).json();m.textContent=x.machineId||'-';if(x.active){msg.className='ok';msg.textContent='License active: '+x.customer+' / '+x.expiresAt}}async function a(){let r=await fetch('/api/license/activate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({token:t.value})});let x=await r.json();if(r.ok){msg.className='ok';msg.textContent='Activate สำเร็จ';setTimeout(()=>location='/',700)}else{msg.className='error';msg.textContent=x.error||'Activate ไม่สำเร็จ'}}async function l(){await fetch('/api/auth/logout',{method:'POST'});location='/login'}q()</script>`
