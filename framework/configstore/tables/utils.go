package tables

import (
	"fmt"
	"strconv"
	"time"
)

// IsCalendarAlignableDuration reports whether the given duration string supports calendar-aligned resets.
// Only day ("d"), week ("w"), month ("M"), and year ("Y") suffixes have natural calendar boundaries.
// Sub-day durations like "1h", "30m" are not alignable.
func IsCalendarAlignableDuration(duration string) bool {
	if duration == "" {
		return false
	}
	switch duration[len(duration)-1] {
	case 'd', 'w', 'M', 'Y':
		return true
	default:
		return false
	}
}

// GetCalendarPeriodStart returns the start of the current calendar period for the given duration and time.
// For calendar-scale durations (daily, weekly, monthly, yearly) it snaps to clean boundaries in UTC.
// N > 1 groups N units into one period anchored to a fixed epoch, so a period only starts every N units:
//   - "Nd"  → midnight UTC on the first day of the current N-day block (counted from 1970-01-01)
//   - "Nw"  → midnight UTC on the Monday starting the current N-week block (counted from 1970-01-05)
//   - "NM"  → midnight UTC on the 1st of the month starting the current N-month block ("3M" = quarters)
//   - "NY"  → midnight UTC on Jan 1 of the year starting the current N-year block
//
// For all other durations (e.g. "1h", "30m") the original time t is returned unchanged,
// since sub-day periods don't have a natural calendar boundary.
func GetCalendarPeriodStart(duration string, t time.Time) time.Time {
	if duration == "" {
		return t
	}
	t = t.UTC()
	n := 1
	if v, err := strconv.Atoi(duration[:len(duration)-1]); err == nil && v > 1 {
		n = v
	}
	suffix := duration[len(duration)-1:]
	switch suffix {
	case "d":
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		days := int(day.Sub(time.Unix(0, 0).UTC()).Hours() / 24)
		return day.AddDate(0, 0, -floorMod(days, n))
	case "w":
		weekday := int(t.Weekday())
		// Sunday = 0, so shift to Monday = 0
		daysFromMonday := (weekday + 6) % 7
		monday := t.AddDate(0, 0, -daysFromMonday)
		monday = time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
		weeks := int(monday.Sub(time.Date(1970, time.January, 5, 0, 0, 0, 0, time.UTC)).Hours() / (24 * 7))
		return monday.AddDate(0, 0, -7*floorMod(weeks, n))
	case "M":
		months := t.Year()*12 + int(t.Month()) - 1
		months -= floorMod(months, n)
		return time.Date(months/12, time.Month(months%12+1), 1, 0, 0, 0, 0, time.UTC)
	case "Y":
		year := t.Year() - floorMod(t.Year(), n)
		return time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	default:
		return t
	}
}

func floorMod(a, n int) int {
	m := a % n
	if m < 0 {
		m += n
	}
	return m
}

// ParseDuration function to parse duration strings
func ParseDuration(duration string) (time.Duration, error) {
	if duration == "" {
		return 0, fmt.Errorf("duration is empty")
	}

	// Handle special cases for days, weeks, months, years
	switch {
	case duration[len(duration)-1:] == "d":
		days := duration[:len(duration)-1]
		if d, err := time.ParseDuration(days + "h"); err == nil {
			return d * 24, nil
		}
		return 0, fmt.Errorf("invalid day duration: %s", duration)
	case duration[len(duration)-1:] == "w":
		weeks := duration[:len(duration)-1]
		if w, err := time.ParseDuration(weeks + "h"); err == nil {
			return w * 24 * 7, nil
		}
		return 0, fmt.Errorf("invalid week duration: %s", duration)
	case duration[len(duration)-1:] == "M":
		months := duration[:len(duration)-1]
		if m, err := time.ParseDuration(months + "h"); err == nil {
			return m * 24 * 30, nil // Approximate month as 30 days
		}
		return 0, fmt.Errorf("invalid month duration: %s", duration)
	case duration[len(duration)-1:] == "Y":
		years := duration[:len(duration)-1]
		if y, err := time.ParseDuration(years + "h"); err == nil {
			return y * 24 * 365, nil // Approximate year as 365 days
		}
		return 0, fmt.Errorf("invalid year duration: %s", duration)
	default:
		return time.ParseDuration(duration)
	}
}
