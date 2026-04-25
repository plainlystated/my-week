package build

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/plainlystated/my-week/internal/config"
	"github.com/plainlystated/my-week/internal/cup"
	"github.com/plainlystated/my-week/internal/snapshot"
)

// recap is the rendered "Last week" section content.
type recap struct {
	header string
	lines  []string
}

type recapItem struct {
	id     string
	name   string
	closed bool
	recur  string
	due    time.Time
}

// buildRecap walks the prev cache's Overdue/This Week/Heads Up sections,
// looks up each ID's current status from the bulk fetch, and produces a recap.
// Returns nil if prevSnap is nil or there are no recap-eligible IDs.
func buildRecap(client *cup.Client, _ *config.Config, prevSnap *snapshot.Snapshot, bulk map[string]cup.Task) *recap {
	if prevSnap == nil {
		return nil
	}
	ids := extractRecapIDs(prevSnap.Body)
	if len(ids) == 0 {
		return nil
	}

	items := make([]recapItem, 0, len(ids))
	closedCount := 0
	for _, id := range ids {
		t, ok := bulk[id]
		if !ok {
			continue
		}
		it := recapItem{id: id, name: t.Name, closed: t.IsDone(), due: t.DueTime()}
		if it.closed {
			closedCount++
		}
		items = append(items, it)
	}

	annotateRecur(client, items)

	now := time.Now()
	var lines []string
	for _, it := range items {
		mark := " "
		if it.closed {
			mark = "x"
		}
		line := fmt.Sprintf("- [%s] %s — **%s**", mark, it.id, it.name)
		switch {
		case it.closed && it.recur != "":
			line += fmt.Sprintf(" (recurs %s)", it.recur)
		case !it.closed && !it.due.IsZero():
			d := daysBetween(it.due, now)
			if d > 0 {
				line += fmt.Sprintf(" — still open, now %s late", pluralDays(d))
			}
		}
		lines = append(lines, line)
	}

	return &recap{
		header: fmt.Sprintf("Closed %d of %d deadlined items.", closedCount, len(items)),
		lines:  lines,
	}
}

// extractRecapIDs returns task IDs found under the prev cache's Overdue,
// This Week, and Heads Up section headers. Backlog is intentionally skipped.
func extractRecapIDs(body string) []string {
	prefixes := []string{"## Overdue", "## This Week", "## Heads Up"}
	collect := false
	var ids []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			collect = false
			for _, p := range prefixes {
				if strings.HasPrefix(line, p) {
					collect = true
					break
				}
			}
			continue
		}
		if !collect {
			continue
		}
		if m := snapshot.TaskLineRE.FindStringSubmatch(line); m != nil {
			ids = append(ids, m[4])
		}
	}
	return ids
}

// annotateRecur fetches the recur custom field for each closed item in
// parallel and writes it back into items. Errors are silently ignored — the
// annotation is a nice-to-have.
func annotateRecur(client *cup.Client, items []recapItem) {
	var wg sync.WaitGroup
	for i := range items {
		if !items[i].closed {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			full, err := client.TaskGet(items[i].id)
			if err != nil {
				return
			}
			items[i].recur = full.RecurValue()
		}(i)
	}
	wg.Wait()
}
