package schedule

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CronField describes a single parsed cron field.
type CronField struct {
	Values []int
	Min    int
	Max    int
}

// CronSpec is a parsed cron schedule or an @every interval schedule.
type CronSpec struct {
	Minute     CronField
	Hour       CronField
	DayOfMonth CronField
	Month      CronField
	DayOfWeek  CronField
	Every      time.Duration
	Original   string
}

// ParseCronSpec parses a five-field cron expression plus common aliases.
func ParseCronSpec(expr string) (*CronSpec, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("cron expression is required")
	}

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

	if strings.HasPrefix(strings.ToLower(expr), "@every ") {
		d, err := time.ParseDuration(strings.TrimSpace(expr[len("@every "):]))
		if err != nil {
			return nil, fmt.Errorf("invalid @every duration: %w", err)
		}
		if d <= 0 {
			return nil, fmt.Errorf("@every duration must be greater than zero")
		}
		return &CronSpec{Every: d, Original: expr}, nil
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

// Next returns the first time after t that matches the spec.
func (s *CronSpec) Next(t time.Time) time.Time {
	if s == nil {
		return time.Time{}
	}
	if s.Every > 0 {
		return t.Add(s.Every)
	}

	loc := t.Location()
	current := t.Add(time.Second).Truncate(time.Minute)
	if !current.After(t) {
		current = current.Add(time.Minute)
	}
	deadline := t.AddDate(5, 0, 0)

	for current.Before(deadline) {
		if !s.Month.matches(int(current.Month())) {
			nextMonth := s.Month.nextValue(int(current.Month()))
			if nextMonth == -1 {
				nextMonth = s.Month.Values[0]
				current = time.Date(current.Year()+1, time.Month(nextMonth), 1, 0, 0, 0, 0, loc)
			} else {
				current = time.Date(current.Year(), time.Month(nextMonth), 1, 0, 0, 0, 0, loc)
			}
			continue
		}

		if !s.matchesDay(current) {
			current = time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, loc)
			continue
		}

		if !s.Hour.matches(current.Hour()) {
			nextHour := s.Hour.nextValue(current.Hour())
			if nextHour == -1 {
				current = time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, loc)
			} else {
				current = time.Date(current.Year(), current.Month(), current.Day(), nextHour, 0, 0, 0, loc)
			}
			continue
		}

		if !s.Minute.matches(current.Minute()) {
			nextMinute := s.Minute.nextValue(current.Minute())
			if nextMinute == -1 {
				nextHour := s.Hour.nextValue(current.Hour())
				if nextHour == -1 {
					current = time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, loc)
				} else {
					current = time.Date(current.Year(), current.Month(), current.Day(), nextHour, 0, 0, 0, loc)
				}
			} else {
				current = time.Date(current.Year(), current.Month(), current.Day(), current.Hour(), nextMinute, 0, 0, loc)
			}
			continue
		}

		if current.After(t) {
			return current
		}
		current = current.Add(time.Minute)
	}

	return time.Time{}
}

func (s *CronSpec) matchesDay(t time.Time) bool {
	domMatch := s.DayOfMonth.matches(t.Day())
	dowMatch := s.DayOfWeek.matches(int(t.Weekday()))
	domRestricted := !s.DayOfMonth.isWildcard()
	dowRestricted := !s.DayOfWeek.isWildcard()

	switch {
	case domRestricted && dowRestricted:
		return domMatch || dowMatch
	case domRestricted:
		return domMatch
	case dowRestricted:
		return dowMatch
	default:
		return true
	}
}

// Validate checks whether the parsed spec is usable.
func (s *CronSpec) Validate() error {
	if s == nil {
		return fmt.Errorf("cron spec is required")
	}
	if s.Every > 0 {
		return nil
	}
	if len(s.Minute.Values) == 0 || len(s.Hour.Values) == 0 || len(s.DayOfMonth.Values) == 0 || len(s.Month.Values) == 0 || len(s.DayOfWeek.Values) == 0 {
		return fmt.Errorf("cron spec has empty fields")
	}
	return nil
}

func (f CronField) isWildcard() bool {
	return len(f.Values) == 1 && f.Values[0] == -1
}

func (f CronField) matches(v int) bool {
	if f.isWildcard() {
		return true
	}
	for _, value := range f.Values {
		if value == v {
			return true
		}
	}
	return false
}

func (f CronField) nextValue(v int) int {
	if f.isWildcard() {
		if v < f.Max {
			return v + 1
		}
		return -1
	}
	for _, value := range f.Values {
		if value > v {
			return value
		}
	}
	return -1
}

func parseField(field string, min int, max int) (CronField, error) {
	values := make(map[int]struct{})
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parsed, err := parseFieldPart(part, min, max)
		if err != nil {
			return CronField{}, err
		}
		for _, value := range parsed {
			values[value] = struct{}{}
		}
	}
	if len(values) == 0 {
		return CronField{}, fmt.Errorf("no valid values in field")
	}

	sorted := make([]int, 0, len(values))
	for value := range values {
		sorted = append(sorted, value)
	}
	sort.Ints(sorted)

	return CronField{
		Values: sorted,
		Min:    min,
		Max:    max,
	}, nil
}

func parseFieldPart(part string, min int, max int) ([]int, error) {
	step := 1
	if idx := strings.Index(part, "/"); idx != -1 {
		stepValue, err := strconv.Atoi(part[idx+1:])
		if err != nil {
			return nil, fmt.Errorf("invalid step value %q: %w", part[idx+1:], err)
		}
		if stepValue < 1 {
			return nil, fmt.Errorf("step value must be >= 1")
		}
		step = stepValue
		part = part[:idx]
	}

	switch {
	case part == "*":
		values := make([]int, 0, ((max-min)/step)+1)
		for value := min; value <= max; value += step {
			values = append(values, value)
		}
		if step == 1 {
			return []int{-1}, nil
		}
		return values, nil
	case strings.Contains(part, "-"):
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
		values := make([]int, 0, ((end-start)/step)+1)
		for value := start; value <= end; value += step {
			values = append(values, value)
		}
		return values, nil
	default:
		if step != 1 {
			return nil, fmt.Errorf("step not allowed on single value")
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid value %q: %w", part, err)
		}
		if value < min || value > max {
			return nil, fmt.Errorf("value %d out of range [%d-%d]", value, min, max)
		}
		return []int{value}, nil
	}
}

// Format returns a normalized human-readable schedule.
func (s *CronSpec) Format() string {
	if s == nil {
		return ""
	}
	if s.Original != "" {
		return s.Original
	}
	if s.Every > 0 {
		return "@every " + s.Every.String()
	}

	return strings.Join([]string{
		formatField(s.Minute),
		formatField(s.Hour),
		formatField(s.DayOfMonth),
		formatField(s.Month),
		formatField(s.DayOfWeek),
	}, " ")
}

func formatField(field CronField) string {
	if field.isWildcard() {
		return "*"
	}
	parts := make([]string, len(field.Values))
	for i, value := range field.Values {
		parts[i] = strconv.Itoa(value)
	}
	return strings.Join(parts, ",")
}

// CronSpecForTask parses a task schedule.
func CronSpecForTask(schedule string) (*CronSpec, error) {
	return ParseCronSpec(schedule)
}

// NextRunTimes returns the next N execution timestamps after from.
func NextRunTimes(schedule string, from time.Time, count int) ([]time.Time, error) {
	spec, err := ParseCronSpec(schedule)
	if err != nil {
		return nil, err
	}
	if count <= 0 {
		return []time.Time{}, nil
	}

	result := make([]time.Time, 0, count)
	current := from
	for len(result) < count {
		next := spec.Next(current)
		if next.IsZero() {
			break
		}
		result = append(result, next)
		current = next
	}
	return result, nil
}

// ValidateCronExpression validates a schedule and returns a short description.
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
	if spec.Every > 0 {
		return fmt.Sprintf("Every %s", spec.Every)
	}
	if spec.isEveryMinute() {
		return "Every minute"
	}
	if spec.isEveryHour() {
		return fmt.Sprintf("Every hour at minute %02d", spec.Minute.Values[0])
	}
	if spec.isDaily() {
		return fmt.Sprintf("Every day at %02d:%02d", spec.Hour.Values[0], spec.Minute.Values[0])
	}
	if spec.isWeekly() {
		days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
		return fmt.Sprintf("Every %s at %02d:%02d", days[spec.DayOfWeek.Values[0]], spec.Hour.Values[0], spec.Minute.Values[0])
	}
	return "Custom schedule: " + spec.Format()
}

func (s *CronSpec) isEveryMinute() bool {
	return s.Every == 0 && s.Minute.isWildcard() && s.Hour.isWildcard() && s.DayOfMonth.isWildcard() && s.Month.isWildcard() && s.DayOfWeek.isWildcard()
}

func (s *CronSpec) isEveryHour() bool {
	return s.Every == 0 && len(s.Minute.Values) == 1 && s.Hour.isWildcard() && s.DayOfMonth.isWildcard() && s.Month.isWildcard() && s.DayOfWeek.isWildcard()
}

func (s *CronSpec) isDaily() bool {
	return s.Every == 0 && len(s.Minute.Values) == 1 && len(s.Hour.Values) == 1 && s.DayOfMonth.isWildcard() && s.Month.isWildcard() && s.DayOfWeek.isWildcard()
}

func (s *CronSpec) isWeekly() bool {
	return s.Every == 0 && len(s.Minute.Values) == 1 && len(s.Hour.Values) == 1 && s.DayOfMonth.isWildcard() && s.Month.isWildcard() && len(s.DayOfWeek.Values) == 1 && !s.DayOfWeek.isWildcard()
}
