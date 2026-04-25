package build

import (
	"fmt"
	"strings"
	"time"

	"github.com/plainlystated/my-week/internal/config"
	"github.com/plainlystated/my-week/internal/cup"
	"github.com/plainlystated/my-week/internal/sweep"
)

// render produces the markdown body for the cache file.
func render(cfg *config.Config, today time.Time, sw sweep.Result, b *buckets, r *recap) string {
	var out strings.Builder

	monday := mondayOf(today)
	fmt.Fprintf(&out, "\n# %s\n", title(cfg.Profile, monday))

	if r != nil {
		out.WriteString("\n## Last week recap\n")
		out.WriteString(r.header)
		out.WriteString("\n\n")
		for _, ln := range r.lines {
			out.WriteString(ln)
			out.WriteString("\n")
		}
	}

	if len(b.inbox) > 0 {
		fmt.Fprintf(&out, "\n## Inbox (%d to triage)\n", len(b.inbox))
		for _, t := range b.inbox {
			fmt.Fprintf(&out, "- [ ] %s — **%s**\n", t.ID, t.Name)
		}
	}

	if len(b.overdue) > 0 {
		out.WriteString("\n## Overdue\n")
		for _, t := range b.overdue {
			fmt.Fprintf(&out, "- [ ] %s — **%s**%s\n", t.ID, t.Name, overdueNudge(t, today))
		}
	}

	// "This Week" always renders, even when empty.
	out.WriteString("\n## This Week\n")
	if len(b.thisWeek) == 0 {
		out.WriteString("(nothing due)\n")
	} else {
		for _, t := range b.thisWeek {
			fmt.Fprintf(&out, "- [ ] %s — **%s**%s\n", t.ID, t.Name, thisWeekNudge(t, today))
		}
	}

	if len(b.headsUp) > 0 {
		weeks := cfg.LookaheadDays * cfg.LookaheadMultiplier
		fmt.Fprintf(&out, "\n## Heads Up (next %d days)\n", weeks)
		for _, t := range b.headsUp {
			fmt.Fprintf(&out, "- [ ] %s — **%s**%s\n", t.ID, t.Name, headsUpNudge(t, today))
		}
	}

	if info := renderInfo(sw.InfoCleared, b.birthdays); info != "" {
		out.WriteString("\n## Info\n")
		out.WriteString(info)
	}

	if len(b.backlogPrio) > 0 {
		out.WriteString("\n## Backlog — always-on (High/Urgent)\n")
		for _, t := range b.backlogPrio {
			suffix := ""
			if p := titleCasePriority(t.Priority); p != "" && p != "Normal" {
				suffix = " — " + p
			}
			fmt.Fprintf(&out, "- [ ] %s — **%s**%s\n", t.ID, t.Name, suffix)
		}
	}

	if len(b.backlogRotate) > 0 {
		out.WriteString("\n## Backlog — rotating (locked for this week)\n")
		for _, t := range b.backlogRotate {
			fmt.Fprintf(&out, "- [ ] %s — **%s**\n", t.ID, t.Name)
		}
	}

	out.WriteString("\n---\n\nOpen Claude Code in `my-week/` and run `mw chat <id>` for help on any item.\n")
	return out.String()
}

func overdueNudge(t cup.Task, today time.Time) string {
	due := t.DueTime()
	if due.IsZero() {
		return ""
	}
	d := daysBetween(due, today)
	if d <= 0 {
		return ""
	}
	return fmt.Sprintf(" — %s late", pluralDays(d))
}

func thisWeekNudge(t cup.Task, today time.Time) string {
	due := t.DueTime()
	if due.IsZero() {
		return ""
	}
	d := daysBetween(today, due)
	switch {
	case d < 0:
		return fmt.Sprintf(" — %s late", pluralDays(-d))
	case d == 0:
		return " — due today"
	case d == 1:
		return " — due tomorrow"
	}
	return fmt.Sprintf(" — due %s", due.Format("Mon"))
}

func headsUpNudge(t cup.Task, today time.Time) string {
	due := t.DueTime()
	if due.IsZero() {
		return ""
	}
	d := daysBetween(today, due)
	return fmt.Sprintf(" — due %s (%d days out)", due.Format("2006-01-02"), d)
}

func renderInfo(cleared int, birthdays []cup.Task) string {
	var lines []string
	if cleared > 0 {
		lines = append(lines, fmt.Sprintf("- %d awareness items auto-cleared this week.", cleared))
	}
	if len(birthdays) > 0 {
		var names []string
		for _, b := range birthdays {
			due := b.DueTime()
			if due.IsZero() {
				names = append(names, b.Name)
				continue
			}
			names = append(names, fmt.Sprintf("%s (%s)", b.Name, due.Format("Jan 2")))
		}
		lines = append(lines, "- Upcoming birthdays: "+strings.Join(names, ", ")+".")
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
