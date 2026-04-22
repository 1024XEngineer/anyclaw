package schedule

import (
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
