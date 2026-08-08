// Package bootstrap seeds the accounts a brand new database needs before
// anyone can log in. It is shared by the platform-admin CLI (explicit,
// one-shot) and platform-api startup (automatic, only when the System Admin
// account is missing) so both paths create identical rows.
package bootstrap

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
)

// Pool is the subset of *pgxpool.Pool this package uses.
type Pool interface {
	Begin(context.Context) (pgx.Tx, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type MiddlewareInput struct {
	Name, OrganizationCode string
	AutoOnboard            bool
}

func MiddlewareInputFromEnvironment() (MiddlewareInput, error) {
	input := MiddlewareInput{Name: strings.TrimSpace(os.Getenv("PLATFORM_MIDDLEWARE_NAME")), OrganizationCode: strings.TrimSpace(os.Getenv("PLATFORM_MIDDLEWARE_ORGANIZATION_CODE"))}
	if input.Name == "" || len(input.Name) > 200 || input.OrganizationCode == "" || len(input.OrganizationCode) > 100 {
		return input, fmt.Errorf("PLATFORM_MIDDLEWARE_NAME and PLATFORM_MIDDLEWARE_ORGANIZATION_CODE are required")
	}
	value := strings.TrimSpace(os.Getenv("PLATFORM_MIDDLEWARE_AUTO_ONBOARD"))
	if value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return input, fmt.Errorf("PLATFORM_MIDDLEWARE_AUTO_ONBOARD must be true or false")
		}
		input.AutoOnboard = parsed
	}
	return input, nil
}

type UserInput struct {
	Email, Username, DisplayName, Password, OrganizationCode, OrganizationName, Role string
}

func UserInputFromEnvironment() (UserInput, error) {
	input := UserInput{
		Email:            strings.ToLower(strings.TrimSpace(os.Getenv("PLATFORM_BOOTSTRAP_EMAIL"))),
		Username:         strings.ToLower(strings.TrimSpace(os.Getenv("PLATFORM_BOOTSTRAP_USERNAME"))),
		DisplayName:      strings.TrimSpace(os.Getenv("PLATFORM_BOOTSTRAP_DISPLAY_NAME")),
		Password:         os.Getenv("PLATFORM_BOOTSTRAP_PASSWORD"),
		OrganizationCode: strings.TrimSpace(os.Getenv("PLATFORM_BOOTSTRAP_ORGANIZATION_CODE")),
		OrganizationName: strings.TrimSpace(os.Getenv("PLATFORM_BOOTSTRAP_ORGANIZATION_NAME")),
		Role:             strings.TrimSpace(os.Getenv("PLATFORM_BOOTSTRAP_ROLE")),
	}
	if input.Role == "" {
		input.Role = "System Admin"
	}
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email {
		return input, fmt.Errorf("PLATFORM_BOOTSTRAP_EMAIL must be a normalized email address")
	}
	if input.DisplayName == "" || len(input.DisplayName) > 200 {
		return input, fmt.Errorf("PLATFORM_BOOTSTRAP_DISPLAY_NAME is required and must not exceed 200 characters")
	}
	if !auth.ValidateNewPassword(input.Password) {
		return input, fmt.Errorf("PLATFORM_BOOTSTRAP_PASSWORD must be 12 to 72 bytes")
	}
	if input.OrganizationCode == "" || len(input.OrganizationCode) > 100 || input.OrganizationName == "" || len(input.OrganizationName) > 200 {
		return input, fmt.Errorf("bootstrap organization code and name are required")
	}
	if len(input.Username) > 100 {
		return input, fmt.Errorf("PLATFORM_BOOTSTRAP_USERNAME must not exceed 100 characters")
	}
	validRole := false
	for _, role := range []string{"System Admin", "Organization Admin", "Plant Manager", "Engineer", "Operator", "Viewer", "Auditor"} {
		if input.Role == role {
			validRole = true
			break
		}
	}
	if !validRole {
		return input, fmt.Errorf("PLATFORM_BOOTSTRAP_ROLE must be a baseline system role")
	}
	return input, nil
}

// EnsureAdmin creates the configured System Admin from PLATFORM_BOOTSTRAP_*
// when the database holds no active one, so a fresh install — or a database
// moved to another machine — comes up with a usable login without anyone
// remembering to run `platform-admin bootstrap-user`. A database that already
// has an admin is left untouched, which makes this safe to run on every start.
func EnsureAdmin(ctx context.Context, pool Pool) error {
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM auth.user_role ur
		JOIN auth.role r ON r.id = ur.role_id
		JOIN auth.app_user u ON u.id = ur.user_id
		WHERE r.organization_id IS NULL AND r.name = 'System Admin' AND u.status = 'ACTIVE')`).Scan(&exists); err != nil {
		return fmt.Errorf("check for existing System Admin: %w", err)
	}
	if exists {
		return nil
	}
	input, err := UserInputFromEnvironment()
	if err != nil {
		return fmt.Errorf("no System Admin exists and PLATFORM_BOOTSTRAP_* is unusable, nobody can sign in: %w", err)
	}
	if err = CreateUser(ctx, pool, input); err != nil {
		return err
	}
	log.Printf("bootstrap: created System Admin %s in organization %s", input.Email, input.OrganizationCode)
	return nil
}

func CreateMiddleware(ctx context.Context, pool Pool, input MiddlewareInput) (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generate middleware key: %w", err)
	}
	key := "ygm_" + base64.RawURLEncoding.EncodeToString(secret)
	hash := sha256.Sum256([]byte(key))
	id, err := newUUID()
	if err != nil {
		return "", err
	}
	correlationID, err := newUUID()
	if err != nil {
		return "", err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin middleware bootstrap: %w", err)
	}
	defer tx.Rollback(ctx)
	var organizationID pgtype.UUID
	if err = tx.QueryRow(ctx, "SELECT id FROM organization WHERE code=$1 AND is_active=true", input.OrganizationCode).Scan(&organizationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("active organization %q not found", input.OrganizationCode)
		}
		return "", fmt.Errorf("load middleware organization: %w", err)
	}
	prefix := key[:12]
	if _, err = tx.Exec(ctx, `INSERT INTO auth.middleware_client(id,organization_id,name,key_prefix,key_hash,auto_onboard)
		VALUES($1,$2,$3,$4,$5,$6)`, id, organizationID, input.Name, prefix, hash[:], input.AutoOnboard); err != nil {
		if isUniqueViolation(err) {
			return "", fmt.Errorf("middleware client name already exists")
		}
		return "", fmt.Errorf("create middleware client: %w", err)
	}
	detail, _ := json.Marshal(map[string]any{"name": input.Name, "keyPrefix": prefix, "autoOnboard": input.AutoOnboard})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_log(organization_id,action,target_type,target_id,after_data,correlation_id)
		VALUES($1,'auth.middleware.bootstrap','middleware_client',$2,$3,$4)`, organizationID, id, detail, correlationID); err != nil {
		return "", fmt.Errorf("audit middleware bootstrap: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit middleware bootstrap: %w", err)
	}
	return key, nil
}

func CreateUser(ctx context.Context, pool Pool, input UserInput) error {
	passwordHash, err := auth.HashPassword(input.Password)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}
	organizationID, err := newUUID()
	if err != nil {
		return err
	}
	userID, err := newUUID()
	if err != nil {
		return err
	}
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	userRoleID, err := newUUID()
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin bootstrap: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO organization(id,code,name)
		VALUES($1,$2,$3) ON CONFLICT(code) DO NOTHING`, organizationID, input.OrganizationCode, input.OrganizationName); err != nil {
		return fmt.Errorf("create bootstrap organization: %w", err)
	}
	if err = tx.QueryRow(ctx, "SELECT id FROM organization WHERE code=$1", input.OrganizationCode).Scan(&organizationID); err != nil {
		return fmt.Errorf("load bootstrap organization: %w", err)
	}
	var username any
	if input.Username != "" {
		username = input.Username
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth.app_user(
		id,organization_id,email,username,display_name,password_hash
	) VALUES($1,$2,$3,$4,$5,$6)`, userID, organizationID, input.Email, username, input.DisplayName, passwordHash); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("bootstrap email or username already exists")
		}
		return fmt.Errorf("create bootstrap user: %w", err)
	}
	var roleID pgtype.UUID
	if err = tx.QueryRow(ctx, "SELECT id FROM auth.role WHERE organization_id IS NULL AND name=$1", input.Role).Scan(&roleID); err != nil {
		return fmt.Errorf("load bootstrap role: %w", err)
	}
	var roleOrganization any = organizationID
	if input.Role == "System Admin" {
		roleOrganization = nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth.user_role(id,organization_id,user_id,role_id)
		VALUES($1,$2,$3,$4)`, userRoleID, roleOrganization, userID, roleID); err != nil {
		return fmt.Errorf("assign bootstrap role: %w", err)
	}
	detail, _ := json.Marshal(map[string]string{"email": input.Email, "organizationCode": input.OrganizationCode, "role": input.Role})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_log(
		organization_id,actor_user_id,action,target_type,target_id,after_data,correlation_id
	) VALUES($1,$2,'auth.user.bootstrap','app_user',$2,$3,$4)`, organizationID, userID, detail, correlationID); err != nil {
		return fmt.Errorf("audit bootstrap user: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit bootstrap user: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func newUUID() (pgtype.UUID, error) {
	var value pgtype.UUID
	if _, err := rand.Read(value.Bytes[:]); err != nil {
		return value, fmt.Errorf("generate UUID: %w", err)
	}
	value.Bytes[6] = value.Bytes[6]&0x0f | 0x40
	value.Bytes[8] = value.Bytes[8]&0x3f | 0x80
	value.Valid = true
	return value, nil
}
