package license

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Payload struct {
	Customer  string   `json:"customer"`
	MachineID string   `json:"machineId"`
	ExpiresAt string   `json:"expiresAt,omitempty"`
	Features  []string `json:"features,omitempty"`
}

type Activation struct {
	Token       string    `json:"token"`
	ActivatedAt time.Time `json:"activatedAt"`
	MachineID   string    `json:"machineId"`
}

type Status struct {
	Payload     Payload   `json:"payload"`
	ActivatedAt time.Time `json:"activatedAt"`
	MachineID   string    `json:"machineId"`
	LicenseFile string    `json:"licenseFile"`
}

func MachineID() string {
	if v := strings.TrimSpace(os.Getenv("CHPP_LICENSE_MACHINE_ID")); v != "" {
		return v
	}
	host, _ := os.Hostname()
	sum := sha256.Sum256([]byte(runtime.GOOS + ":" + strings.ToLower(strings.TrimSpace(host))))
	return strings.ToUpper(hex.EncodeToString(sum[:8]))
}

func Activate(path, token, publicKey string) (Status, error) {
	machineID := MachineID()
	payload, err := VerifyToken(token, publicKey, machineID, time.Now())
	if err != nil {
		return Status{}, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return Status{}, err
	}
	activation := Activation{Token: strings.TrimSpace(token), ActivatedAt: time.Now().UTC(), MachineID: machineID}
	b, err := json.MarshalIndent(activation, "", "  ")
	if err != nil {
		return Status{}, err
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		return Status{}, err
	}
	return Status{Payload: payload, ActivatedAt: activation.ActivatedAt, MachineID: machineID, LicenseFile: path}, nil
}

func CheckFile(path, publicKey string) (Status, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Status{}, fmt.Errorf("license file not found: %s", path)
	}
	var activation Activation
	if err = json.Unmarshal(b, &activation); err != nil {
		return Status{}, fmt.Errorf("invalid license file: %w", err)
	}
	machineID := MachineID()
	payload, err := VerifyToken(activation.Token, publicKey, machineID, time.Now())
	if err != nil {
		return Status{}, err
	}
	return Status{Payload: payload, ActivatedAt: activation.ActivatedAt, MachineID: machineID, LicenseFile: path}, nil
}

func VerifyToken(token, publicKey, machineID string, now time.Time) (Payload, error) {
	if strings.TrimSpace(publicKey) == "" {
		return Payload{}, fmt.Errorf("license public key is not configured in this build")
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 {
		return Payload{}, fmt.Errorf("license token must be payload.signature")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Payload{}, fmt.Errorf("invalid license payload: %w", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Payload{}, fmt.Errorf("invalid license signature: %w", err)
	}
	pub, err := decodePublicKey(publicKey)
	if err != nil {
		return Payload{}, err
	}
	if !ed25519.Verify(pub, []byte(parts[0]), sig) {
		return Payload{}, fmt.Errorf("invalid license signature")
	}
	var payload Payload
	if err = json.Unmarshal(payloadBytes, &payload); err != nil {
		return Payload{}, fmt.Errorf("invalid license payload json: %w", err)
	}
	if payload.MachineID != "" && payload.MachineID != "*" && !strings.EqualFold(payload.MachineID, machineID) {
		return Payload{}, fmt.Errorf("license is for machine %s, current machine is %s", payload.MachineID, machineID)
	}
	if payload.ExpiresAt != "" {
		expires, err := time.Parse(time.RFC3339, payload.ExpiresAt)
		if err != nil {
			return Payload{}, fmt.Errorf("invalid license expiresAt: %w", err)
		}
		if !now.Before(expires) {
			return Payload{}, fmt.Errorf("license expired at %s", payload.ExpiresAt)
		}
	}
	return payload, nil
}

func decodePublicKey(value string) (ed25519.PublicKey, error) {
	value = strings.TrimSpace(value)
	decoders := []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding}
	for _, decoder := range decoders {
		b, err := decoder.DecodeString(value)
		if err == nil && len(b) == ed25519.PublicKeySize {
			return ed25519.PublicKey(b), nil
		}
	}
	return nil, fmt.Errorf("license public key must be base64 ed25519 public key")
}
