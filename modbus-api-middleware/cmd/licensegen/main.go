package main

import (
	"chpp/modbus-api-middleware/internal/license"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	generateKeypair := flag.Bool("generate-keypair", false, "print a new ed25519 keypair and exit")
	privateKeyText := flag.String("private-key", os.Getenv("CHPP_LICENSE_PRIVATE_KEY"), "base64 ed25519 private key")
	customer := flag.String("customer", "CHPP", "customer name")
	machineID := flag.String("machine-id", "", "target machine id; use * for floating license")
	expires := flag.String("expires", time.Now().AddDate(1, 0, 0).UTC().Format("2006-01-02"), "expiry date YYYY-MM-DD or RFC3339")
	flag.Parse()
	if *generateKeypair {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("CHPP_LICENSE_PUBLIC_KEY=" + base64.StdEncoding.EncodeToString(pub))
		fmt.Println("CHPP_LICENSE_PRIVATE_KEY=" + base64.StdEncoding.EncodeToString(priv))
		return
	}
	if strings.TrimSpace(*privateKeyText) == "" || strings.TrimSpace(*machineID) == "" {
		log.Fatal("private-key and machine-id are required")
	}
	priv, err := base64.StdEncoding.DecodeString(strings.TrimSpace(*privateKeyText))
	if err != nil || len(priv) != ed25519.PrivateKeySize {
		log.Fatal("private-key must be base64 ed25519 private key")
	}
	expiresAt, err := parseExpiry(*expires)
	if err != nil {
		log.Fatal(err)
	}
	payload := license.Payload{Customer: *customer, MachineID: *machineID, ExpiresAt: expiresAt.UTC().Format(time.RFC3339), Features: []string{"modbus-middleware"}}
	b, err := json.Marshal(payload)
	if err != nil {
		log.Fatal(err)
	}
	part := base64.RawURLEncoding.EncodeToString(b)
	sig := ed25519.Sign(ed25519.PrivateKey(priv), []byte(part))
	fmt.Println(part + "." + base64.RawURLEncoding.EncodeToString(sig))
}

func parseExpiry(value string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", value)
}
