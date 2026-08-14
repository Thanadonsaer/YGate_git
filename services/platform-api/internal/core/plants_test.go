package core

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestValidatePlant(t *testing.T) {
	latitude, longitude := 13.7563, 100.5018
	dc, ac := 1250.5, 1000.0
	code, name, timezone, err := validatePlant("  bkk-01 ", "  Bangkok Solar  ", "Asia/Bangkok", &latitude, &longitude, &dc, &ac)
	if err != nil {
		t.Fatal(err)
	}
	if code != "BKK-01" || name != "Bangkok Solar" || timezone != "Asia/Bangkok" {
		t.Fatalf("normalized values = %q, %q, %q", code, name, timezone)
	}
}

func TestValidatePlantAcceptsLegacyEqualsCode(t *testing.T) {
	code, _, _, err := validatePlant("ne=49712672", "Legacy Plant", "Asia/Bangkok", nil, nil, nil, nil)
	if err != nil || code != "NE=49712672" {
		t.Fatalf("validatePlant() = %q, %v; want normalized legacy code", code, err)
	}
}

func TestValidatePlantAcceptsCodeWithSpacesAndParens(t *testing.T) {
	code, _, _, err := validatePlant("  vistec i  ", "VISTEC I", "Asia/Bangkok", nil, nil, nil, nil)
	if err != nil || code != "VISTEC I" {
		t.Fatalf("validatePlant() = %q, %v; want normalized real-world code with a space", code, err)
	}
}

func TestValidatePlantDefaultsEmptyTimezoneToBangkok(t *testing.T) {
	_, _, timezone, err := validatePlant("BKK-01", "Bangkok Solar", "  ", nil, nil, nil, nil)
	if err != nil || timezone != "Asia/Bangkok" {
		t.Fatalf("timezone=%q err=%v", timezone, err)
	}
}

func TestValidatePlantRejectsInvalidValues(t *testing.T) {
	nan := math.NaN()
	latitude := 91.0
	negativeCapacity := -1.0
	tests := []struct {
		name     string
		code     string
		timezone string
		latitude *float64
		dc       *float64
	}{
		{name: "invalid code", code: "plant#code", timezone: "UTC"},
		{name: "invalid timezone", code: "PLANT-1", timezone: "Mars/Olympus"},
		{name: "latitude out of range", code: "PLANT-1", timezone: "UTC", latitude: &latitude},
		{name: "negative capacity", code: "PLANT-1", timezone: "UTC", dc: &negativeCapacity},
		{name: "not a number", code: "PLANT-1", timezone: "UTC", dc: &nan},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, err := validatePlant(test.code, "Plant", test.timezone, test.latitude, nil, test.dc, nil)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestPlantFromFieldsIncludesImageURL(t *testing.T) {
	imageURL := "/api/v1/plants/plant/image/photo.webp"
	plant := plantFromFields(
		pgtype.UUID{Valid: true}, pgtype.UUID{Valid: true}, "Org", "P-1", "Plant", "UTC",
		pgtype.Float8{}, pgtype.Float8{}, pgtype.Numeric{}, pgtype.Numeric{}, PlantLifecycleOperational, &imageURL,
		true, false, pgtype.UUID{}, pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true}, pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true},
	)
	if plant.ImageURL == nil || *plant.ImageURL != imageURL {
		t.Fatalf("ImageURL = %#v, want %q", plant.ImageURL, imageURL)
	}
}
