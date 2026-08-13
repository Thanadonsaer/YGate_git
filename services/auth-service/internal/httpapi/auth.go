package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"ygate/auth-service/internal/auth"
)

// LoginFunc, RequestPasswordResetFunc, and ResetPasswordFunc, plus every
// handler in this file, are extracted verbatim (request/response shapes and
// status codes byte-for-byte) from platform-api's former
// internal/httpapi/server.go -- apps/web's auth-screen.tsx/sessions-page.tsx/
// profile-page.tsx call these endpoints unchanged.

type LoginFunc func(context.Context, auth.LoginInput) (auth.LoginResult, error)

type RequestPasswordResetFunc func(context.Context, string, *netip.Addr) error
type ResetPasswordFunc func(context.Context, string, string, *netip.Addr) error

func loginHandler(login LoginFunc, cookieSecure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if login == nil {
			http.Error(w, "authentication is unavailable", http.StatusServiceUnavailable)
			return
		}
		var request struct {
			Identifier string `json:"identifier"`
			Password   string `json:"password"`
		}
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		if len(request.Identifier) > 320 || len(request.Password) > 1024 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		userAgent := []rune(r.UserAgent())
		if len(userAgent) > 512 {
			userAgent = userAgent[:512]
		}
		result, err := login(r.Context(), auth.LoginInput{
			Identifier: request.Identifier,
			Password:   request.Password,
			SourceIP:   remoteIP(r.RemoteAddr),
			UserAgent:  string(userAgent),
		})
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		case errors.Is(err, auth.ErrRateLimited):
			w.Header().Set("Retry-After", "900")
			http.Error(w, "too many login attempts", http.StatusTooManyRequests)
			return
		case err != nil:
			log.Printf("login failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		setAuthCookies(w, result, cookieSecure)
		writeJSON(w, http.StatusOK, result)
	}
}

type PermissionsFunc func(context.Context, pgtype.UUID) ([]string, error)

func meHandler(permissions PermissionsFunc) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		grants, err := permissions(r.Context(), principal.UserID)
		if err != nil {
			log.Printf("load permissions failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		user := principal.User()
		user.Permissions = grants
		writeJSON(w, http.StatusOK, user)
	}
}

func logoutHandler(service *auth.Service, cookieSecure bool) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		if err := service.Logout(r.Context(), principal, remoteIP(r.RemoteAddr)); err != nil {
			log.Printf("logout failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		clearAuthCookies(w, cookieSecure)
		w.WriteHeader(http.StatusNoContent)
	}
}

func logoutAllHandler(service *auth.Service, cookieSecure bool) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		if err := service.LogoutAll(r.Context(), principal, remoteIP(r.RemoteAddr)); err != nil {
			log.Printf("logout all failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		clearAuthCookies(w, cookieSecure)
		w.WriteHeader(http.StatusNoContent)
	}
}

func forgotPasswordHandler(requestReset RequestPasswordResetFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var request struct {
			Email string `json:"email"`
		}
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		if strings.TrimSpace(request.Email) == "" || len(request.Email) > 320 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := requestReset(r.Context(), request.Email, remoteIP(r.RemoteAddr)); err != nil {
			log.Printf("password reset request failed: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}
}

func resetPasswordHandler(reset ResetPasswordFunc, cookieSecure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var request struct {
			Token       string `json:"token"`
			NewPassword string `json:"newPassword"`
		}
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		err := reset(r.Context(), request.Token, request.NewPassword, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, auth.ErrWeakPassword):
			http.Error(w, "new password must be 12 to 72 bytes", http.StatusBadRequest)
		case errors.Is(err, auth.ErrInvalidResetToken):
			http.Error(w, "invalid or expired reset token", http.StatusBadRequest)
		case errors.Is(err, auth.ErrRateLimited):
			w.Header().Set("Retry-After", "900")
			http.Error(w, "too many reset attempts", http.StatusTooManyRequests)
		case err != nil:
			log.Printf("password reset failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			clearAuthCookies(w, cookieSecure)
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func sessionsHandler(service *auth.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		sessions, err := service.Sessions(r.Context(), principal)
		if err != nil {
			log.Printf("list sessions failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, sessions)
	}
}

func revokeSessionHandler(service *auth.Service, cookieSecure bool) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		current, err := service.RevokeOwnSession(r.Context(), principal, r.PathValue("sessionId"), remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, auth.ErrSessionNotFound):
			http.Error(w, "session not found", http.StatusNotFound)
		case err != nil:
			log.Printf("revoke session failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			if current {
				clearAuthCookies(w, cookieSecure)
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func clearSessionsHandler(service *auth.Service, cookieSecure bool) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		err := service.ClearOwnSessions(r.Context(), principal, r.Header.Get("X-Hard-Delete-Confirm"), remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, auth.ErrSessionConfirmation):
			http.Error(w, "invalid confirmation", http.StatusBadRequest)
		case err != nil:
			log.Printf("clear sessions failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			clearAuthCookies(w, cookieSecure)
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func changePasswordHandler(service *auth.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		var request struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		err := service.ChangePassword(r.Context(), principal, request.CurrentPassword, request.NewPassword, remoteIP(r.RemoteAddr))
		switch {
		case errors.Is(err, auth.ErrInvalidCurrentPassword):
			http.Error(w, "current password is invalid", http.StatusBadRequest)
		case errors.Is(err, auth.ErrWeakPassword):
			http.Error(w, "new password must be 12 to 72 bytes", http.StatusBadRequest)
		case err != nil:
			log.Printf("change password failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func setAuthCookies(w http.ResponseWriter, result auth.LoginResult, secure bool) {
	maxAge := max(1, int(time.Until(result.ExpiresAt).Seconds()))
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: result.Token, Path: "/", Expires: result.ExpiresAt,
		MaxAge: maxAge, HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookieName, Value: result.CSRFToken, Path: "/", Expires: result.ExpiresAt,
		MaxAge: maxAge, HttpOnly: false, Secure: secure, SameSite: http.SameSiteStrictMode,
	})
}

func clearAuthCookies(w http.ResponseWriter, secure bool) {
	for _, cookie := range []*http.Cookie{
		{Name: sessionCookieName, HttpOnly: true},
		{Name: csrfCookieName, HttpOnly: false},
	} {
		cookie.Path = "/"
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0)
		cookie.Secure = secure
		cookie.SameSite = http.SameSiteStrictMode
		http.SetCookie(w, cookie)
	}
}

func registerHandler(service *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Email       string `json:"email"`
			Username    string `json:"username"`
			DisplayName string `json:"displayName"`
			Password    string `json:"password"`
		}
		if !decodeJSON(w, r, &request, 16<<10) {
			return
		}
		err := service.Register(r.Context(), auth.RegisterInput{Email: request.Email, Username: request.Username, DisplayName: request.DisplayName, Password: request.Password}, remoteIP(r.RemoteAddr))
		switch {
		case err == nil:
			writeJSON(w, http.StatusAccepted, map[string]string{"message": "check your email to verify your account"})
		case errors.Is(err, auth.ErrRegistrationConflict):
			http.Error(w, "email or username already exists", http.StatusConflict)
		case errors.Is(err, auth.ErrRegistrationUnavailable):
			http.Error(w, "registration email is unavailable", http.StatusServiceUnavailable)
		case errors.Is(err, auth.ErrRegistrationInvalid):
			http.Error(w, "invalid registration data", http.StatusBadRequest)
		default:
			log.Printf("registration failed: %v", err)
			http.Error(w, "registration unavailable", http.StatusServiceUnavailable)
		}
	}
}

func verifyEmailHandler(service *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := service.VerifyEmail(r.Context(), r.URL.Query().Get("token")); err != nil {
			if errors.Is(err, auth.ErrEmailVerificationToken) {
				http.Error(w, "invalid or expired verification link", http.StatusBadRequest)
				return
			}
			log.Printf("email verification failed: %v", err)
			http.Error(w, "email verification unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "email verified"})
	}
}

func resendVerificationHandler(service *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Email string `json:"email"`
		}
		if !decodeJSON(w, r, &request, 8<<10) {
			return
		}
		if err := service.ResendVerification(r.Context(), request.Email); err != nil {
			log.Printf("resend verification failed: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}
}
