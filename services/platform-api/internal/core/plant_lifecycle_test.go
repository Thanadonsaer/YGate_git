package core

import (
	"errors"
	"testing"
)

func TestPlantLifecycleValidation(t *testing.T) {
	for _, status := range []string{"IN_CONSTRUCTION", "OPERATIONAL", "OFFLINE", "RETIRED"} {
		if err := validatePlantLifecycle(status); err != nil {
			t.Fatalf("status %q rejected: %v", status, err)
		}
	}
	if err := validatePlantLifecycle("UNKNOWN"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown status error = %v, want ErrInvalid", err)
	}
}
