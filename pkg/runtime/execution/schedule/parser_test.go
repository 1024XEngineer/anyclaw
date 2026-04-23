package schedule

import (
	"strings"
	"testing"
	"time"
)

func TestParseCronSpecEvery(t *testing.T) {
	spec, err := ParseCronSpec("@every 5m")
	if err != nil {
		t.Fatalf("ParseCronSpec failed: %v", err)
	}
	if spec.Every != 5*time.Minute {
		t.Fatalf("expected 5m interval, got %s", spec.Every)
	}

	start := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	next := spec.Next(start)
	want := start.Add(5 * time.Minute)
	if !next.Equal(want) {
		t.Fatalf("expected next run %s, got %s", want, next)
	}
}

func TestParseCronSpecRangeAndStep(t *testing.T) {
	spec, err := ParseCronSpec("*/15 9-17 * * 1-5")
	if err != nil {
		t.Fatalf("ParseCronSpec failed: %v", err)
	}

	start := time.Date(2026, 4, 20, 9, 7, 0, 0, time.UTC)
	next := spec.Next(start)
	want := time.Date(2026, 4, 20, 9, 15, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("expected %s, got %s", want, next)
	}
}

func TestValidateCronExpression(t *testing.T) {
	description, err := ValidateCronExpression("@daily")
	if err != nil {
		t.Fatalf("ValidateCronExpression failed: %v", err)
	}
	if description == "" {
		t.Fatal("expected non-empty description")
	}
}

func TestNextRunTimes(t *testing.T) {
	start := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	times, err := NextRunTimes("0 * * * *", start, 3)
	if err != nil {
		t.Fatalf("NextRunTimes failed: %v", err)
	}
	if len(times) != 3 {
		t.Fatalf("expected 3 run times, got %d", len(times))
	}
	if !times[0].Equal(time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected first run time: %s", times[0])
	}
}

func TestParseCronSpecErrorsAndHelpers(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{name: "empty", expr: "", want: "cron expression is required"},
		{name: "bad fields", expr: "* * *", want: "must have 5 fields"},
		{name: "bad every", expr: "@every nope", want: "invalid @every duration"},
		{name: "bad range", expr: "5-1 * * * *", want: "range start 5 > end 1"},
		{name: "bad value", expr: "100 * * * *", want: "out of range"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCronSpec(tc.expr)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParseCronSpec(%q) error = %v, want substring %q", tc.expr, err, tc.want)
			}
		})
	}

	if _, err := parseFieldPart("1/2", 0, 59); err == nil || !strings.Contains(err.Error(), "step not allowed") {
		t.Fatalf("expected single-value step parse failure, got %v", err)
	}
	if _, err := parseFieldPart("*/0", 0, 59); err == nil || !strings.Contains(err.Error(), ">= 1") {
		t.Fatalf("expected zero-step parse failure, got %v", err)
	}
}

func TestCronSpecFormatDescribeAndValidateEdges(t *testing.T) {
	if got := (*CronSpec)(nil).Format(); got != "" {
		t.Fatalf("expected nil spec format to be empty, got %q", got)
	}
	if got := (*CronSpec)(nil).Next(time.Now()); !got.IsZero() {
		t.Fatalf("expected nil spec next time to be zero, got %s", got)
	}
	if err := (*CronSpec)(nil).Validate(); err == nil {
		t.Fatal("expected nil spec validate to fail")
	}

	empty := &CronSpec{}
	if err := empty.Validate(); err == nil {
		t.Fatal("expected empty spec validate to fail")
	}

	hourly, err := ParseCronSpec("@hourly")
	if err != nil {
		t.Fatalf("ParseCronSpec(@hourly) failed: %v", err)
	}
	if got := hourly.Format(); got != "0 * * * *" {
		t.Fatalf("expected normalized hourly format, got %q", got)
	}
	if got := describeSchedule(hourly); !strings.Contains(got, "Every hour") {
		t.Fatalf("expected hourly description, got %q", got)
	}

	weekly, err := ParseCronSpec("30 8 * * 1")
	if err != nil {
		t.Fatalf("ParseCronSpec(weekly) failed: %v", err)
	}
	if got := describeSchedule(weekly); !strings.Contains(got, "Monday") {
		t.Fatalf("expected weekly description to mention Monday, got %q", got)
	}

	custom := &CronSpec{
		Minute:     CronField{Values: []int{5}, Min: 0, Max: 59},
		Hour:       CronField{Values: []int{7}, Min: 0, Max: 23},
		DayOfMonth: CronField{Values: []int{1, 15}, Min: 1, Max: 31},
		Month:      CronField{Values: []int{-1}, Min: 1, Max: 12},
		DayOfWeek:  CronField{Values: []int{-1}, Min: 0, Max: 6},
	}
	if got := custom.Format(); got != "5 7 1,15 * *" {
		t.Fatalf("unexpected custom format: %q", got)
	}
}

func TestCronHelpersAndNextRunCounts(t *testing.T) {
	if _, err := CronSpecForTask("@daily"); err != nil {
		t.Fatalf("CronSpecForTask failed: %v", err)
	}

	start := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	times, err := NextRunTimes("@daily", start, 0)
	if err != nil {
		t.Fatalf("NextRunTimes zero count failed: %v", err)
	}
	if len(times) != 0 {
		t.Fatalf("expected zero run times for zero count, got %d", len(times))
	}

	field := CronField{Values: []int{-1}, Min: 0, Max: 59}
	if !field.matches(42) {
		t.Fatal("expected wildcard field to match arbitrary value")
	}
	if got := field.nextValue(10); got != 11 {
		t.Fatalf("expected wildcard next value to increment, got %d", got)
	}
	if got := field.nextValue(59); got != -1 {
		t.Fatalf("expected wildcard next value at max to return -1, got %d", got)
	}
}
