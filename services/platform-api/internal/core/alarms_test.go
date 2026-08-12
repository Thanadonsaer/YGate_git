package core

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateAlarmRule(t *testing.T) {
	minValue, maxValue := 0.0, 100.0
	label, severity, delaySeconds, conditions, err := validateAlarmRule(
		"  High temperature  ", "  MAJOR  ", 300,
		[]ConditionInput{
			{PointKey: "  temp  ", MinValue: &minValue, MaxValue: &maxValue},
			{PointKey: "rpm", MinValue: &minValue, Logic: "  or  "},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if label != "High temperature" || severity != "major" || delaySeconds != 300 || len(conditions) != 2 {
		t.Fatalf("normalized = %q %q %d %+v", label, severity, delaySeconds, conditions)
	}
	if conditions[0].PointKey != "temp" || *conditions[0].MinValue != 0 || *conditions[0].MaxValue != 100 {
		t.Fatalf("condition = %+v", conditions[0])
	}
	if conditions[1].Logic != "OR" {
		t.Fatalf("second condition logic = %q, want OR", conditions[1].Logic)
	}
}

func TestValidateAlarmRuleDefaultsConditionLogicToAND(t *testing.T) {
	zero := 0.0
	// The first condition joins to nothing, so even an explicit OR on it
	// normalizes to AND rather than being rejected.
	_, _, _, conditions, err := validateAlarmRule("ok", "warning", 0, []ConditionInput{
		{PointKey: "x", MinValue: &zero, Logic: "OR"},
		{PointKey: "y", MinValue: &zero},
	})
	if err != nil || conditions[0].Logic != "AND" || conditions[1].Logic != "AND" {
		t.Fatalf("conditions=%+v err=%v", conditions, err)
	}
}

func TestValidateAlarmRuleRejectsBadValues(t *testing.T) {
	zero, hundred := 0.0, 100.0
	backwards := 200.0
	nan := 0.0
	nan = nan / nan
	for _, test := range []struct {
		name         string
		label        string
		severity     string
		delaySeconds int32
		conditions   []ConditionInput
	}{
		{name: "empty label", label: "  ", severity: "warning", conditions: []ConditionInput{{PointKey: "x", MinValue: &zero}}},
		{name: "label too long", label: strings.Repeat("a", 201), severity: "warning", conditions: []ConditionInput{{PointKey: "x", MinValue: &zero}}},
		{name: "unknown severity", label: "ok", severity: "urgent", conditions: []ConditionInput{{PointKey: "x", MinValue: &zero}}},
		{name: "unknown condition logic", label: "ok", severity: "warning", conditions: []ConditionInput{{PointKey: "x", MinValue: &zero}, {PointKey: "y", MinValue: &zero, Logic: "XOR"}}},
		{name: "no conditions", label: "ok", severity: "warning"},
		{name: "empty point key", label: "ok", severity: "warning", conditions: []ConditionInput{{PointKey: "  ", MinValue: &zero}}},
		{name: "no threshold", label: "ok", severity: "warning", conditions: []ConditionInput{{PointKey: "x"}}},
		{name: "min >= max", label: "ok", severity: "warning", conditions: []ConditionInput{{PointKey: "x", MinValue: &backwards, MaxValue: &hundred}}},
		{name: "NaN threshold", label: "ok", severity: "warning", conditions: []ConditionInput{{PointKey: "x", MinValue: &nan}}},
		{name: "negative alarm delay", label: "ok", severity: "warning", delaySeconds: -1, conditions: []ConditionInput{{PointKey: "x", MinValue: &zero}}},
		{name: "alarm delay past a day", label: "ok", severity: "warning", delaySeconds: MaxAlarmDelaySeconds + 1, conditions: []ConditionInput{{PointKey: "x", MinValue: &zero}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, _, _, err := validateAlarmRule(test.label, test.severity, test.delaySeconds, test.conditions); !errors.Is(err, ErrAlarmRuleInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestEvaluateConditions(t *testing.T) {
	for _, test := range []struct {
		name    string
		results []bool
		logic   []string
		want    bool
	}{
		{name: "single true", results: []bool{true}, logic: []string{"AND"}, want: true},
		{name: "single false", results: []bool{false}, logic: []string{"AND"}, want: false},
		{name: "all AND needs all", results: []bool{true, true, false}, logic: []string{"AND", "AND", "AND"}, want: false},
		{name: "all AND satisfied", results: []bool{true, true}, logic: []string{"AND", "AND"}, want: true},
		{name: "all OR needs one", results: []bool{false, false, true}, logic: []string{"AND", "OR", "OR"}, want: true},
		{name: "all OR none match", results: []bool{false, false}, logic: []string{"AND", "OR"}, want: false},
		// The whole point of the change: AND binds tighter, so this is
		// "(A and B) or C" -- fires on C alone even though B is false.
		{name: "A and B or C, C only", results: []bool{true, false, true}, logic: []string{"AND", "AND", "OR"}, want: true},
		{name: "A and B or C, A and B only", results: []bool{true, true, false}, logic: []string{"AND", "AND", "OR"}, want: true},
		{name: "A and B or C, nothing", results: []bool{true, false, false}, logic: []string{"AND", "AND", "OR"}, want: false},
		// "A or (B and C)": a lone A fires it, a lone B does not.
		{name: "A or B and C, A only", results: []bool{true, false, false}, logic: []string{"AND", "OR", "AND"}, want: true},
		{name: "A or B and C, B only", results: []bool{false, true, false}, logic: []string{"AND", "OR", "AND"}, want: false},
		{name: "A or B and C, B and C", results: []bool{false, true, true}, logic: []string{"AND", "OR", "AND"}, want: true},
		{name: "no conditions never fires", results: nil, logic: nil, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := EvaluateConditions(test.results, test.logic); got != test.want {
				t.Fatalf("EvaluateConditions(%v, %v) = %v, want %v", test.results, test.logic, got, test.want)
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

func TestAlarmSuppressed(t *testing.T) {
	last := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	const delay = 1800 // 30 minutes

	for _, test := range []struct {
		name         string
		last         time.Time
		observedAt   time.Time
		delaySeconds int32
		want         bool
	}{
		{name: "no delay configured", last: last, observedAt: last.Add(time.Second), delaySeconds: 0, want: false},
		{name: "rule has never alarmed", last: time.Time{}, observedAt: last, delaySeconds: delay, want: false},
		{name: "inside the window", last: last, observedAt: last.Add(10 * time.Minute), delaySeconds: delay, want: true},
		{name: "one second short of the window", last: last, observedAt: last.Add(30*time.Minute - time.Second), delaySeconds: delay, want: true},
		// Exclusive boundary: exactly N seconds later is allowed to alarm, so a
		// 30-minute delay does not silently become "30 minutes plus one poll".
		{name: "exactly at the boundary", last: last, observedAt: last.Add(30 * time.Minute), delaySeconds: delay, want: false},
		{name: "past the window", last: last, observedAt: last.Add(31 * time.Minute), delaySeconds: delay, want: false},
		// A late-arriving reading older than the last alarm must not open a
		// second alarm behind the one already recorded.
		{name: "reading older than the last alarm", last: last, observedAt: last.Add(-time.Minute), delaySeconds: delay, want: true},
		{name: "negative delay behaves as disabled", last: last, observedAt: last, delaySeconds: -5, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := AlarmSuppressed(test.last, test.observedAt, test.delaySeconds); got != test.want {
				t.Fatalf("AlarmSuppressed(%v, %v, %d) = %v, want %v", test.last, test.observedAt, test.delaySeconds, got, test.want)
			}
		})
	}
}
