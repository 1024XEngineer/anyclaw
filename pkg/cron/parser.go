package cron

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CronField represents a parsed cron field (minute, hour, etc.)
// It supports specific values, ranges (1-5), lists (1,3,5), steps (*/5), and wildcards (*).
type CronField struct {
	Values []int // sorted list of matching values
	Min    int
	Max    int
}

// CronSpec is a fully parsed cron specification supporting all standard features.
type CronSpec struct {
	Minute     CronField
	Hour       CronField
	DayOfMonth CronField
	Month      CronField
	DayOfWeek  CronField
	Original   string
}

// ParseCronSpec parses a full cron expression with support for:
//   - Wildcards: *
//   - Ranges: 1-5
//   - Lists: 1,3,5
//   - Steps: */5, 1-10/2
//   - Predefined aliases: @hourly, @daily, @weekly, @monthly, @yearly, @every 5m
type CronSpecParser struct{}

// Parse parses a cron expression string into a CronSpec.
func ParseCronSpec(expr string) (*CronSpec, error) {
	expr = strings.TrimSpace(expr)

	// Handle predefined aliases
	switch strings.ToLower(expr) {
	case "@yearly", "@annually":
		expr = "0 0 1 1 *"
	case "@monthly":
		expr = "0 0 1 * *"
	case "@weekly":
		expr = "0 0 * * 0"
	case "@daily", "@midnight":
		expr = "0 0 * * *"
	case "@hourly":
		expr = "0 * * * *"
	}

	// Handle @every duration
	if strings.HasPrefix(expr, "@every ") {
		_, err := time.ParseDuration(strings.TrimPrefix(expr, "@every "))
		if err != nil {
			return nil, fmt.Errorf("invalid @every duration: %w", err)
		}
		return &CronSpec{
			Original:   expr,
			Minute:     CronField{Values: []int{-1}, Min: 0, Max: 59},
			Hour:       CronField{Values: []int{-1}, Min: 0, Max: 23},
			DayOfMonth: CronField{Values: []int{-1}, Min: 1, Max: 31},
			Month:      CronField{Values: []int{-1}, Min: 1, Max: 12},
			DayOfWeek:  CronField{Values: []int{-1}, Min: 0, Max: 6},
		}, nil
	}

	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron expression must have 5 fields (got %d): %s", len(parts), expr)
	}

	spec := &CronSpec{Original: expr}

	var err error
	spec.Minute, err = parseField(parts[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("invalid minute field %q: %w", parts[0], err)
	}

	spec.Hour, err = parseField(parts[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("invalid hour field %q: %w", parts[1], err)
	}

	spec.DayOfMonth, err = parseField(parts[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("invalid day-of-month field %q: %w", parts[2], err)
	}

	spec.Month, err = parseField(parts[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("invalid month field %q: %w", parts[3], err)
	}

	spec.DayOfWeek, err = parseField(parts[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("invalid day-of-week field %q: %w", parts[4], err)
	}

	return spec, nil
}

// Next returns the next time after t that matches the cron spec.
func (s *CronSpec) Next(t time.Time) time.Time {
	loc := t.Location()
	start := t.Add(time.Second)

	// Search forward up to 4 years (to handle leap years and all combinations)
	deadline := t.AddDate(4, 0, 0)

	for current := start; current.Before(deadline); {
		// Check month
		month := int(current.Month())
		if !s.Month.matches(month) {
			// Jump to next matching month
			nextMonth := s.Month.nextValue(month)
			if nextMonth == -1 {
				nextMonth = s.Month.Values[0]
				current = time.Date(current.Year()+1, time.Month(nextMonth), 1, 0, 0, 0, 0, loc)
			} else {
				current = time.Date(current.Year(), time.Month(nextMonth), 1, 0, 0, 0, 0, loc)
			}
			continue
		}

		// Check day (both day-of-month and day-of-week)
		day := current.Day()
		domMatch := s.DayOfMonth.matches(day)
		dowMatch := s.DayOfWeek.matches(int(current.Weekday()))

		// If both are restricted (not *), either can match (OR logic per standard cron)
		// If only one is restricted, that one must match
		// If neither is restricted (both *), any day matches
		domRestricted := !s.DayOfMonth.isWildcard()
		dowRestricted := !s.DayOfWeek.isWildcard()

		dayMatch := false
		if domRestricted && dowRestricted {
			dayMatch = domMatch || dowMatch
		} else if domRestricted {
			dayMatch = domMatch
		} else if dowRestricted {
			dayMatch = dowMatch
		} else {
			dayMatch = true
		}

		if !dayMatch {
			current = time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, loc)
			continue
		}

		// Check hour
		hour := current.Hour()
		if !s.Hour.matches(hour) {
			nextHour := s.Hour.nextValue(hour)
			if nextHour == -1 {
				// No more matching hours today, go to next day
				current = time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, loc)
			} else {
				current = time.Date(current.Year(), current.Month(), current.Day(), nextHour, 0, 0, 0, loc)
			}
			continue
		}

		// Check minute
		minute := current.Minute()
		if !s.Minute.matches(minute) {
			nextMinute := s.Minute.nextValue(minute)
			if nextMinute == -1 {
				// No more matching minutes this hour, go to next hour
				nextHour := s.Hour.nextValue(hour + 1)
				if nextHour == -1 {
					current = time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, loc)
				} else {
					current = time.Date(current.Year(), current.Month(), current.Day(), nextHour, 0, 0, 0, loc)
				}
			} else {
				current = time.Date(current.Year(), current.Month(), current.Day(), hour, nextMinute, 0, 0, loc)
			}
			continue
		}

		// All fields match
		return current
	}

	// No match found within 4 years
	return time.Time{}
}

// Validate returns an error if the cron spec is invalid.
func (s *CronSpec) Validate() error {
	if len(s.Minute.Values) == 0 {
		return fmt.Errorf("minute field has no valid values")
	}
	if len(s.Hour.Values) == 0 {
		return fmt.Errorf("hour field has no valid values")
	}
	return nil
}

// IsWildcard returns true if the field matches all values.
func (f *CronField) isWildcard() bool {
	return len(f.Values) == 1 && f.Values[0] == -1
}

// matches checks if a value is in the field's values.
func (f *CronField) matches(v int) bool {
	if f.isWildcard() {
		return true
	}
	for _, val := range f.Values {
		if val == v {
			return true
		}
	}
	return false
}

// nextValue returns the next value in the field that is greater than v, or -1 if none.
func (f *CronField) nextValue(v int) int {
	if f.isWildcard() {
		return -1
	}
	for _, val := range f.Values {
		if val > v {
			return val
		}
	}
	return -1
}

// parseField parses a single cron field (e.g., "*/5", "1-5", "1,3,5", "*").
func parseField(field string, min, max int) (CronField, error) {
	values := make(map[int]bool)

	// Handle comma-separated list
	parts := strings.Split(field, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		fieldValues, err := parseFieldPart(part, min, max)
		if err != nil {
			return CronField{}, err
		}

		for _, v := range fieldValues {
			values[v] = true
		}
	}

	if len(values) == 0 {
		return CronField{}, fmt.Errorf("no valid values in field")
	}

	// Convert to sorted slice
	sorted := make([]int, 0, len(values))
	for v := range values {
		sorted = append(sorted, v)
	}
	sort.Ints(sorted)

	return CronField{
		Values: sorted,
		Min:    min,
		Max:    max,
	}, nil
}

// parseFieldPart parses a single part of a cron field (handles *, ranges, steps).
func parseFieldPart(part string, min, max int) ([]int, error) {
	// Handle step values
	step := 1
	if idx := strings.Index(part, "/"); idx != -1 {
		stepStr := part[idx+1:]
		var err error
		step, err = strconv.Atoi(stepStr)
		if err != nil {
			return nil, fmt.Errorf("invalid step value %q: %w", stepStr, err)
		}
		if step < 1 {
			return nil, fmt.Errorf("step value must be >= 1")
		}
		part = part[:idx]
	}

	var values []int

	if part == "*" {
		// Wildcard with optional step
		for i := min; i <= max; i += step {
			values = append(values, i)
		}
	} else if strings.Contains(part, "-") {
		// Range with optional step
		rangeParts := strings.SplitN(part, "-", 2)
		start, err := strconv.Atoi(rangeParts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid range start %q: %w", rangeParts[0], err)
		}
		end, err := strconv.Atoi(rangeParts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid range end %q: %w", rangeParts[1], err)
		}
		if start > end {
			return nil, fmt.Errorf("range start %d > end %d", start, end)
		}
		if start < min || end > max {
			return nil, fmt.Errorf("range %d-%d out of bounds [%d-%d]", start, end, min, max)
		}
		for i := start; i <= end; i += step {
			values = append(values, i)
		}
	} else {
		// Single value
		if step != 1 {
			return nil, fmt.Errorf("step not allowed on single value")
		}
		v, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid value %q: %w", part, err)
		}
		if v < min || v > max {
			return nil, fmt.Errorf("value %d out of range [%d-%d]", v, min, max)
		}
		values = append(values, v)
	}

	return values, nil
}

// Format returns a human-readable description of the cron schedule.
func (s *CronSpec) Format() string {
	if s.Original != "" {
		return s.Original
	}

	parts := []string{
		formatField(s.Minute, 0, 59),
		formatField(s.Hour, 0, 23),
		formatField(s.DayOfMonth, 1, 31),
		formatField(s.Month, 1, 12),
		formatField(s.DayOfWeek, 0, 6),
	}
	return strings.Join(parts, " ")
}

func formatField(f CronField, min, max int) string {
	if f.isWildcard() {
		return "*"
	}

	// Check if it's a simple step pattern
	if len(f.Values) > 1 {
		step := f.Values[1] - f.Values[0]
		isStep := true
		for i := 2; i < len(f.Values); i++ {
			if f.Values[i]-f.Values[i-1] != step {
				isStep = false
				break
			}
		}
		if isStep && f.Values[0] == min {
			return fmt.Sprintf("*/%d", step)
		}
	}

	// Check if it's a range
	if len(f.Values) > 1 {
		isRange := true
		for i := 1; i < len(f.Values); i++ {
			if f.Values[i] != f.Values[i-1]+1 {
				isRange = false
				break
			}
		}
		if isRange {
			return fmt.Sprintf("%d-%d", f.Values[0], f.Values[len(f.Values)-1])
		}
	}

	// List of values
	parts := make([]string, len(f.Values))
	for i, v := range f.Values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}

// CronSpecForTask returns a parsed CronSpec for a task's schedule.
func CronSpecForTask(schedule string) (*CronSpec, error) {
	return ParseCronSpec(schedule)
}

// NextRunTimes returns the next N run times for a cron expression.
func NextRunTimes(schedule string, from time.Time, count int) ([]time.Time, error) {
	spec, err := ParseCronSpec(schedule)
	if err != nil {
		return nil, err
	}

	times := make([]time.Time, 0, count)
	current := from
	for i := 0; i < count; i++ {
		next := spec.Next(current)
		if next.IsZero() {
			break
		}
		times = append(times, next)
		current = next
	}

	return times, nil
}

// ValidateCronExpression validates a cron expression and returns a description.
func ValidateCronExpression(expr string) (string, error) {
	spec, err := ParseCronSpec(expr)
	if err != nil {
		return "", err
	}

	if err := spec.Validate(); err != nil {
		return "", err
	}

	return describeSchedule(spec), nil
}

func describeSchedule(spec *CronSpec) string {
	if spec.isEveryMinute() {
		return "Every minute"
	}
	if spec.isEveryHour() {
		return "Every hour"
	}
	if spec.isDaily() {
		return fmt.Sprintf("Every day at %02d:%02d", spec.Hour.Values[0], spec.Minute.Values[0])
	}
	if spec.isWeekly() {
		days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
		day := "Sunday"
		if len(spec.DayOfWeek.Values) > 0 && spec.DayOfWeek.Values[0] >= 0 && spec.DayOfWeek.Values[0] <= 6 {
			day = days[spec.DayOfWeek.Values[0]]
		}
		return fmt.Sprintf("Every %s at %02d:%02d", day, spec.Hour.Values[0], spec.Minute.Values[0])
	}
	if spec.isMonthly() {
		return fmt.Sprintf("On day %d of every month at %02d:%02d", spec.DayOfMonth.Values[0], spec.Hour.Values[0], spec.Minute.Values[0])
	}
	return "Custom schedule: " + spec.Format()
}

func (s *CronSpec) isEveryMinute() bool {
	return s.Minute.isWildcard() && s.Hour.isWildcard() && s.DayOfMonth.isWildcard() && s.Month.isWildcard() && s.DayOfWeek.isWildcard()
}

func (s *CronSpec) isEveryHour() bool {
	return !s.Minute.isWildcard() && len(s.Minute.Values) == 1 && s.Hour.isWildcard() && s.DayOfMonth.isWildcard() && s.Month.isWildcard() && s.DayOfWeek.isWildcard()
}

func (s *CronSpec) isDaily() bool {
	return !s.Minute.isWildcard() && len(s.Minute.Values) == 1 && !s.Hour.isWildcard() && len(s.Hour.Values) == 1 && s.DayOfMonth.isWildcard() && s.Month.isWildcard() && s.DayOfWeek.isWildcard()
}

func (s *CronSpec) isWeekly() bool {
	return !s.Minute.isWildcard() && len(s.Minute.Values) == 1 && !s.Hour.isWildcard() && len(s.Hour.Values) == 1 && s.DayOfMonth.isWildcard() && s.Month.isWildcard() && !s.DayOfWeek.isWildcard() && len(s.DayOfWeek.Values) == 1
}

func (s *CronSpec) isMonthly() bool {
	return !s.Minute.isWildcard() && len(s.Minute.Values) == 1 && !s.Hour.isWildcard() && len(s.Hour.Values) == 1 && !s.DayOfMonth.isWildcard() && len(s.DayOfMonth.Values) == 1 && s.Month.isWildcard() && s.DayOfWeek.isWildcard()
}
