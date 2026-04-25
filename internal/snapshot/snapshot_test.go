package snapshot

import (
	"strings"
	"testing"
	"time"
)

const fixture = `---
profile: personal
iso_week: 2026-W18
generated_at: 2026-04-27T08:00:12-05:00
refreshed_at: 2026-04-27T14:00:03-05:00
swept_on: 2026-04-27
---

# Personal — week of Mon Apr 27

## Last week (W17) recap
Closed 4 of 6 deadlined items.

- [x] 86b9gg5ux — **Update GitHub token for Claude**
- [ ] 86b9bbxxx — **Renew passport** — still open, now 5 days late

## Overdue
- [ ] 86b9ovxx1 — **Schedule oil change** — 3 days late

## This Week
- [ ] 86b9twxx1 — **Renew passport** — due Thu

## Backlog — rotating (locked for this week)
- [ ] 86b9bk010 — **Order laptop chargers**
- [ ] 86b9bk011 — **Tree health check**

---

Open Claude Code in ` + "`my-week/`" + ` and run ` + "`mw chat <id>`" + ` for help on any item.
`

func TestRoundTrip(t *testing.T) {
	s, err := Parse(fixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := s.Render()
	if got != fixture {
		t.Errorf("round-trip differs:\nwant:\n%q\n\ngot:\n%q", fixture, got)
	}
}

func TestParseFrontMatter(t *testing.T) {
	s, err := Parse(fixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fm := s.FrontMatter
	if fm.Profile != "personal" {
		t.Errorf("profile = %q", fm.Profile)
	}
	if fm.ISOWeek != "2026-W18" {
		t.Errorf("iso_week = %q", fm.ISOWeek)
	}
	if fm.SweptOn != "2026-04-27" {
		t.Errorf("swept_on = %q", fm.SweptOn)
	}
	wantGen := time.Date(2026, time.April, 27, 8, 0, 12, 0, time.FixedZone("", -5*3600))
	if !fm.GeneratedAt.Equal(wantGen) {
		t.Errorf("generated_at = %v, want %v", fm.GeneratedAt, wantGen)
	}
}

func TestIDs(t *testing.T) {
	s, _ := Parse(fixture)
	got := s.IDs()
	want := []string{"86b9gg5ux", "86b9bbxxx", "86b9ovxx1", "86b9twxx1", "86b9bk010", "86b9bk011"}
	if len(got) != len(want) {
		t.Fatalf("IDs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("IDs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFlipCheckboxes(t *testing.T) {
	s, _ := Parse(fixture)
	s.FlipCheckboxes(map[string]bool{
		"86b9ovxx1": true,  // open → done
		"86b9gg5ux": false, // already done, becomes open
		"unknown":   true,  // missing IDs are ignored
	})
	if !strings.Contains(s.Body, "- [x] 86b9ovxx1 — **Schedule oil change**") {
		t.Errorf("expected 86b9ovxx1 to be flipped to [x]")
	}
	if !strings.Contains(s.Body, "- [ ] 86b9gg5ux — **Update GitHub token for Claude**") {
		t.Errorf("expected 86b9gg5ux to be flipped to [ ]")
	}
	// Unchanged lines stay intact.
	if !strings.Contains(s.Body, "- [ ] 86b9bbxxx — **Renew passport** — still open, now 5 days late") {
		t.Errorf("unchanged line was modified")
	}
}

func TestAppendNewItems_NewSection(t *testing.T) {
	s, _ := Parse(fixture)
	s.AppendNewItems([]string{
		"- [ ] 86b9nz001 — **Return Costco rotisserie** — due Fri",
	})
	if !strings.Contains(s.Body, "## New since snapshot\n- [ ] 86b9nz001") {
		t.Errorf("expected new section with item, got body:\n%s", s.Body)
	}
	// Footer ("Open Claude Code...") must still come after the new section.
	newIdx := strings.Index(s.Body, "## New since snapshot")
	footerIdx := strings.Index(s.Body, "Open Claude Code")
	if newIdx == -1 || footerIdx == -1 || newIdx > footerIdx {
		t.Errorf("new section must precede footer; new=%d footer=%d", newIdx, footerIdx)
	}
}

func TestRemoveID(t *testing.T) {
	s, _ := Parse(fixture)
	s.RemoveID("86b9ovxx1") // the Overdue line
	if strings.Contains(s.Body, "86b9ovxx1") {
		t.Errorf("RemoveID did not drop the line")
	}
	// Other tasks remain untouched.
	for _, id := range []string{"86b9gg5ux", "86b9twxx1", "86b9bk010"} {
		if !strings.Contains(s.Body, id) {
			t.Errorf("RemoveID dropped unrelated id %s", id)
		}
	}
}

func TestAppendNewItems_MergeDeduplicates(t *testing.T) {
	body := strings.Replace(fixture, "## Backlog", `## New since snapshot
- [ ] 86b9old01 — **Existing**

## Backlog`, 1)
	s, err := Parse(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s.AppendNewItems([]string{
		"- [ ] 86b9old01 — **Duplicate** — should be skipped",
		"- [ ] 86b9new01 — **Fresh** — added",
	})
	if strings.Contains(s.Body, "**Duplicate**") {
		t.Errorf("duplicate should not have been appended")
	}
	if !strings.Contains(s.Body, "86b9new01") {
		t.Errorf("fresh item should have been appended")
	}
}
