package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"ygate/auth-service/internal/auth"
)

// These tests were originally platform-api's (services/platform-api/internal/httpapi/server_test.go)
// and moved here with the login/forgot-password/reset-password handlers
// themselves -- the request/response shapes and status codes they assert are
// unchanged, so apps/web's auth-screen.tsx needs no edits.

func TestLoginSetsSecureSessionAndCSRFCookiesWithoutReturningTokens(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	login := func(_ context.Context, input auth.LoginInput) (auth.LoginResult, error) {
		if input.Identifier != "operator@example.com" || input.Password != "secret" || input.SourceIP == nil || input.SourceIP.String() != "192.0.2.10" {
			t.Fatalf("input=%+v", input)
		}
		return auth.LoginResult{Token: "raw-secret-token", CSRFToken: "raw-csrf-token", ExpiresAt: expires, User: auth.LoginUser{ID: "user-1", Email: input.Identifier}}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"identifier":"operator@example.com","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.10:4321"
	res := httptest.NewRecorder()
	loginHandler(login, true).ServeHTTP(res, req)
	if res.Code != http.StatusOK || strings.Contains(res.Body.String(), "raw-secret-token") || strings.Contains(res.Body.String(), "raw-csrf-token") {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	cookies := res.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("cookies=%+v", cookies)
	}
	byName := map[string]*http.Cookie{cookies[0].Name: cookies[0], cookies[1].Name: cookies[1]}
	session, csrf := byName[sessionCookieName], byName[csrfCookieName]
	if session == nil || !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteStrictMode || session.Value != "raw-secret-token" {
		t.Fatalf("session cookie=%+v", session)
	}
	if csrf == nil || csrf.HttpOnly || !csrf.Secure || csrf.SameSite != http.SameSiteStrictMode || csrf.Value != "raw-csrf-token" {
		t.Fatalf("CSRF cookie=%+v", csrf)
	}
}

func TestLoginUsesGenericCredentialError(t *testing.T) {
	login := func(context.Context, auth.LoginInput) (auth.LoginResult, error) {
		return auth.LoginResult{}, auth.ErrInvalidCredentials
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"identifier":"missing@example.com","password":"wrong"}`))
	request.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	loginHandler(login, true).ServeHTTP(res, request)
	if res.Code != http.StatusUnauthorized || !strings.Contains(res.Body.String(), "invalid credentials") {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestLoginRejectsNonJSON(t *testing.T) {
	res := httptest.NewRecorder()
	loginHandler(func(context.Context, auth.LoginInput) (auth.LoginResult, error) {
		t.Fatal("login must not be called")
		return auth.LoginResult{}, nil
	}, true).ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("identifier=x")))
	if res.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestClearAuthCookiesExpiresBothCookies(t *testing.T) {
	res := httptest.NewRecorder()
	clearAuthCookies(res, true)
	cookies := res.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("cookies=%+v", cookies)
	}
	for _, cookie := range cookies {
		if cookie.MaxAge != -1 || !cookie.Secure || cookie.Path != "/" {
			t.Fatalf("cookie=%+v", cookie)
		}
	}
}

func TestForgotPasswordAlwaysAcceptsKnownAndUnknownOutcomes(t *testing.T) {
	for _, serviceErr := range []error{nil, errors.New("delivery failed")} {
		called := false
		handler := forgotPasswordHandler(func(_ context.Context, email string, sourceIP *netip.Addr) error {
			called = true
			if email != "operator@example.com" || sourceIP == nil || sourceIP.String() != "192.0.2.20" {
				t.Fatalf("email=%q sourceIP=%v", email, sourceIP)
			}
			return serviceErr
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", bytes.NewBufferString(`{"email":"operator@example.com"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.0.2.20:4321"
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if !called || res.Code != http.StatusAccepted {
			t.Fatalf("called=%v status=%d body=%s", called, res.Code, res.Body.String())
		}
	}
}

func TestResetPasswordMapsSecurityErrorsAndClearsCookies(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{
		{err: auth.ErrWeakPassword, status: http.StatusBadRequest},
		{err: auth.ErrInvalidResetToken, status: http.StatusBadRequest},
		{err: auth.ErrRateLimited, status: http.StatusTooManyRequests},
		{status: http.StatusNoContent},
	}
	for _, test := range tests {
		handler := resetPasswordHandler(func(_ context.Context, token, password string, _ *netip.Addr) error {
			if token != "reset-token" || password != "long-enough-password" {
				t.Fatalf("token=%q password=%q", token, password)
			}
			return test.err
		}, true)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", bytes.NewBufferString(`{"token":"reset-token","newPassword":"long-enough-password"}`))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != test.status {
			t.Fatalf("err=%v status=%d body=%s", test.err, res.Code, res.Body.String())
		}
		if test.err == nil && len(res.Result().Cookies()) != 2 {
			t.Fatalf("successful reset cookies=%+v", res.Result().Cookies())
		}
	}
}

func TestMeHandlerAttachesPermissions(t *testing.T) {
	var userID pgtype.UUID
	_ = userID.Scan("10000000-0000-4000-8000-000000000099")
	principal := auth.Principal{UserID: userID, Email: "operator@example.com", DisplayName: "Operator"}
	permissions := func(_ context.Context, id pgtype.UUID) ([]string, error) {
		if id != userID {
			t.Fatalf("userID = %v", id)
		}
		return []string{"plant:read", "alarm:read"}, nil
	}
	res := httptest.NewRecorder()
	meHandler(permissions)(res, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil), principal)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, `"permissions":["plant:read","alarm:read"]`) || !strings.Contains(body, `"email":"operator@example.com"`) {
		t.Fatalf("body=%s", body)
	}
}
