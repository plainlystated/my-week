// Package refresh dispatches between the three refresh paths (FRESH_BUILD,
// REFRESH_WITH_SWEEPS, REFRESH_ONLY) based on cache state, and implements the
// REFRESH_ONLY path inline.
//
// FRESH_BUILD and REFRESH_WITH_SWEEPS are delegated to their own packages.
package refresh

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/plainlystated/my-week/internal/build"
	"github.com/plainlystated/my-week/internal/config"
	"github.com/plainlystated/my-week/internal/cup"
	"github.com/plainlystated/my-week/internal/digest"
	"github.com/plainlystated/my-week/internal/meta"
	"github.com/plainlystated/my-week/internal/paths"
	"github.com/plainlystated/my-week/internal/snapshot"
	"github.com/plainlystated/my-week/internal/sweep"
)

// Path identifies which path Run took. Useful for the digest gate decision.
type Path int

const (
	PathUnknown Path = iota
	PathFreshBuild
	PathRefreshWithSweeps
	PathRefreshOnly
)

func (p Path) String() string {
	switch p {
	case PathFreshBuild:
		return "FRESH_BUILD"
	case PathRefreshWithSweeps:
		return "REFRESH_WITH_SWEEPS"
	case PathRefreshOnly:
		return "REFRESH_ONLY"
	}
	return "UNKNOWN"
}

// Result reports what happened during Run.
type Result struct {
	Path               Path
	CachePath          string
	NewItems           int
	StatusFlips        int
	InfoCleared        int
	RecurrencesCreated int
	SweepErrors        []string
	DigestSent         bool
	DigestError        error // non-nil if the digest gate opened but the send failed
}

// Run loads the cache (if present) and dispatches to the appropriate path.
// Now is injected for testing; pass time.Now() in production.
func Run(cfg *config.Config, now time.Time) (*Result, error) {
	isoYear, isoWeek := now.ISOWeek()
	currentISO := fmt.Sprintf("%d-W%02d", isoYear, isoWeek)
	cachePath, err := paths.CachePath(cfg.Profile, currentISO)
	if err != nil {
		return nil, err
	}

	existing, err := loadSnapshot(cachePath)
	if err != nil {
		return nil, err
	}

	path := decidePath(existing, currentISO, now)
	if err := paths.EnsureStateDir(); err != nil {
		return nil, err
	}

	var (
		res *Result
		runErr error
	)
	switch path {
	case PathFreshBuild:
		res, runErr = runFreshBuild(cfg, cachePath, currentISO, now)
	case PathRefreshWithSweeps:
		res, runErr = runRefreshWithSweeps(cfg, existing, cachePath, now)
	case PathRefreshOnly:
		res, runErr = runRefreshOnly(cfg, existing, cachePath, now)
	default:
		return nil, fmt.Errorf("could not determine refresh path")
	}
	if runErr != nil {
		return nil, runErr
	}

	// Digest gate runs after a successful FRESH_BUILD.
	if path == PathFreshBuild {
		sent, sendErr := maybeSendDigest(cfg, cachePath, now, currentISO)
		res.DigestSent = sent
		res.DigestError = sendErr
	}
	return res, nil
}

// maybeSendDigest evaluates the digest gate and, if open, sends.
//
// Gate (per spec):
//   - today.weekday >= config.digest.send_on
//   - today.time    >= config.digest.send_after
//   - meta.last_digest_sent != current_iso_week
func maybeSendDigest(cfg *config.Config, cachePath string, now time.Time, currentISO string) (bool, error) {
	if !weekdayReached(cfg.Digest.SendOn, now) {
		return false, nil
	}
	if !timeReached(cfg.Digest.SendAfter, now) {
		return false, nil
	}
	m, err := meta.Load(cfg.Profile)
	if err != nil {
		return false, err
	}
	if m.LastDigestSent == currentISO {
		return false, nil
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return false, fmt.Errorf("re-read cache for digest: %w", err)
	}
	snap, err := snapshot.Parse(string(data))
	if err != nil {
		return false, err
	}
	subject := digest.Subject(cfg, mondayOf(now))
	if err := digest.Send(cfg, subject, digest.Render(snap)); err != nil {
		return false, err
	}
	m.LastDigestSent = currentISO
	if err := meta.Save(cfg.Profile, m); err != nil {
		return true, fmt.Errorf("digest sent but meta.Save failed: %w", err)
	}
	return true, nil
}

func weekdayReached(configured string, now time.Time) bool {
	cfg, ok := dayOfWeek(configured)
	if !ok {
		return false
	}
	return dayOfWeekISO(now) >= cfg
}

func dayOfWeek(s string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "monday":
		return 1, true
	case "tuesday":
		return 2, true
	case "wednesday":
		return 3, true
	case "thursday":
		return 4, true
	case "friday":
		return 5, true
	case "saturday":
		return 6, true
	case "sunday":
		return 7, true
	}
	return 0, false
}

func dayOfWeekISO(t time.Time) int {
	wd := int(t.Weekday())
	if wd == 0 {
		return 7
	}
	return wd
}

func timeReached(hm string, now time.Time) bool {
	var h, m int
	if _, err := fmt.Sscanf(hm, "%d:%d", &h, &m); err != nil {
		return false
	}
	return now.Hour() > h || (now.Hour() == h && now.Minute() >= m)
}

func mondayOf(t time.Time) time.Time {
	delta := dayOfWeekISO(t) - 1
	d := t.AddDate(0, 0, -delta)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, t.Location())
}

// runFreshBuild builds a brand-new cache file for `currentISO`.
func runFreshBuild(cfg *config.Config, cachePath, currentISO string, now time.Time) (*Result, error) {
	client := cup.New(cfg.CupProfile)

	prevSnap, err := loadPrevSnapshot(cfg.Profile, now)
	if err != nil {
		return nil, fmt.Errorf("loading previous week's cache: %w", err)
	}

	res, err := build.FreshBuild(client, cfg, now, prevSnap)
	if err != nil {
		return nil, err
	}

	snap := &snapshot.Snapshot{
		FrontMatter: snapshot.FrontMatter{
			Profile:     cfg.Profile,
			ISOWeek:     currentISO,
			GeneratedAt: now,
			RefreshedAt: now,
			SweptOn:     now.Format("2006-01-02"),
		},
		Body: res.Body,
	}
	if err := os.WriteFile(cachePath, []byte(snap.Render()), 0o644); err != nil {
		return nil, err
	}

	return &Result{
		Path:               PathFreshBuild,
		CachePath:          cachePath,
		InfoCleared:        res.Sweep.InfoCleared,
		RecurrencesCreated: res.Sweep.RecurrencesCreated,
		SweepErrors:        res.Sweep.Errors,
	}, nil
}

// runRefreshWithSweeps runs daily sweeps + REFRESH_ONLY logic on the existing
// cache. Preserves recap, rotation, and section structure.
func runRefreshWithSweeps(cfg *config.Config, snap *snapshot.Snapshot, cachePath string, now time.Time) (*Result, error) {
	if snap == nil {
		return nil, errors.New("REFRESH_WITH_SWEEPS requires existing cache")
	}
	client := cup.New(cfg.CupProfile)
	sw := sweep.Run(client, cfg, now)

	res, err := refreshExistingSnapshot(client, cfg, snap, cachePath, now, true)
	if err != nil {
		return nil, err
	}
	res.Path = PathRefreshWithSweeps
	res.InfoCleared = sw.InfoCleared
	res.RecurrencesCreated = sw.RecurrencesCreated
	res.SweepErrors = sw.Errors
	return res, nil
}

func loadPrevSnapshot(profile string, now time.Time) (*snapshot.Snapshot, error) {
	prev := now.AddDate(0, 0, -7)
	prevYear, prevWeek := prev.ISOWeek()
	prevISO := fmt.Sprintf("%d-W%02d", prevYear, prevWeek)
	p, err := paths.CachePath(profile, prevISO)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return snapshot.Parse(string(data))
}

// decidePath compares the cache's frontmatter to today's date.
func decidePath(snap *snapshot.Snapshot, currentISO string, now time.Time) Path {
	if snap == nil {
		return PathFreshBuild
	}
	if snap.FrontMatter.ISOWeek != currentISO {
		return PathFreshBuild
	}
	if snap.FrontMatter.SweptOn != now.Format("2006-01-02") {
		return PathRefreshWithSweeps
	}
	return PathRefreshOnly
}

func loadSnapshot(path string) (*snapshot.Snapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return snapshot.Parse(string(data))
}

// runRefreshOnly: single bulk fetch, refresh checkboxes, detect new items, write.
// Equivalent to legacy scripts/rerun_refresh.py but in Go.
func runRefreshOnly(cfg *config.Config, snap *snapshot.Snapshot, cachePath string, now time.Time) (*Result, error) {
	client := cup.New(cfg.CupProfile)
	res, err := refreshExistingSnapshot(client, cfg, snap, cachePath, now, false)
	if err != nil {
		return nil, err
	}
	res.Path = PathRefreshOnly
	return res, nil
}

// refreshExistingSnapshot is the shared body for REFRESH_ONLY and the
// post-sweep portion of REFRESH_WITH_SWEEPS: bulk-fetch, flip checkboxes,
// detect new items, write back. Updates SweptOn iff updateSweptOn is true.
func refreshExistingSnapshot(client *cup.Client, cfg *config.Config, snap *snapshot.Snapshot, cachePath string, now time.Time, updateSweptOn bool) (*Result, error) {
	bulk, err := client.Tasks(cup.TaskQuery{
		All:           true,
		IncludeClosed: true,
		SpaceID:       cfg.ClickUp.SpaceID,
	})
	if err != nil {
		return nil, fmt.Errorf("bulk fetch: %w", err)
	}

	statusByID := make(map[string]bool, len(bulk))
	for _, t := range bulk {
		statusByID[t.ID] = t.IsDone()
	}

	// Count flips before mutating, so we report only actual changes.
	flips := 0
	for _, line := range strings.Split(snap.Body, "\n") {
		m := snapshot.TaskLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		want, ok := statusByID[m[4]]
		if !ok {
			continue
		}
		was := m[2] == "x"
		if was != want {
			flips++
		}
	}
	snap.FlipCheckboxes(statusByID)

	newItems, err := detectNewItems(client, cfg, snap, now)
	if err != nil {
		return nil, fmt.Errorf("new items: %w", err)
	}
	snap.AppendNewItems(newItems)

	if err := refreshInboxSection(client, cfg, snap); err != nil {
		return nil, fmt.Errorf("inbox refresh: %w", err)
	}

	snap.FrontMatter.RefreshedAt = now
	if updateSweptOn {
		snap.FrontMatter.SweptOn = now.Format("2006-01-02")
	}
	if err := os.WriteFile(cachePath, []byte(snap.Render()), 0o644); err != nil {
		return nil, err
	}

	return &Result{
		CachePath:   cachePath,
		NewItems:    len(newItems),
		StatusFlips: flips,
	}, nil
}

// refreshInboxSection re-queries the inbox list and rewrites the Inbox
// section in-place. The bulk fetch covers the rest of the space but the
// inbox is its own list and its membership turns over independently of the
// other sections — new email-forwarded items should appear within an hour,
// not wait for the next FRESH_BUILD.
func refreshInboxSection(client *cup.Client, cfg *config.Config, snap *snapshot.Snapshot) error {
	if cfg.ClickUp.Lists.Inbox == "" {
		return nil
	}
	items, err := client.Tasks(cup.TaskQuery{
		All:    true,
		ListID: cfg.ClickUp.Lists.Inbox,
	})
	if err != nil {
		return err
	}
	open := make([]cup.Task, 0, len(items))
	for _, t := range items {
		if t.IsDone() {
			continue
		}
		open = append(open, t)
	}
	snap.SetSection("Inbox", build.RenderInboxBlock(open), "Overdue")
	return nil
}

func detectNewItems(client *cup.Client, cfg *config.Config, snap *snapshot.Snapshot, now time.Time) ([]string, error) {
	since := snap.FrontMatter.GeneratedAt
	if since.IsZero() {
		since = now.AddDate(0, 0, -7)
	}

	created, err := client.Tasks(cup.TaskQuery{
		All:          true,
		SpaceID:      cfg.ClickUp.SpaceID,
		CreatedAfter: since,
	})
	if err != nil {
		return nil, err
	}

	known := make(map[string]bool)
	for _, id := range snap.IDs() {
		known[id] = true
	}

	weekEnd := now.AddDate(0, 0, cfg.LookaheadDays)
	weekEndCutoff := time.Date(weekEnd.Year(), weekEnd.Month(), weekEnd.Day(), 23, 59, 59, 0, weekEnd.Location())

	var lines []string
	for _, t := range created {
		if known[t.ID] {
			continue
		}
		if t.IsDone() {
			continue
		}
		due := t.DueTime()
		if due.IsZero() {
			continue
		}
		if due.After(weekEndCutoff) {
			continue
		}
		lines = append(lines, formatNewItem(t, now))
	}
	return lines, nil
}

func formatNewItem(t cup.Task, now time.Time) string {
	nudge := ""
	due := t.DueTime()
	if !due.IsZero() {
		todayDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		dueDate := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, due.Location())
		if dueDate.Before(todayDate) {
			n := int(todayDate.Sub(dueDate).Hours() / 24)
			plural := "s"
			if n == 1 {
				plural = ""
			}
			nudge = fmt.Sprintf("%d day%s late", n, plural)
		} else if dueDate.Equal(todayDate) {
			nudge = "due today"
		} else {
			nudge = fmt.Sprintf("due %s", due.Format("Mon"))
		}
	}
	suffix := ""
	if nudge != "" {
		suffix = " — " + nudge
	}
	return fmt.Sprintf("- [ ] %s — **%s**%s", t.ID, t.Name, suffix)
}
