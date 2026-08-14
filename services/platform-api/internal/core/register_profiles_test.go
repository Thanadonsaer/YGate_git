package core

import "testing"

func TestNormalizeRegisterProfileName(t *testing.T) {
	if got := normalizeRegisterProfileName("  Huawei SUN2000  "); got != "Huawei SUN2000" {
		t.Fatalf("normalizeRegisterProfileName() = %q", got)
	}
}
