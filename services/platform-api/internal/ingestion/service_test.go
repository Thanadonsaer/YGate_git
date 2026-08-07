package ingestion

import "testing"

func TestCanonicalRegisterKeyConvertsLegacyAddress(t *testing.T) {
	for input, want := range map[string]string{
		"40001":        "reg40001",
		" reg40002 ":   "reg40002",
		"reg40003":     "reg40003",
		"active_power": "active_power",
	} {
		if got := canonicalRegisterKey(input); got != want {
			t.Errorf("canonicalRegisterKey(%q) = %q, want %q", input, got, want)
		}
	}
}
