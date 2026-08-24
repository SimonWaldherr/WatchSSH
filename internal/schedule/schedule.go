// Package schedule parses the compact cron subset used by WatchSSH jobs.
// It intentionally has no external dependency and accepts standard five-field
// expressions plus the familiar @daily-style descriptors.
package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule matches a local wall-clock minute.
type Schedule struct {
	minute      field
	hour        field
	dayOfMonth  field
	month       field
	dayOfWeek   field
	domWildcard bool
	dowWildcard bool
}

type field struct {
	values map[int]struct{}
}

// Parse accepts five cron fields: minute, hour, day of month, month, and day
// of week. Fields support wildcards, lists, ranges, steps, English month/day
// names, and the standard @hourly through @yearly descriptors.
func Parse(expression string) (Schedule, error) {
	expression = strings.TrimSpace(strings.ToLower(expression))
	if replacement, ok := descriptorExpressions[expression]; ok {
		expression = replacement
	}
	parts := strings.Fields(expression)
	if len(parts) != 5 {
		return Schedule{}, fmt.Errorf("schedule must contain five fields or a supported descriptor")
	}
	minute, err := parseField(parts[0], 0, 59, nil, false)
	if err != nil {
		return Schedule{}, fmt.Errorf("minute: %w", err)
	}
	hour, err := parseField(parts[1], 0, 23, nil, false)
	if err != nil {
		return Schedule{}, fmt.Errorf("hour: %w", err)
	}
	dayOfMonth, err := parseField(parts[2], 1, 31, nil, false)
	if err != nil {
		return Schedule{}, fmt.Errorf("day of month: %w", err)
	}
	month, err := parseField(parts[3], 1, 12, monthNames, false)
	if err != nil {
		return Schedule{}, fmt.Errorf("month: %w", err)
	}
	dayOfWeek, err := parseField(parts[4], 0, 7, weekdayNames, true)
	if err != nil {
		return Schedule{}, fmt.Errorf("day of week: %w", err)
	}
	return Schedule{
		minute:      minute,
		hour:        hour,
		dayOfMonth:  dayOfMonth,
		month:       month,
		dayOfWeek:   dayOfWeek,
		domWildcard: dayOfMonth.complete(1, 31),
		dowWildcard: dayOfWeek.complete(0, 6),
	}, nil
}

// Matches reports whether the local minute containing at matches the schedule.
// Day-of-month and day-of-week follow traditional cron semantics: when both
// fields are restricted, either field may match.
func (s Schedule) Matches(at time.Time) bool {
	if !s.minute.has(at.Minute()) || !s.hour.has(at.Hour()) || !s.month.has(int(at.Month())) {
		return false
	}
	domMatches, dowMatches := s.dayOfMonth.has(at.Day()), s.dayOfWeek.has(int(at.Weekday()))
	switch {
	case s.domWildcard && s.dowWildcard:
		return true
	case s.domWildcard:
		return dowMatches
	case s.dowWildcard:
		return domMatches
	default:
		return domMatches || dowMatches
	}
}

func (f field) has(value int) bool {
	_, ok := f.values[value]
	return ok
}

func (f field) complete(minimum, maximum int) bool {
	for value := minimum; value <= maximum; value++ {
		if !f.has(value) {
			return false
		}
	}
	return true
}

func parseField(input string, minimum, maximum int, names map[string]int, weekday bool) (field, error) {
	if input == "" {
		return field{}, fmt.Errorf("field is empty")
	}
	parsed := field{values: make(map[int]struct{})}
	for _, item := range strings.Split(input, ",") {
		if item == "" {
			return field{}, fmt.Errorf("empty list item")
		}
		base, step, err := parseStep(item)
		if err != nil {
			return field{}, err
		}
		start, end, err := parseRange(base, minimum, maximum, names)
		if err != nil {
			return field{}, err
		}
		for value := start; value <= end; value += step {
			if weekday && value == 7 {
				parsed.values[0] = struct{}{}
				continue
			}
			parsed.values[value] = struct{}{}
		}
	}
	return parsed, nil
}

func parseStep(item string) (string, int, error) {
	parts := strings.Split(item, "/")
	if len(parts) > 2 || parts[0] == "" {
		return "", 0, fmt.Errorf("invalid step %q", item)
	}
	if len(parts) == 1 {
		return parts[0], 1, nil
	}
	step, err := strconv.Atoi(parts[1])
	if err != nil || step <= 0 {
		return "", 0, fmt.Errorf("invalid step %q", parts[1])
	}
	return parts[0], step, nil
}

func parseRange(input string, minimum, maximum int, names map[string]int) (int, int, error) {
	if input == "*" {
		return minimum, maximum, nil
	}
	parts := strings.Split(input, "-")
	if len(parts) > 2 || parts[0] == "" {
		return 0, 0, fmt.Errorf("invalid range %q", input)
	}
	start, err := parseValue(parts[0], minimum, maximum, names)
	if err != nil {
		return 0, 0, err
	}
	if len(parts) == 1 {
		return start, start, nil
	}
	end, err := parseValue(parts[1], minimum, maximum, names)
	if err != nil {
		return 0, 0, err
	}
	if start > end {
		return 0, 0, fmt.Errorf("range %q is descending", input)
	}
	return start, end, nil
}

func parseValue(input string, minimum, maximum int, names map[string]int) (int, error) {
	if value, ok := names[input]; ok {
		return value, nil
	}
	value, err := strconv.Atoi(input)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("value %q is outside %d-%d", input, minimum, maximum)
	}
	return value, nil
}

var descriptorExpressions = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

var monthNames = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

var weekdayNames = map[string]int{
	"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
}
