package core

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateAlarmRule(t *testing.T) {
	minValue, maxValue := 0.0, 100.0
	label, severity, conditionLogic, conditions, err := validateAlarmRule(
		"  High temperature  ", "  MAJOR  ", "  or  ",
		[]ConditionInput{{PointKey: "  temp  ", MinValue: &minValue, MaxValue: &maxValue}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if label != "High temperature" || severity != "major" || conditionLogic != "OR" || len(conditions) != 1 {
		t.Fatalf("normalized = %q %q %q %+v", label, severity, conditionLogic, conditions)
	}
	if conditions[0].PointKey != "temp" || *conditions[0].MinValue != 0 || *conditions[0].MaxValue != 100 {
		t.Fatalf("condition = %+v", conditions[0])
	}
}

func TestValidateAlarmRuleDefaultsConditionLogicToAND(t *testing.T) {
	zero := 0.0
	_, _, conditionLogic, _, err := validateAlarmRule("ok", "warning", "", []ConditionInput{{PointKey: "x", MinValue: &zero}})
	if err != nil || conditionLogic != "AND" {
		t.Fatalf("conditionLogic=%q err=%v", conditionLogic, err)
	}
}

func TestValidateAlarmRuleRejectsBadValues(t *testing.T) {
	zero, hundred := 0.0, 100.0
	backwards := 200.0
	nan := 0.0
	nan = nan / nan
	for _, test := range []struct {
		name           string
		label          string
		severity       string
		conditionLogic string
		conditions     []ConditionInput
	}{
		{name: "empty label", label: "  ", severity: "warning", conditions: []ConditionInput{{PointKey: "x", MinValue: &zero}}},
		{name: "label too long", label: strings.Repeat("a", 201), severity: "warning", conditions: []ConditionInput{{PointKey: "x", MinValue: &zero}}},
		{name: "unknown severity", label: "ok", severity: "urgent", conditions: []ConditionInput{{PointKey: "x", MinValue: &zero}}},
		{name: "unknown condition logic", label: "ok", severity: "warning", conditionLogic: "XOR", conditions: []ConditionInput{{PointKey: "x", MinValue: &zero}}},
		{name: "no conditions", label: "ok", severity: "warning"},
		{name: "empty point key", label: "ok", severity: "warning", conditions: []ConditionInput{{PointKey: "  ", MinValue: &zero}}},
		{name: "no threshold", label: "ok", severity: "warning", conditions: []ConditionInput{{PointKey: "x"}}},
		{name: "min >= max", label: "ok", severity: "warning", conditions: []ConditionInput{{PointKey: "x", MinValue: &backwards, MaxValue: &hundred}}},
		{name: "NaN threshold", label: "ok", severity: "warning", conditions: []ConditionInput{{PointKey: "x", MinValue: &nan}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, _, _, err := validateAlarmRule(test.label, test.severity, test.conditionLogic, test.conditions); !errors.Is(err, ErrAlarmRuleInvalid) {
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
