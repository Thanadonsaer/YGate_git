package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const webSessionCookie = "chpp_web_session"

type webSession struct{ expiresAt time.Time }
type webAuth struct {
	mu       sync.Mutex
	sessions map[string]webSession
}

func (s *Server) authGate(next http.Handler) http.Handler {
	if strings.TrimSpace(s.AdminUsername) == "" || s.AdminPassword == "" {
		return next
	}
	if s.auth == nil {
		s.auth = &webAuth{sessions: map[string]webSession{}}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" || r.URL.Path == "/api/auth/login" || r.URL.Path == "/api/auth/logout" {
			next.ServeHTTP(w, r)
			return
		}
		if s.authenticated(r) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("login required"))
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	})
}
func (s *Server) authenticated(r *http.Request) bool {
	c, err := r.Cookie(webSessionCookie)
	if err != nil || c.Value == "" {
		return false
	}
	s.auth.mu.Lock()
	defer s.auth.mu.Unlock()
	session, ok := s.auth.sessions[c.Value]
	if !ok || time.Now().After(session.expiresAt) {
		delete(s.auth.sessions, c.Value)
		return false
	}
	return true
}
func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(loginHTML))
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || in.Username != s.AdminUsername || in.Password != s.AdminPassword {
		writeError(w, 401, fmt.Errorf("invalid username or password"))
		return
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		writeError(w, 500, fmt.Errorf("could not create session"))
		return
	}
	token := hex.EncodeToString(b)
	s.auth.mu.Lock()
	s.auth.sessions[token] = webSession{expiresAt: time.Now().Add(12 * time.Hour)}
	s.auth.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: webSessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	writeJSON(w, 200, map[string]bool{"authenticated": true})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(webSessionCookie); err == nil {
		s.auth.mu.Lock()
		delete(s.auth.sessions, c.Value)
		s.auth.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: webSessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, 200, map[string]bool{"authenticated": false})
}

const loginHTML = `<!doctype html><html lang="th"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Middleware Login</title><style>body{font:14px Arial;background:#f4f7f8}.card{max-width:420px;margin:12vh auto;padding:28px;background:white;border-radius:12px}label{display:block;margin:14px 0 6px;font-weight:bold}input{box-sizing:border-box;width:100%;padding:11px}button{margin-top:18px;width:100%;padding:12px;background:#174f68;color:white;border:0;border-radius:8px;font-weight:bold}.error{color:#b44}</style><main class="card"><h1>Modbus API Middleware</h1><p>เข้าสู่ระบบ</p><form id="f"><label>Username<input id="u" required></label><label>Password<input id="p" type="password" required></label><div class="error" id="e"></div><button>Login</button></form></main><script>f.onsubmit=async x=>{x.preventDefault();let r=await fetch('/api/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:u.value,password:p.value})});if(r.ok)location='/';else e.textContent='Username หรือ Password ไม่ถูกต้อง'}</script>`
