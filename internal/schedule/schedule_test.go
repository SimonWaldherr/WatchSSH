package schedule

import (
	"testing"
	"time"
)

func TestParseAndMatches(t *testing.T) {
	tests := []struct {
		expression string
		at         time.Time
		want       bool
	}{
		{"15 3 * * 1", time.Date(2026, time.July, 20, 3, 15, 0, 0, time.UTC), true},
		{"15 3 * * 1", time.Date(2026, time.July, 21, 3, 15, 0, 0, time.UTC), false},
		{"*/10 8-18 * * mon-fri", time.Date(2026, time.July, 20, 8, 20, 0, 0, time.UTC), true},
		{"*/10 8-18 * * mon-fri", time.Date(2026, time.July, 19, 8, 20, 0, 0, time.UTC), false},
		{"@daily", time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC), true},
		{"@daily", time.Date(2026, time.July, 20, 0, 1, 0, 0, time.UTC), false},
		{"0 0 13 * fri", time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC), true},
		{"0 0 13 * fri", time.Date(2026, time.July, 13, 0, 0, 0, 0, time.UTC), true},
		{"0 0 13 * fri", time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC), false},
	}
	for _, test := range tests {
		schedule, err := Parse(test.expression)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", test.expression, err)
		}
		if got := schedule.Matches(test.at); got != test.want {
			t.Errorf("%q Matches(%s) = %t, want %t", test.expression, test.at, got, test.want)
		}
	}
}

func TestParseRejectsInvalidSchedules(t *testing.T) {
	for _, expression := range []string{"", "* * * *", "61 * * * *", "*/0 * * * *", "0 0 * foo *", "0 0 * * 8"} {
		if _, err := Parse(expression); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", expression)
		}
	}
}
