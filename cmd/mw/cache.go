package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/plainlystated/my-week/internal/paths"
	"github.com/plainlystated/my-week/internal/snapshot"
)

// loadCurrentCache loads this week's cache file. Returns (nil, nil) if absent.
func loadCurrentCache(profile string, now time.Time) (*snapshot.Snapshot, string, error) {
	isoYear, isoWeek := now.ISOWeek()
	iso := fmt.Sprintf("%d-W%02d", isoYear, isoWeek)
	p, err := paths.CachePath(profile, iso)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, p, nil
	}
	if err != nil {
		return nil, p, err
	}
	snap, err := snapshot.Parse(string(data))
	return snap, p, err
}

// flipCacheLine loads the cache, flips a single ID's checkbox, and writes back.
// Silently no-ops if the cache or the ID is missing.
func flipCacheLine(profile, id string, done bool, now time.Time) error {
	snap, path, err := loadCurrentCache(profile, now)
	if err != nil {
		return err
	}
	if snap == nil {
		return nil
	}
	snap.FlipCheckboxes(map[string]bool{id: done})
	snap.FrontMatter.RefreshedAt = now
	return os.WriteFile(path, []byte(snap.Render()), 0o644)
}

// removeCacheLine drops every line referencing `id`. Used by promote so the
// task disappears from its (now-stale) inbox section immediately.
func removeCacheLine(profile, id string, now time.Time) error {
	snap, path, err := loadCurrentCache(profile, now)
	if err != nil {
		return err
	}
	if snap == nil {
		return nil
	}
	snap.RemoveID(id)
	snap.FrontMatter.RefreshedAt = now
	return os.WriteFile(path, []byte(snap.Render()), 0o644)
}
