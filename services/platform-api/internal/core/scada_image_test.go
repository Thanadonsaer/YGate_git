package core

import "testing"

func TestValidateScadaImage(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n")
	if ext, err := validateScadaImage(png); err != nil || ext != ".png" {
		t.Fatalf("valid PNG rejected: ext=%q err=%v", ext, err)
	}
	if _, err := validateScadaImage([]byte("not an image")); err == nil {
		t.Fatal("non-image accepted")
	}
	if _, err := validateScadaImage(make([]byte, maxScadaImageBytes+1)); err == nil {
		t.Fatal("oversized image accepted")
	}
}
