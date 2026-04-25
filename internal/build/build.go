// Package build runs the FRESH_BUILD path: sweeps, parallel bucket fetches,
// last-week recap, and render of the cache file body.
package build

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/plainlystated/my-week/internal/config"
	"github.com/plainlystated/my-week/internal/cup"
	"github.com/plainlystated/my-week/internal/snapshot"
	"github.com/plainlystated/my-week/internal/sweep"
)

// Result carries everything FreshBuild produces.
type Result struct {
	Body  string // markdown body for the cache file (excludes frontmatter)
	Sweep sweep.Result
}

// FreshBuild runs the full weekly build for `today`. prevSnap is last week's
// cache (may be nil); used only for the recap section.
func FreshBuild(client *cup.Client, cfg *config.Config, today time.Time, prevSnap *snapshot.Snapshot) (*Result, error) {
	sweepResult := sweep.Run(client, cfg, today)

	buckets, err := fetchBuckets(client, cfg, today)
	if err != nil {
		return nil, err
	}

	recap := buildRecap(client, cfg, prevSnap, buckets.bulkByID)

	body := render(cfg, today, sweepResult, buckets, recap)

	return &Result{Body: body, Sweep: sweepResult}, nil
}

// title returns "Personal — week of Mon Apr 27" style heading content.
func title(profile string, monday time.Time) string {
	return fmt.Sprintf("%s — week of %s", capitalize(profile), monday.Format("Mon Jan 2"))
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

// mondayOf returns the Monday at 00:00 in t's location for t's ISO week.
func mondayOf(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7 // Sunday → 7 in ISO
	}
	delta := wd - 1
	d := t.AddDate(0, 0, -delta)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, t.Location())
}

// daysBetween returns the integer number of calendar days from `from` to `to`.
// Negative if to is before from.
func daysBetween(from, to time.Time) int {
	a := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	b := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location())
	return int(b.Sub(a).Hours() / 24)
}

// pluralDays formats "1 day" / "N days".
func pluralDays(n int) string {
	if n == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", n)
}

// titleCasePriority maps "high"/"urgent" → "High"/"Urgent" for inline rendering.
func titleCasePriority(p string) string {
	switch strings.ToLower(p) {
	case "urgent":
		return "Urgent"
	case "high":
		return "High"
	case "normal":
		return "Normal"
	case "low":
		return "Low"
	}
	return ""
}
