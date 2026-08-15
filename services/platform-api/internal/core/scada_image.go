package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"ygate/platform-api/internal/auth"
)

const maxScadaImageBytes = 2 << 20

// UploadScadaImage stores a canvas asset without coupling it to the Plant's
// cover image. Access remains scoped through the owning SCADA screen.
func (s *Service) UploadScadaImage(ctx context.Context, principal auth.Principal, screenID string, data []byte) (string, error) {
	ext, err := validateScadaImage(data)
	if err != nil {
		return "", err
	}
	id, err := parseUUID(screenID)
	if err != nil {
		return "", ErrScadaNotFound
	}
	if _, err = getAuthorizedScadaScreen(ctx, s.pool, principal, id, "edit", false); err != nil {
		return "", err
	}
	assetID, err := newUUID()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(s.plantImageDir, "scada", screenID)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create scada image dir: %w", err)
	}
	filename := uuidString(assetID) + ext
	if err = os.WriteFile(filepath.Join(dir, filename), data, 0o644); err != nil {
		return "", fmt.Errorf("store scada image: %w", err)
	}
	return "/api/v1/scada/screens/" + screenID + "/images/" + filename, nil
}

func (s *Service) ScadaImageFilePath(ctx context.Context, principal auth.Principal, screenID, filename string) (string, error) {
	if filepath.Base(filename) != filename || filepath.Ext(filename) == "" {
		return "", ErrScadaNotFound
	}
	id, err := parseUUID(screenID)
	if err != nil {
		return "", ErrScadaNotFound
	}
	if _, err = getAuthorizedScadaScreen(ctx, s.pool, principal, id, "view", false); err != nil {
		return "", err
	}
	path := filepath.Join(s.plantImageDir, "scada", screenID, filename)
	if _, err = os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "", ErrScadaNotFound
	} else if err != nil {
		return "", fmt.Errorf("stat scada image: %w", err)
	}
	return path, nil
}

func validateScadaImage(data []byte) (string, error) {
	if len(data) == 0 || len(data) > maxScadaImageBytes {
		return "", ErrInvalid
	}
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	contentType := http.DetectContentType(sniff)
	if contentType == "image/webp" || len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return ".webp", nil
	}
	if ext, ok := plantImageExtensions[contentType]; ok {
		return ext, nil
	}
	return "", ErrInvalid
}
