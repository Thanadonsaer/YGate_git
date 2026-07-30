package core

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateAlarmThresholds(t *testing.T) {
	minValue, maxValue := 0.0, 100.0
	label, severity, min, max, err := validateAlarmThresholds("  High temperature  ", "  MAJOR  ", &minValue, &maxValue)
	if err != nil {
		t.Fatal(err)
	}
	if label != "High temperature" || severity != "major" || *min != 0 || *max != 100 {
		t.Fatalf("normalized = %q %q %v %v", label, severity, min, max)
	}
}

func TestValidateAlarmThresholdsRejectsBadValues(t *testing.T) {
	zero, hundred := 0.0, 100.0
	backwards := 200.0
	nan := 0.0
	nan = nan / nan
	for _, test := range []struct {
		name     string
		label    string
		severity string
		minValue *float64
		maxValue *float64
	}{
		{name: "empty label", label: "  ", severity: "warning", minValue: &zero},
		{name: "label too long", label: strings.Repeat("a", 201), severity: "warning", minValue: &zero},
		{name: "unknown severity", label: "ok", severity: "urgent", minValue: &zero},
		{name: "no threshold", label: "ok", severity: "warning"},
		{name: "min >= max", label: "ok", severity: "warning", minValue: &backwards, maxValue: &hundred},
		{name: "NaN threshold", label: "ok", severity: "warning", minValue: &nan},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, _, _, err := validateAlarmThresholds(test.label, test.severity, test.minValue, test.maxValue); !errors.Is(err, ErrAlarmRuleInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestParseInt64(t *testing.T) {
	if id, err := parseInt64(" 42 "); err != nil || id != 42 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	for _, bad := range []string{"0", "-1", "12abc", "abc", ""} {
		if _, err := parseInt64(bad); !errors.Is(err, ErrAlarmEventNotFound) {
			t.Fatalf("input=%q error=%v", bad, err)
		}
	}
}
