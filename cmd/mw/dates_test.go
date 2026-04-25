package main

import (
	"regexp"
	"testing"
)

func TestParseDate(t *testing.T) {
	// Reject empty / garbage inputs.
	for _, bad := range []string{"", "  ", "asdf", "wensday"} {
		if _, err := parseDate(bad); err == nil {
			t.Errorf("parseDate(%q): expected error", bad)
		}
	}

	// Pass-through for canonical YYYY-MM-DD.
	got, err := parseDate("2026-04-29")
	if err != nil || got != "2026-04-29" {
		t.Errorf("parseDate(\"2026-04-29\") = %q, %v; want 2026-04-29, nil", got, err)
	}

	// Natural-language phrases must produce a YYYY-MM-DD result. We don't
	// pin specific dates because the result is relative to "now".
	ymd := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	for _, in := range []string{
		"wednesday", "wed", "tomorrow", "next friday",
		"3 days", "in 3 days", "next monday", "may 15",
	} {
		got, err := parseDate(in)
		if err != nil {
			t.Errorf("parseDate(%q): unexpected error %v", in, err)
			continue
		}
		if !ymd.MatchString(got) {
			t.Errorf("parseDate(%q) = %q; expected YYYY-MM-DD", in, got)
		}
	}
}
