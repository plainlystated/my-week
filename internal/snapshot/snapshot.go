// Package snapshot parses, manipulates, and renders the weekly cache file.
//
// The cache file is YAML frontmatter + a markdown body. The body is preserved
// verbatim across round-trips; manipulations only touch task lines (matched
// by TaskLineRE) and a small number of named sections.
package snapshot

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// TaskLineRE matches `- [ ] <id> ...` and `- [x] <id> ...`. Capture groups:
//
//	1: "- ["
//	2: " " or "x"
//	3: "] "
//	4: ClickUp task ID
//	5: trailing text (everything after the ID, may include " — **name** — nudge")
var TaskLineRE = regexp.MustCompile(`^(- \[)([ x])(\] )([a-zA-Z0-9_]+)(.*)$`)

// Snapshot is a parsed cache file.
type Snapshot struct {
	FrontMatter FrontMatter
	Body        string // markdown body (everything after the closing ---), preserved verbatim
}

// FrontMatter is the cache file's YAML header.
type FrontMatter struct {
	Profile     string
	ISOWeek     string
	GeneratedAt time.Time
	RefreshedAt time.Time
	SweptOn     string // YYYY-MM-DD; "" if never swept
}

// Parse extracts frontmatter and body from a cache file's text.
func Parse(text string) (*Snapshot, error) {
	// Frontmatter convention: file starts with "---\n", then key: value lines,
	// then a closing "---\n", then the body.
	if !strings.HasPrefix(text, "---\n") {
		return nil, errors.New("cache file missing leading '---' frontmatter delimiter")
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return nil, errors.New("cache file missing closing '---' frontmatter delimiter")
	}
	fmText := rest[:end]
	body := rest[end+len("\n---\n"):]

	fm, err := parseFrontMatter(fmText)
	if err != nil {
		return nil, err
	}
	return &Snapshot{FrontMatter: fm, Body: body}, nil
}

// Render writes the snapshot back to the on-disk format.
func (s *Snapshot) Render() string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(renderFrontMatter(s.FrontMatter))
	b.WriteString("---\n")
	b.WriteString(s.Body)
	return b.String()
}

// IDs returns every ClickUp ID referenced by a task line in the body.
func (s *Snapshot) IDs() []string {
	var out []string
	for _, line := range strings.Split(s.Body, "\n") {
		if m := TaskLineRE.FindStringSubmatch(line); m != nil {
			out = append(out, m[4])
		}
	}
	return out
}

// RemoveID deletes any task line whose ID matches `id`, plus any blank
// section that drop leaves behind. Used after `mw promote` so the inbox line
// disappears immediately rather than waiting for the next FRESH_BUILD.
func (s *Snapshot) RemoveID(id string) {
	lines := strings.Split(s.Body, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		m := TaskLineRE.FindStringSubmatch(line)
		if m != nil && m[4] == id {
			continue
		}
		out = append(out, line)
	}
	s.Body = strings.Join(out, "\n")
}

// FlipCheckboxes updates [x]/[ ] on every task line whose ID is in statusByID.
// IDs missing from the map are left as-is (treated as "deleted from ClickUp").
func (s *Snapshot) FlipCheckboxes(doneByID map[string]bool) {
	lines := strings.Split(s.Body, "\n")
	for i, line := range lines {
		m := TaskLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id := m[4]
		done, present := doneByID[id]
		if !present {
			continue
		}
		mark := " "
		if done {
			mark = "x"
		}
		lines[i] = fmt.Sprintf("%s%s%s%s%s", m[1], mark, m[3], id, m[5])
	}
	s.Body = strings.Join(lines, "\n")
}

// AppendNewItems adds bullet lines under "## New since snapshot", merging
// duplicates by ID. If the section doesn't exist, it's inserted before the
// trailing footer (if any) or at the end of the body.
func (s *Snapshot) AppendNewItems(newLines []string) {
	if len(newLines) == 0 {
		return
	}
	const section = "## New since snapshot"

	body := s.Body
	footerIdx := lastFooterIndex(body)

	var content, footer string
	if footerIdx >= 0 {
		content = strings.TrimRight(body[:footerIdx], "\n")
		footer = strings.TrimLeft(body[footerIdx:], "\n")
	} else {
		content = strings.TrimRight(body, "\n")
	}

	if idx := strings.Index(content, section); idx >= 0 {
		existing := collectIDs(content[idx+len(section):])
		var merged []string
		for _, ln := range newLines {
			m := TaskLineRE.FindStringSubmatch(ln)
			if m != nil && existing[m[4]] {
				continue
			}
			merged = append(merged, ln)
		}
		if len(merged) > 0 {
			content = strings.TrimRight(content, "\n") + "\n" + strings.Join(merged, "\n")
		}
	} else {
		content = content + "\n\n" + section + "\n" + strings.Join(newLines, "\n")
	}

	if footer != "" {
		if !strings.HasSuffix(footer, "\n") {
			footer += "\n"
		}
		s.Body = content + "\n\n" + footer
	} else {
		s.Body = content + "\n"
	}
}

// lastFooterIndex returns the index of the final "\n---\n" in body, which
// marks the start of the footer block. Returns -1 if absent.
func lastFooterIndex(body string) int {
	idx := -1
	for i := 0; ; {
		j := strings.Index(body[i:], "\n---\n")
		if j < 0 {
			break
		}
		idx = i + j + 1 // point AT the "---" line, not the "\n" before it
		i = i + j + 1
	}
	return idx
}

func collectIDs(s string) map[string]bool {
	out := make(map[string]bool)
	for _, line := range strings.Split(s, "\n") {
		if m := TaskLineRE.FindStringSubmatch(line); m != nil {
			out[m[4]] = true
		}
	}
	return out
}

func parseFrontMatter(text string) (FrontMatter, error) {
	var fm FrontMatter
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch k {
		case "profile":
			fm.Profile = v
		case "iso_week":
			fm.ISOWeek = v
		case "generated_at":
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return fm, fmt.Errorf("frontmatter generated_at: %w", err)
			}
			fm.GeneratedAt = t
		case "refreshed_at":
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return fm, fmt.Errorf("frontmatter refreshed_at: %w", err)
			}
			fm.RefreshedAt = t
		case "swept_on":
			fm.SweptOn = v
		}
	}
	return fm, nil
}

func renderFrontMatter(fm FrontMatter) string {
	var b strings.Builder
	if fm.Profile != "" {
		fmt.Fprintf(&b, "profile: %s\n", fm.Profile)
	}
	if fm.ISOWeek != "" {
		fmt.Fprintf(&b, "iso_week: %s\n", fm.ISOWeek)
	}
	if !fm.GeneratedAt.IsZero() {
		fmt.Fprintf(&b, "generated_at: %s\n", fm.GeneratedAt.Format(time.RFC3339))
	}
	if !fm.RefreshedAt.IsZero() {
		fmt.Fprintf(&b, "refreshed_at: %s\n", fm.RefreshedAt.Format(time.RFC3339))
	}
	if fm.SweptOn != "" {
		fmt.Fprintf(&b, "swept_on: %s\n", fm.SweptOn)
	}
	return b.String()
}
