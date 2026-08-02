package auth

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"ygate/auth-service/internal/database"
)

func TestAuthenticationLifecycleAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	organizationID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	userID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	suffix := uuidString(userID)
	email := "login-" + suffix + "@example.com"
	username := "login-" + suffix
	passwordHash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "INSERT INTO organization(id,code,name) VALUES($1,$2,$3)", organizationID, "ORG-"+suffix, "Auth Integration"); err != nil {
		t.Fatal(err)
	}

	if _, err = pool.Exec(ctx, `INSERT INTO app_user(id,organization_id,email,username,display_name,password_hash)
		VALUES($1,$2,$3,$4,$5,$6)`, userID, organizationID, email, username, "Integration Operator", passwordHash); err != nil {
		t.Fatal(err)
	}

	service := New(pool, 30*time.Minute, 24*time.Hour)
	if _, err = service.Login(ctx, LoginInput{Identifier: username, Password: "wrong"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error=%v", err)
	}
	result, err := service.Login(ctx, LoginInput{Identifier: username, Password: "correct-password"})
	if err != nil || result.Token == "" || result.CSRFToken == "" || result.User.Email != email {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	var storedTokenHash, storedCSRFHash []byte
	if err = pool.QueryRow(ctx, "SELECT token_hash,csrf_hash FROM user_session WHERE user_id=$1", userID).Scan(&storedTokenHash, &storedCSRFHash); err != nil {
		t.Fatal(err)
	}
	if !hashesEqual(storedTokenHash, result.Token) || !hashesEqual(storedCSRFHash, result.CSRFToken) {
		t.Fatal("database did not store only session and CSRF token hashes")
	}
	principal, err := service.Authenticate(ctx, result.Token)
	if err != nil || !principal.ValidCSRF(result.CSRFToken) || principal.ValidCSRF("wrong") {
		t.Fatalf("principal=%+v err=%v", principal.User(), err)
	}

	if err = service.ChangePassword(ctx, principal, "correct-password", "new-correct-password", nil); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Authenticate(ctx, result.Token); err != nil {
		t.Fatalf("current session was revoked after password change: %v", err)
	}
	if _, err = service.Login(ctx, LoginInput{Identifier: email, Password: "correct-password"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password error=%v", err)
	}
	second, err := service.Login(ctx, LoginInput{Identifier: email, Password: "new-correct-password"})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := service.Sessions(ctx, principal)
	if err != nil || len(sessions) < 2 {
		t.Fatalf("sessions=%+v err=%v", sessions, err)
	}
	secondPrincipal, err := service.Authenticate(ctx, second.Token)
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.RevokeOwnSession(ctx, principal, secondPrincipal.SessionID.String(), nil)
	if err != nil || current {
		t.Fatalf("revoke other session current=%v err=%v", current, err)
	}
	if _, err = service.Authenticate(ctx, second.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked session remained active: %v", err)
	}
	if err = service.LogoutAll(ctx, principal, nil); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{result.Token, second.Token} {
		if _, err = service.Authenticate(ctx, token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("logout-all token remained active: %v", err)
		}
	}

	for range 5 {
		if _, err = service.Login(ctx, LoginInput{Identifier: email, Password: "wrong"}); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("lockout failure error=%v", err)
		}
	}
	if _, err = service.Login(ctx, LoginInput{Identifier: email, Password: "new-correct-password"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("locked account error=%v", err)
	}

	var resetToken string
	service.ConfigurePasswordRecovery(10*time.Minute, func(_ context.Context, recipient, token string) error {
		if recipient != email {
			t.Fatalf("reset recipient=%q", recipient)
		}
		resetToken = token
		return nil
	})
	if err = service.RequestPasswordReset(ctx, "missing-"+email, nil); err != nil || resetToken != "" {
		t.Fatalf("unknown account reset token=%q err=%v", resetToken, err)
	}
	if err = service.RequestPasswordReset(ctx, email, nil); err != nil || resetToken == "" {
		t.Fatalf("known account reset token=%q err=%v", resetToken, err)
	}
	var storedResetHash []byte
	if err = pool.QueryRow(ctx, "SELECT token_hash FROM password_reset_token WHERE user_id=$1 AND used_at IS NULL", userID).Scan(&storedResetHash); err != nil {
		t.Fatal(err)
	}
	if !hashesEqual(storedResetHash, resetToken) {
		t.Fatal("database did not store only the password reset token hash")
	}
	if err = service.ResetPassword(ctx, resetToken, "reset-correct-password", nil); err != nil {
		t.Fatal(err)
	}
	if err = service.ResetPassword(ctx, resetToken, "another-correct-password", nil); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("reused reset token error=%v", err)
	}
	if _, err = service.Login(ctx, LoginInput{Identifier: email, Password: "reset-correct-password"}); err != nil {
		t.Fatalf("login after reset: %v", err)
	}
}

func TestPermissionsReflectsRoleGrantsAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	organizationID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	userID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	suffix := uuidString(userID)
	if _, err = pool.Exec(ctx, "INSERT INTO organization(id,code,name) VALUES($1,$2,$3)", organizationID, "PERM-"+suffix, "Permissions Integration"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO app_user(id,organization_id,email,username,display_name,password_hash)
		VALUES($1,$2,$3,$4,$5,'unused')`, userID, organizationID, "perm-"+suffix+"@example.com", "perm-"+suffix, "Permissions Viewer"); err != nil {
		t.Fatal(err)
	}
	// 00000000-0000-4000-8000-000000000206 is the seeded "Viewer" system role
	// (read-only across organization/plant/asset_group/device_model/device),
	// assigned scoped to this test's organization.
	viewerRoleID, err := parseUUID("00000000-0000-4000-8000-000000000206")
	if err != nil {
		t.Fatal(err)
	}
	assignmentID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_role(id,organization_id,user_id,role_id) VALUES($1,$2,$3,$4)`,
		assignmentID, organizationID, userID, viewerRoleID); err != nil {
		t.Fatal(err)
	}

	service := New(pool, 30*time.Minute, 24*time.Hour)
	permissions, err := service.Permissions(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	byValue := map[string]bool{}
	for _, permission := range permissions {
		byValue[permission] = true
	}
	if !byValue["plant:read"] || !byValue["device:read"] {
		t.Fatalf("expected read grants missing, got %v", permissions)
	}
	if byValue["user:create"] || byValue["plant:create"] {
		t.Fatalf("viewer role must not carry write grants, got %v", permissions)
	}
}
