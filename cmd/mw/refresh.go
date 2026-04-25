package main

import (
	"fmt"
	"time"

	"github.com/plainlystated/my-week/internal/config"
	"github.com/plainlystated/my-week/internal/refresh"
)

// cmdRefresh runs the refresh dispatcher.
func cmdRefresh(profile string) error {
	cfg, err := config.Load(profile)
	if err != nil {
		return err
	}
	return runRefresh(cfg)
}

// runRefresh is the shared entry point used by both `mw refresh` and the
// auto-refresh paths in `mw` (read).
func runRefresh(cfg *config.Config) error {
	res, err := refresh.Run(cfg, time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(stderr, "[%s] %d flips, %d new", res.Path, res.StatusFlips, res.NewItems)
	if res.InfoCleared > 0 || res.RecurrencesCreated > 0 {
		fmt.Fprintf(stderr, ", %d info-cleared, %d recurrences", res.InfoCleared, res.RecurrencesCreated)
	}
	if res.DigestSent {
		fmt.Fprint(stderr, ", digest sent")
	}
	fmt.Fprintf(stderr, " → %s\n", res.CachePath)
	for _, e := range res.SweepErrors {
		fmt.Fprintf(stderr, "  warn: %s\n", e)
	}
	if res.DigestError != nil {
		fmt.Fprintf(stderr, "  digest error: %v\n", res.DigestError)
	}
	return nil
}
