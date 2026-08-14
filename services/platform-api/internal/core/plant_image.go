package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

const maxPlantImageBytes = 2 << 20

var plantImageExtensions = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
}

func validatePlantImage(data []byte) (string, error) {
	if len(data) == 0 || len(data) > maxPlantImageBytes {
		return "", ErrInvalid
	}
	sniffLen := len(data)
	if sniffLen > 512 {
		sniffLen = 512
	}
	contentType := http.DetectContentType(data[:sniffLen])
	if contentType == "image/webp" || (len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP") {
		return ".webp", nil
	}
	ext, ok := plantImageExtensions[contentType]
	if !ok {
		return "", ErrInvalid
	}
	return ext, nil
}

func (s *Service) WithPlantImageDir(dir string) *Service {
	if dir != "" {
		s.plantImageDir = dir
	}
	return s
}

func (s *Service) UploadPlantImage(ctx context.Context, principal auth.Principal, plantID string, data []byte, sourceIP *netip.Addr) (Plant, error) {
	ext, err := validatePlantImage(data)
	if err != nil {
		return Plant{}, err
	}
	id, err := parseUUID(plantID)
	if err != nil {
		return Plant{}, ErrNotFound
	}
	filenameID, err := newUUID()
	if err != nil {
		return Plant{}, err
	}
	if err = os.MkdirAll(s.plantImageDir, 0o755); err != nil {
		return Plant{}, fmt.Errorf("create plant image dir: %w", err)
	}
	filename := uuidString(filenameID) + ext
	storedPath := filepath.Join(s.plantImageDir, filename)
	if err = os.WriteFile(storedPath, data, 0o644); err != nil {
		return Plant{}, fmt.Errorf("store plant image: %w", err)
	}
	removeNewFile := true
	defer func() {
		if removeNewFile {
			_ = os.Remove(storedPath)
		}
	}()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Plant{}, fmt.Errorf("begin upload plant image: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	current, err := q.GetAuthorizedPlantForUpdate(ctx, dbgen.GetAuthorizedPlantForUpdateParams{PlantID: id, UserID: principal.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Plant{}, ErrNotFound
	}
	if err != nil {
		return Plant{}, fmt.Errorf("lock plant for image upload: %w", err)
	}
	beforePlant := plantFromFields(
		current.ID, current.OrganizationID, current.OrganizationName, current.Code, current.Name, current.Timezone,
		current.Latitude, current.Longitude, current.InstalledDcKw, current.InstalledAcKw, PlantLifecycleOperational, textPointer(current.ImageUrl),
		current.IsActive, current.AlarmEmailEnabled, current.AlarmNotifyRoleID, current.CreatedAt, current.UpdatedAt,
	)
	imageURL := "/api/v1/plants/" + plantID + "/image/" + filename
	if _, err = tx.Exec(ctx, "UPDATE plant.plant SET image_url=$1, updated_at=now() WHERE id=$2", imageURL, id); err != nil {
		return Plant{}, fmt.Errorf("update plant image URL: %w", err)
	}
	afterPlant := beforePlant
	afterPlant.ImageURL = &imageURL
	afterPlant.UpdatedAt = time.Now()
	if err = auditPlantImage(ctx, q, principal, current.OrganizationID, id, "plant.image_uploaded", beforePlant, afterPlant, sourceIP); err != nil {
		return Plant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Plant{}, fmt.Errorf("commit plant image upload: %w", err)
	}
	removeNewFile = false
	removePlantImageFile(s.plantImageDir, beforePlant.ImageURL)
	return afterPlant, nil
}

func (s *Service) DeletePlantImage(ctx context.Context, principal auth.Principal, plantID string, sourceIP *netip.Addr) (Plant, error) {
	id, err := parseUUID(plantID)
	if err != nil {
		return Plant{}, ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Plant{}, fmt.Errorf("begin delete plant image: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	current, err := q.GetAuthorizedPlantForUpdate(ctx, dbgen.GetAuthorizedPlantForUpdateParams{PlantID: id, UserID: principal.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Plant{}, ErrNotFound
	}
	if err != nil {
		return Plant{}, fmt.Errorf("lock plant for image delete: %w", err)
	}
	beforePlant := plantFromFields(
		current.ID, current.OrganizationID, current.OrganizationName, current.Code, current.Name, current.Timezone,
		current.Latitude, current.Longitude, current.InstalledDcKw, current.InstalledAcKw, PlantLifecycleOperational, textPointer(current.ImageUrl),
		current.IsActive, current.AlarmEmailEnabled, current.AlarmNotifyRoleID, current.CreatedAt, current.UpdatedAt,
	)
	if beforePlant.ImageURL == nil {
		return beforePlant, nil
	}
	if _, err = tx.Exec(ctx, "UPDATE plant.plant SET image_url=NULL, updated_at=now() WHERE id=$1", id); err != nil {
		return Plant{}, fmt.Errorf("clear plant image URL: %w", err)
	}
	afterPlant := beforePlant
	afterPlant.ImageURL = nil
	afterPlant.UpdatedAt = time.Now()
	if err = auditPlantImage(ctx, q, principal, current.OrganizationID, id, "plant.image_removed", beforePlant, afterPlant, sourceIP); err != nil {
		return Plant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Plant{}, fmt.Errorf("commit plant image delete: %w", err)
	}
	removePlantImageFile(s.plantImageDir, beforePlant.ImageURL)
	return afterPlant, nil
}

func (s *Service) PlantImageFilePath(ctx context.Context, principal auth.Principal, plantID, filename string) (string, error) {
	if filepath.Base(filename) != filename || filepath.Ext(filename) == "" {
		return "", ErrNotFound
	}
	plant, err := s.Plant(ctx, principal, plantID)
	if err != nil {
		return "", err
	}
	if plant.ImageURL == nil || filepath.Base(*plant.ImageURL) != filename {
		return "", ErrNotFound
	}
	return filepath.Join(s.plantImageDir, filename), nil
}

func auditPlantImage(ctx context.Context, q *dbgen.Queries, principal auth.Principal, organizationID, plantID pgtype.UUID, action string, before, after Plant, sourceIP *netip.Addr) error {
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{
		OrganizationID: organizationID, ActorUserID: principal.UserID, Action: action,
		TargetType: "plant", TargetID: plantID, BeforeData: beforeJSON, AfterData: afterJSON,
		SourceIp: sourceIP, CorrelationID: correlationID,
	}); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

func removePlantImageFile(dir string, imageURL *string) {
	if imageURL == nil {
		return
	}
	filename := filepath.Base(strings.TrimSpace(*imageURL))
	if filename != "." && filename != string(filepath.Separator) && filepath.Ext(filename) != "" {
		_ = os.Remove(filepath.Join(dir, filename))
	}
}
