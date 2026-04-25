// Package recur computes the next due date for a recurring task.
//
// All arithmetic is calendar-date based with end-of-month clamping, so:
//   - Jan 31 + 1 month       → Feb 28 (or Feb 29 in leap years)
//   - Feb 29 + 1 year        → Feb 28 in non-leap years; recovers to Feb 29 next leap year
//   - Mar 31 + 1 month       → Apr 30
//   - Dec 31 + 1 quarter (3mo) → Mar 31
package recur

import (
	"fmt"
	"time"
)

// Interval is a recur custom-field value.
type Interval string

const (
	Daily      Interval = "daily"
	Weekly     Interval = "weekly"
	Monthly    Interval = "monthly"
	Quarterly  Interval = "quarterly"
	SemiAnnual Interval = "semi-annual"
	Annual     Interval = "annual"
)

// Parse normalizes a string into a known Interval. Returns an error if the
// value is not one of the recognized recur values.
func Parse(s string) (Interval, error) {
	switch Interval(s) {
	case Daily, Weekly, Monthly, Quarterly, SemiAnnual, Annual:
		return Interval(s), nil
	}
	return "", fmt.Errorf("unknown recur interval %q", s)
}

// Next returns the successor due date for `due` given the recurrence interval.
func Next(due time.Time, in Interval) (time.Time, error) {
	switch in {
	case Daily:
		return due.AddDate(0, 0, 1), nil
	case Weekly:
		return due.AddDate(0, 0, 7), nil
	case Monthly:
		return addMonthsClamped(due, 1), nil
	case Quarterly:
		return addMonthsClamped(due, 3), nil
	case SemiAnnual:
		return addMonthsClamped(due, 6), nil
	case Annual:
		return addMonthsClamped(due, 12), nil
	}
	return time.Time{}, fmt.Errorf("unknown recur interval %q", in)
}

// addMonthsClamped adds n calendar months, clamping the day to the last day of
// the target month if the original day doesn't exist there. We can't use
// time.AddDate directly because it normalizes overflow forward (Jan 31 +
// 1 month → Mar 3) rather than clamping (we want Feb 28/29).
func addMonthsClamped(t time.Time, n int) time.Time {
	year, month, day := t.Date()
	hour, min, sec := t.Clock()
	loc := t.Location()

	// Move to the first of the target month, then clamp the day.
	target := time.Date(year, month+time.Month(n), 1, hour, min, sec, t.Nanosecond(), loc)
	tYear, tMonth, _ := target.Date()
	last := lastDayOfMonth(tYear, tMonth, loc)
	if day > last {
		day = last
	}
	return time.Date(tYear, tMonth, day, hour, min, sec, t.Nanosecond(), loc)
}

func lastDayOfMonth(year int, month time.Month, loc *time.Location) int {
	// Day 0 of the next month equals the last day of `month`.
	t := time.Date(year, month+1, 0, 0, 0, 0, 0, loc)
	return t.Day()
}
