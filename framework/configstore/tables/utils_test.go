package tables

import (
	"testing"
	"time"
)

func TestGetCalendarPeriodStartHonorsMultiplier(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }
	cases := []struct {
		duration string
		at       time.Time
		want     time.Time
	}{
		{"1d", time.Date(2026, 10, 3, 15, 4, 0, 0, time.UTC), d(2026, 10, 3)},
		{"1w", d(2026, 10, 3), d(2026, 9, 28)},
		{"1M", d(2026, 10, 15), d(2026, 10, 1)},
		{"1Y", d(2026, 10, 15), d(2026, 1, 1)},
		{"3M", d(2026, 11, 15), d(2026, 10, 1)},
		{"3M", d(2026, 8, 15), d(2026, 7, 1)},
		{"2Y", d(2027, 5, 1), d(2026, 1, 1)},
		{"1h", time.Date(2026, 10, 3, 15, 4, 0, 0, time.UTC), time.Date(2026, 10, 3, 15, 4, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		if got := GetCalendarPeriodStart(c.duration, c.at); !got.Equal(c.want) {
			t.Errorf("GetCalendarPeriodStart(%q, %s) = %s, want %s", c.duration, c.at, got, c.want)
		}
	}
}

func TestGetCalendarPeriodStartMultiDayBlocksAreStable(t *testing.T) {
	for _, duration := range []string{"7d", "2w"} {
		start := GetCalendarPeriodStart(duration, time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC))
		length, err := ParseDuration(duration)
		if err != nil {
			t.Fatal(err)
		}
		for ts := start; ts.Before(start.Add(length)); ts = ts.Add(6 * time.Hour) {
			if got := GetCalendarPeriodStart(duration, ts); !got.Equal(start) {
				t.Fatalf("%s: period start at %s = %s, want %s", duration, ts, got, start)
			}
		}
		if got := GetCalendarPeriodStart(duration, start.Add(length)); !got.Equal(start.Add(length)) {
			t.Fatalf("%s: next period should start at %s, got %s", duration, start.Add(length), got)
		}
	}
}
