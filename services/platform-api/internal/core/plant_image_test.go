package core

import (
	"bytes"
	"testing"
)

func TestValidatePlantImage(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		ext     string
		wantErr bool
	}{
		{name: "png", data: append([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{0}, 16)...), ext: ".png"},
		{name: "jpeg", data: append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte{0}, 16)...), ext: ".jpg"},
		{name: "webp", data: append([]byte("RIFF"), append([]byte{0, 0, 0, 0}, []byte("WEBP")...)...), ext: ".webp"},
		{name: "text", data: []byte("not an image"), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ext, err := validatePlantImage(test.data)
			if test.wantErr {
				if err == nil {
					t.Fatal("validatePlantImage() error = nil, want error")
				}
				return
			}
			if err != nil || ext != test.ext {
				t.Fatalf("validatePlantImage() = %q, %v; want %q", ext, err, test.ext)
			}
		})
	}
}
