package domain

import (
	"testing"
	"time"
)

func TestCalendar_CurrentStreak(t *testing.T) {
	day := func(s string) int64 {
		d, err := time.Parse(time.DateOnly, s)
		if err != nil {
			t.Fatal(err)
		}
		return d.Unix()
	}

	// Four days in a row across New Year, then a gap, then an older run.
	calendar := Calendar{Submissions: map[int64]int{
		day("2025-12-30"): 2,
		day("2025-12-31"): 1,
		day("2026-01-01"): 3,
		day("2026-01-02"): 1,
		day("2025-12-20"): 5,
		day("2025-12-21"): 1,
		day("2026-01-04"): 0,
	}}

	tests := []struct {
		name string
		now  string
		want int
	}{
		{"today has a submission", "2026-01-02T23:59:59Z", 4},
		{"none yet today, the run ended yesterday", "2026-01-03T00:00:00Z", 4},
		{"the last submission is older than yesterday", "2026-01-04T10:00:00Z", 0},
		{"a day with zero submissions does not count", "2026-01-05T10:00:00Z", 0},
		{"an older run is reached from its own last day", "2025-12-22T08:00:00Z", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tt.now)
			if err != nil {
				t.Fatal(err)
			}
			if got := calendar.CurrentStreak(now); got != tt.want {
				t.Fatalf("CurrentStreak(%s) = %d, want %d", tt.now, got, tt.want)
			}
		})
	}

	if got := (Calendar{}).CurrentStreak(time.Now()); got != 0 {
		t.Fatalf("empty calendar: streak %d, want 0", got)
	}
}
