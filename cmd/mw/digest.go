package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/plainlystated/my-week/internal/config"
	"github.com/plainlystated/my-week/internal/digest"
	"github.com/plainlystated/my-week/internal/meta"
)

// cmdDigest renders the digest markdown to stdout.
func cmdDigest(profile string) error {
	cfg, err := config.Load(profile)
	if err != nil {
		return err
	}
	snap, _, err := loadCurrentCache(cfg.Profile, time.Now())
	if err != nil {
		return err
	}
	if snap == nil {
		return errors.New("no cache file — run `mw refresh` first")
	}
	fmt.Print(digest.Render(snap))
	return nil
}

// cmdSendDigest renders the digest and POSTs to Resend. Updates meta state.
func cmdSendDigest(profile string) error {
	cfg, err := config.Load(profile)
	if err != nil {
		return err
	}
	now := time.Now()
	snap, _, err := loadCurrentCache(cfg.Profile, now)
	if err != nil {
		return err
	}
	if snap == nil {
		return errors.New("no cache file — run `mw refresh` first")
	}
	monday := mondayOf(now)
	subject := digest.Subject(cfg, monday)
	if err := digest.Send(cfg, subject, digest.Render(snap)); err != nil {
		return err
	}
	m, err := meta.Load(cfg.Profile)
	if err != nil {
		return err
	}
	m.LastDigestSent = isoWeekKey(now)
	if err := meta.Save(cfg.Profile, m); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "Digest sent to %s.\n", cfg.Email.To)
	return nil
}

// isoWeekKey returns the ISO-week label like "2026-W18".
func isoWeekKey(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}

// mondayOf returns the Monday at 00:00 in t's location for t's ISO week.
func mondayOf(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	delta := wd - 1
	d := t.AddDate(0, 0, -delta)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, t.Location())
}
