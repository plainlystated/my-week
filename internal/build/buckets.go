package build

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/plainlystated/my-week/internal/config"
	"github.com/plainlystated/my-week/internal/cup"
)

// buckets holds the categorized task data the renderer needs.
type buckets struct {
	overdue        []cup.Task
	thisWeek       []cup.Task
	headsUp        []cup.Task
	birthdays      []cup.Task
	backlogPrio    []cup.Task // High/Urgent, no due date
	backlogRotate  []cup.Task // 3 picked for rotation, no due date, normal/low priority
	inbox          []cup.Task
	bulkByID       map[string]cup.Task // every task seen, indexed by ID — used for recap status lookup
}

// fetchBuckets runs all needed cup queries in parallel and assembles buckets.
func fetchBuckets(client *cup.Client, cfg *config.Config, today time.Time) (*buckets, error) {
	weekEnd := today.AddDate(0, 0, cfg.LookaheadDays)
	leadEnd := today.AddDate(0, 0, cfg.LookaheadDays*cfg.LookaheadMultiplier)
	tomorrow := today.AddDate(0, 0, 1)

	type fr struct {
		name  string
		tasks []cup.Task
		err   error
	}

	jobs := []struct {
		name string
		q    cup.TaskQuery
		skip bool
	}{
		{"overdue", cup.TaskQuery{All: true, SpaceID: cfg.ClickUp.SpaceID, DueBefore: today}, false},
		{"this-week", cup.TaskQuery{All: true, SpaceID: cfg.ClickUp.SpaceID, DueAfter: today.AddDate(0, 0, -1), DueBefore: weekEnd.AddDate(0, 0, 1)}, false},
		{"heads-up", cup.TaskQuery{All: true, SpaceID: cfg.ClickUp.SpaceID, Tag: "needs-lead-time", DueAfter: weekEnd, DueBefore: leadEnd.AddDate(0, 0, 1)}, false},
		{"birthdays", cup.TaskQuery{All: true, ListID: cfg.ClickUp.Lists.Birthdays, DueAfter: today.AddDate(0, 0, -1), DueBefore: leadEnd.AddDate(0, 0, 1)}, false},
		{"admin-list", cup.TaskQuery{All: true, ListID: cfg.ClickUp.Lists.Admin}, false},
		{"info-open", cup.TaskQuery{All: true, SpaceID: cfg.ClickUp.SpaceID, Tag: "info"}, false},
		{"inbox", cup.TaskQuery{All: true, ListID: cfg.ClickUp.Lists.Inbox}, cfg.ClickUp.Lists.Inbox == ""},
		{"bulk", cup.TaskQuery{All: true, SpaceID: cfg.ClickUp.SpaceID, IncludeClosed: true}, false},
	}
	_ = tomorrow // dueBefore semantics: we use today for overdue (strictly before), week_end+1 for inclusive

	results := make(chan fr, len(jobs))
	var wg sync.WaitGroup
	for _, j := range jobs {
		if j.skip {
			results <- fr{name: j.name}
			continue
		}
		wg.Add(1)
		go func(name string, q cup.TaskQuery) {
			defer wg.Done()
			tasks, err := client.Tasks(q)
			results <- fr{name: name, tasks: tasks, err: err}
		}(j.name, j.q)
	}
	wg.Wait()
	close(results)

	collected := make(map[string][]cup.Task, len(jobs))
	for r := range results {
		if r.err != nil {
			return nil, fmt.Errorf("bucket %q: %w", r.name, r.err)
		}
		collected[r.name] = r.tasks
	}

	infoSet := make(map[string]bool)
	for _, t := range collected["info-open"] {
		infoSet[t.ID] = true
	}

	b := &buckets{
		overdue:   excludeIDs(filterOpen(collected["overdue"]), infoSet),
		thisWeek:  excludeIDs(filterOpen(collected["this-week"]), infoSet),
		headsUp:   filterOpen(collected["heads-up"]),
		birthdays: filterOpen(collected["birthdays"]),
		inbox:     filterOpen(collected["inbox"]),
		bulkByID:  make(map[string]cup.Task, len(collected["bulk"])),
	}

	for _, t := range collected["bulk"] {
		b.bulkByID[t.ID] = t
	}

	// Backlog: open tasks in admin list with no due date.
	var backlog []cup.Task
	for _, t := range collected["admin-list"] {
		if t.IsDone() {
			continue
		}
		if t.DueRaw != "" {
			continue
		}
		backlog = append(backlog, t)
	}
	b.backlogPrio, b.backlogRotate = splitBacklog(backlog)
	return b, nil
}

// filterOpen drops done tasks and tasks missing an ID.
func filterOpen(tasks []cup.Task) []cup.Task {
	out := make([]cup.Task, 0, len(tasks))
	for _, t := range tasks {
		if t.ID == "" {
			continue
		}
		if t.IsDone() {
			continue
		}
		out = append(out, t)
	}
	return out
}

// excludeIDs returns tasks whose IDs are not in `excl`.
func excludeIDs(tasks []cup.Task, excl map[string]bool) []cup.Task {
	if len(excl) == 0 {
		return tasks
	}
	out := make([]cup.Task, 0, len(tasks))
	for _, t := range tasks {
		if excl[t.ID] {
			continue
		}
		out = append(out, t)
	}
	return out
}

// splitBacklog partitions backlog into (priority, rotating). Rotating is
// sorted deterministically and capped at 3.
//
// NOTE: spec calls for sorting by date_updated ascending, but the bulk Task
// shape doesn't include date_updated. We fall back to sorting by ID, which is
// loosely creation-order. Refinement: fetch full task data for backlog
// candidates if the rotation feels stale.
func splitBacklog(tasks []cup.Task) (priority, rotating []cup.Task) {
	for _, t := range tasks {
		switch t.Priority {
		case "urgent", "high":
			priority = append(priority, t)
		default:
			rotating = append(rotating, t)
		}
	}
	sort.Slice(rotating, func(i, j int) bool { return rotating[i].ID < rotating[j].ID })
	if len(rotating) > 3 {
		rotating = rotating[:3]
	}
	sort.Slice(priority, func(i, j int) bool {
		// urgent before high; then by name for stability
		if priority[i].Priority != priority[j].Priority {
			return priority[i].Priority == "urgent"
		}
		return priority[i].Name < priority[j].Name
	})
	return priority, rotating
}
