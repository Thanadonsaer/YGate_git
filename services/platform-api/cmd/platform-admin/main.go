package main

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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/config"
	"ygate/platform-api/internal/database"
	"ygate/platform-api/internal/envfile"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "bootstrap-user" && os.Args[1] != "bootstrap-middleware" && os.Args[1] != "migrate") {
		log.Fatal("usage: platform-admin bootstrap-user|bootstrap-middleware|migrate")
	}
	if err := envfile.LoadDefault(); err != nil {
		log.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// database.Open applies every embedded forward-only migration that is not
	// yet in schema_migrations before returning (see internal/database), under
	// a Postgres advisory lock so concurrent instances don't race. Running
	// this subcommand IS "migrate deploy": the target database must already
	// exist, and this only brings its schema up to date — no separate
	// migration runner needed.
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if os.Args[1] == "migrate" {
		fmt.Println("database schema is up to date")
		return
	}
	if os.Args[1] == "bootstrap-middleware" {
		input, inputErr := middlewareInputFromEnvironment()
		if inputErr != nil {
			log.Fatal(inputErr)
		}
		key, bootstrapErr := bootstrapMiddleware(ctx, pool, input)
		if bootstrapErr != nil {
			log.Fatal(bootstrapErr)
		}
		fmt.Printf("middleware API key (shown once): %s\n", key)
		return
	}
	input, err := bootstrapInputFromEnvironment()
	if err != nil {
		log.Fatal(err)
	}
	if err = bootstrapUser(ctx, pool, input); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("bootstrap user created: %s (%s)\n", input.email, input.organizationCode)
}

type middlewareInput struct {
	name, organizationCode string
	autoOnboard            bool
}

func middlewareInputFromEnvironment() (middlewareInput, error) {
	input := middlewareInput{name: strings.TrimSpace(os.Getenv("PLATFORM_MIDDLEWARE_NAME")), organizationCode: strings.TrimSpace(os.Getenv("PLATFORM_MIDDLEWARE_ORGANIZATION_CODE"))}
	if input.name == "" || len(input.name) > 200 || input.organizationCode == "" || len(input.organizationCode) > 100 {
		return input, fmt.Errorf("PLATFORM_MIDDLEWARE_NAME and PLATFORM_MIDDLEWARE_ORGANIZATION_CODE are required")
	}
	value := strings.TrimSpace(os.Getenv("PLATFORM_MIDDLEWARE_AUTO_ONBOARD"))
	if value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return input, fmt.Errorf("PLATFORM_MIDDLEWARE_AUTO_ONBOARD must be true or false")
		}
		input.autoOnboard = parsed
	}
	return input, nil
}

type bootstrapInput struct {
	email, username, displayName, password, organizationCode, organizationName, role string
}

func bootstrapInputFromEnvironment() (bootstrapInput, error) {
	input := bootstrapInput{
		email:            strings.ToLower(strings.TrimSpace(os.Getenv("PLATFORM_BOOTSTRAP_EMAIL"))),
		username:         strings.ToLower(strings.TrimSpace(os.Getenv("PLATFORM_BOOTSTRAP_USERNAME"))),
		displayName:      strings.TrimSpace(os.Getenv("PLATFORM_BOOTSTRAP_DISPLAY_NAME")),
		password:         os.Getenv("PLATFORM_BOOTSTRAP_PASSWORD"),
		organizationCode: strings.TrimSpace(os.Getenv("PLATFORM_BOOTSTRAP_ORGANIZATION_CODE")),
		organizationName: strings.TrimSpace(os.Getenv("PLATFORM_BOOTSTRAP_ORGANIZATION_NAME")),
		role:             strings.TrimSpace(os.Getenv("PLATFORM_BOOTSTRAP_ROLE")),
	}
	if input.role == "" {
		input.role = "Platform Admin"
	}
	address, err := mail.ParseAddress(input.email)
	if err != nil || address.Address != input.email {
		return input, fmt.Errorf("PLATFORM_BOOTSTRAP_EMAIL must be a normalized email address")
	}
	if input.displayName == "" || len(input.displayName) > 200 {
		return input, fmt.Errorf("PLATFORM_BOOTSTRAP_DISPLAY_NAME is required and must not exceed 200 characters")
	}
	if !auth.ValidateNewPassword(input.password) {
		return input, fmt.Errorf("PLATFORM_BOOTSTRAP_PASSWORD must be 12 to 72 bytes")
	}
	if input.organizationCode == "" || len(input.organizationCode) > 100 || input.organizationName == "" || len(input.organizationName) > 200 {
		return input, fmt.Errorf("bootstrap organization code and name are required")
	}
	if len(input.username) > 100 {
		return input, fmt.Errorf("PLATFORM_BOOTSTRAP_USERNAME must not exceed 100 characters")
	}
	validRole := false
	for _, role := range []string{"Platform Admin", "Organization Admin", "Plant Manager", "Engineer", "Operator", "Viewer", "Auditor"} {
		if input.role == role {
			validRole = true
			break
		}
	}
	if !validRole {
		return input, fmt.Errorf("PLATFORM_BOOTSTRAP_ROLE must be a baseline system role")
	}
	return input, nil
}

type databasePool interface {
	Begin(context.Context) (pgx.Tx, error)
}

func bootstrapMiddleware(ctx context.Context, pool databasePool, input middlewareInput) (string, error) {
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
	if err = tx.QueryRow(ctx, "SELECT id FROM organization WHERE code=$1 AND is_active=true", input.organizationCode).Scan(&organizationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("active organization %q not found", input.organizationCode)
		}
		return "", fmt.Errorf("load middleware organization: %w", err)
	}
	prefix := key[:12]
	if _, err = tx.Exec(ctx, `INSERT INTO middleware_client(id,organization_id,name,key_prefix,key_hash,auto_onboard)
		VALUES($1,$2,$3,$4,$5,$6)`, id, organizationID, input.name, prefix, hash[:], input.autoOnboard); err != nil {
		if isUniqueViolation(err) {
			return "", fmt.Errorf("middleware client name already exists")
		}
		return "", fmt.Errorf("create middleware client: %w", err)
	}
	detail, _ := json.Marshal(map[string]any{"name": input.name, "keyPrefix": prefix, "autoOnboard": input.autoOnboard})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_log(organization_id,action,target_type,target_id,after_data,correlation_id)
		VALUES($1,'auth.middleware.bootstrap','middleware_client',$2,$3,$4)`, organizationID, id, detail, correlationID); err != nil {
		return "", fmt.Errorf("audit middleware bootstrap: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit middleware bootstrap: %w", err)
	}
	return key, nil
}

func bootstrapUser(ctx context.Context, pool databasePool, input bootstrapInput) error {
	passwordHash, err := auth.HashPassword(input.password)
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
		VALUES($1,$2,$3) ON CONFLICT(code) DO NOTHING`, organizationID, input.organizationCode, input.organizationName); err != nil {
		return fmt.Errorf("create bootstrap organization: %w", err)
	}
	if err = tx.QueryRow(ctx, "SELECT id FROM organization WHERE code=$1", input.organizationCode).Scan(&organizationID); err != nil {
		return fmt.Errorf("load bootstrap organization: %w", err)
	}
	var username any
	if input.username != "" {
		username = input.username
	}
	if _, err = tx.Exec(ctx, `INSERT INTO app_user(
		id,organization_id,email,username,display_name,password_hash
	) VALUES($1,$2,$3,$4,$5,$6)`, userID, organizationID, input.email, username, input.displayName, passwordHash); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("bootstrap email or username already exists")
		}
		return fmt.Errorf("create bootstrap user: %w", err)
	}
	var roleID pgtype.UUID
	if err = tx.QueryRow(ctx, "SELECT id FROM role WHERE organization_id IS NULL AND name=$1 AND is_system=true", input.role).Scan(&roleID); err != nil {
		return fmt.Errorf("load bootstrap role: %w", err)
	}
	var roleOrganization any = organizationID
	if input.role == "Platform Admin" {
		roleOrganization = nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO user_role(id,organization_id,user_id,role_id)
		VALUES($1,$2,$3,$4)`, userRoleID, roleOrganization, userID, roleID); err != nil {
		return fmt.Errorf("assign bootstrap role: %w", err)
	}
	detail, _ := json.Marshal(map[string]string{"email": input.email, "organizationCode": input.organizationCode, "role": input.role})
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
