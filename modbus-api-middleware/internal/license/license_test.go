package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func signedToken(t *testing.T, private ed25519.PrivateKey, payload Payload) string {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	part := base64.RawURLEncoding.EncodeToString(b)
	sig := ed25519.Sign(private, []byte(part))
	return part + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestActivateAndCheckFile(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := base64.StdEncoding.EncodeToString(pub)
	t.Setenv("CHPP_LICENSE_MACHINE_ID", "TEST-MACHINE")
	token := signedToken(t, priv, Payload{Customer: "CHPP", MachineID: "TEST-MACHINE", ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), Features: []string{"modbus"}})
	path := filepath.Join(t.TempDir(), "license.json")
	status, err := Activate(path, token, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if status.Payload.Customer != "CHPP" || status.MachineID != "TEST-MACHINE" {
		t.Fatalf("status=%+v", status)
	}
	checked, err := CheckFile(path, publicKey)
	if err != nil || checked.Payload.Customer != "CHPP" {
		t.Fatalf("checked=%+v err=%v", checked, err)
	}
}

func TestRejectsWrongMachine(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHPP_LICENSE_MACHINE_ID", "REAL-MACHINE")
	token := signedToken(t, priv, Payload{Customer: "CHPP", MachineID: "OTHER-MACHINE"})
	if _, err = VerifyToken(token, base64.StdEncoding.EncodeToString(pub), MachineID(), time.Now()); err == nil {
		t.Fatal("expected wrong machine error")
	}
}
