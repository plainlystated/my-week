package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/plainlystated/my-week/internal/config"
	"github.com/plainlystated/my-week/internal/paths"
	"github.com/plainlystated/my-week/internal/snapshot"
)

// cmdRead implements the default `mw` command: print the cache, prefixed by a
// freshness banner. Auto-refreshes if stale or missing.
func cmdRead(profile string) error {
	cfg, err := config.Load(profile)
	if err != nil {
		return err
	}

	now := time.Now()
	isoYear, isoWeek := now.ISOWeek()
	currentISO := fmt.Sprintf("%d-W%02d", isoYear, isoWeek)
	cachePath, err := paths.CachePath(cfg.Profile, currentISO)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(cachePath)
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Println("Building cache, one moment...")
		if err := runRefresh(cfg); err != nil {
			return err
		}
		data, err = os.ReadFile(cachePath)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	snap, err := snapshot.Parse(string(data))
	if err != nil {
		return err
	}

	if isStale(snap.FrontMatter.RefreshedAt, cfg, now) {
		fmt.Println("Cache stale, refreshing...")
		if err := runRefresh(cfg); err != nil {
			return err
		}
		data, err = os.ReadFile(cachePath)
		if err != nil {
			return err
		}
		snap, err = snapshot.Parse(string(data))
		if err != nil {
			return err
		}
	}

	printBanner(snap.FrontMatter.RefreshedAt, now)
	fmt.Print(snap.Body)
	return nil
}

func isStale(refreshedAt time.Time, cfg *config.Config, now time.Time) bool {
	if refreshedAt.IsZero() {
		return true
	}
	threshold := time.Duration(cfg.StalenessThresholdMinutes) * time.Minute
	return now.Sub(refreshedAt) > threshold
}

func printBanner(refreshedAt, now time.Time) {
	if refreshedAt.IsZero() {
		fmt.Println("Refreshed: never.")
		return
	}
	d := now.Sub(refreshedAt)
	fmt.Printf("Refreshed %s ago.\n", humanizeDuration(d))
}

func humanizeDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
