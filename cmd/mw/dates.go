package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var ymdRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// parseDate accepts YYYY-MM-DD or a natural-language phrase understood by
// GNU `date -d` ("wednesday", "tomorrow", "next friday", "3 days", "may 15"),
// returning the canonical YYYY-MM-DD form.
func parseDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("date is empty")
	}
	if ymdRE.MatchString(s) {
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return "", err
		}
		return s, nil
	}
	// "in 3 days" → "3 days" (GNU date doesn't accept the leading "in ")
	if rest, ok := strings.CutPrefix(strings.ToLower(s), "in "); ok {
		s = rest
	}
	out, err := exec.Command("date", "-d", s, "+%Y-%m-%d").Output()
	if err != nil {
		return "", fmt.Errorf("could not parse date %q (try YYYY-MM-DD, 'wednesday', 'tomorrow', '3 days', etc.)", s)
	}
	return strings.TrimSpace(string(out)), nil
}

// formatParsedDate returns "2026-04-29 (Wed)" for display alongside the canonical date.
func formatParsedDate(ymd string) string {
	t, err := time.Parse("2006-01-02", ymd)
	if err != nil {
		return ymd
	}
	return fmt.Sprintf("%s (%s)", ymd, t.Format("Mon"))
}
