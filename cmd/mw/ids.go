package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/plainlystated/my-week/internal/config"
)

// resolveID accepts a partial ClickUp ID and resolves it against the IDs
// already present in the current week's cache, matching by suffix. The
// terminal characters of a ClickUp ID vary while the prefix is the timestamp
// portion, so suffix matching is the natural shorthand.
//
// Takes *config.Config (not the -p flag value) because the cache file is
// keyed by cfg.Profile, which may differ from the config filename.
//
// Resolution rules:
//   - exactly one cache ID has `partial` as a suffix → use that ID
//   - no cache match, but `partial` looks like a full ID → pass through (may
//     be a task created since last refresh)
//   - no cache match and `partial` is short → error with a hint
//   - 2+ cache IDs match → error listing the candidates
func resolveID(cfg *config.Config, partial string) (string, error) {
	partial = strings.TrimSpace(partial)
	if partial == "" {
		return "", fmt.Errorf("empty task ID")
	}
	snap, _, err := loadCurrentCache(cfg.Profile, time.Now())
	if err != nil {
		return "", err
	}
	var ids []string
	if snap != nil {
		ids = snap.IDs()
	}
	return resolveIDFromIDs(ids, partial)
}

// resolveIDFromIDs is the pure decision logic, factored for testing.
func resolveIDFromIDs(ids []string, partial string) (string, error) {
	var matches []string
	for _, id := range ids {
		if strings.HasSuffix(id, partial) {
			matches = append(matches, id)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		if looksLikeFullID(partial) {
			return partial, nil
		}
		return "", fmt.Errorf("no task in cache ends with %q (run `mw refresh` if it's new, or pass the full ID)", partial)
	default:
		return "", fmt.Errorf("ambiguous suffix %q — matches: %s", partial, strings.Join(matches, ", "))
	}
}

// looksLikeFullID reports whether s is plausibly a complete ClickUp ID.
// ClickUp IDs are lowercase alphanumeric, typically 9 characters; we accept
// 8+ to be lenient.
func looksLikeFullID(s string) bool {
	if len(s) < 8 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z')) {
			return false
		}
	}
	return true
}
