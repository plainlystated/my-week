package recur

import (
	"testing"
	"time"
)

func ymd(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestNext(t *testing.T) {
	cases := []struct {
		name     string
		due      time.Time
		interval Interval
		want     time.Time
	}{
		{"daily", ymd(2026, time.April, 27), Daily, ymd(2026, time.April, 28)},
		{"daily across month boundary", ymd(2026, time.April, 30), Daily, ymd(2026, time.May, 1)},
		{"weekly", ymd(2026, time.April, 27), Weekly, ymd(2026, time.May, 4)},

		// Monthly clamping: Jan 31 + 1mo → Feb 28 (non-leap) and Feb 29 (leap).
		{"jan 31 + 1mo non-leap", ymd(2025, time.January, 31), Monthly, ymd(2025, time.February, 28)},
		{"jan 31 + 1mo leap", ymd(2024, time.January, 31), Monthly, ymd(2024, time.February, 29)},
		{"mar 31 + 1mo", ymd(2026, time.March, 31), Monthly, ymd(2026, time.April, 30)},
		{"may 31 + 1mo", ymd(2026, time.May, 31), Monthly, ymd(2026, time.June, 30)},

		// Quarterly: Dec 31 + 3mo → Mar 31 (no clamping needed).
		{"dec 31 + 1qtr", ymd(2025, time.December, 31), Quarterly, ymd(2026, time.March, 31)},
		// Quarterly that crosses a 30-day month: Nov 30 + 3mo → Feb 28 in non-leap.
		{"nov 30 + 1qtr non-leap", ymd(2026, time.November, 30), Quarterly, ymd(2027, time.February, 28)},
		{"nov 30 + 1qtr leap year target", ymd(2023, time.November, 30), Quarterly, ymd(2024, time.February, 29)},

		// Semi-annual: Aug 31 + 6mo → Feb 28/29.
		{"aug 31 + 6mo non-leap", ymd(2025, time.August, 31), SemiAnnual, ymd(2026, time.February, 28)},
		{"aug 31 + 6mo leap target", ymd(2023, time.August, 31), SemiAnnual, ymd(2024, time.February, 29)},

		// Annual: Feb 29 leap-year handling.
		{"feb 29 + 1yr non-leap", ymd(2024, time.February, 29), Annual, ymd(2025, time.February, 28)},
		// And recovery the next leap year — chain calls.
		{"feb 28 (from prior leap) + 3yr lands on feb 28", ymd(2025, time.February, 28), Annual, ymd(2026, time.February, 28)},
		// Vanilla annual bump.
		{"apr 27 + 1yr", ymd(2026, time.April, 27), Annual, ymd(2027, time.April, 27)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Next(c.due, c.interval)
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			if !got.Equal(c.want) {
				t.Errorf("Next(%s, %s) = %s, want %s",
					c.due.Format("2006-01-02"), c.interval,
					got.Format("2006-01-02"), c.want.Format("2006-01-02"))
			}
		})
	}
}

// TestFeb29Recovery confirms that drift-free behavior holds across multiple
// leap cycles when chaining: a Feb 29 task should return to Feb 29 each leap year.
func TestFeb29Recovery(t *testing.T) {
	due := ymd(2024, time.February, 29)
	for i, want := range []time.Time{
		ymd(2025, time.February, 28),
		ymd(2026, time.February, 28),
		ymd(2027, time.February, 28),
		ymd(2028, time.February, 28), // chained from prior, stays at 28
	} {
		next, err := Next(due, Annual)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if !next.Equal(want) {
			t.Errorf("step %d: got %s want %s", i, next.Format("2006-01-02"), want.Format("2006-01-02"))
		}
		due = next
	}
}

func TestParse(t *testing.T) {
	for _, s := range []string{"daily", "weekly", "monthly", "quarterly", "semi-annual", "annual"} {
		if _, err := Parse(s); err != nil {
			t.Errorf("Parse(%q): unexpected error %v", s, err)
		}
	}
	if _, err := Parse("biweekly"); err == nil {
		t.Errorf("Parse(\"biweekly\"): expected error")
	}
}
