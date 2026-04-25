// Package sweep runs the daily info-tag and recurrence sweeps.
//
// Both sweeps mutate ClickUp state and are designed to be idempotent across
// runs. The recurrence sweep relies on a "Next instance: <id>" comment on the
// parent task as the dedup marker.
package sweep

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/plainlystated/my-week/internal/config"
	"github.com/plainlystated/my-week/internal/cup"
	"github.com/plainlystated/my-week/internal/recur"
)

// Result reports what the sweeps did.
type Result struct {
	InfoCleared        int
	RecurrencesCreated int
	Errors             []string // human-readable messages, one per failed task
}

// recurWorkers is the concurrency bound for the per-task fetches in the
// recurrence sweep. ClickUp rate-limits aggressive bursts, so keep this modest.
const recurWorkers = 8

// Run executes both daily sweeps and returns combined results.
func Run(client *cup.Client, cfg *config.Config, today time.Time) Result {
	var r Result

	cleared, errs := sweepInfoTag(client, cfg, today)
	r.InfoCleared = cleared
	r.Errors = append(r.Errors, errs...)

	created, errs := sweepRecurrence(client, cfg)
	r.RecurrencesCreated = created
	r.Errors = append(r.Errors, errs...)

	return r
}

// sweepInfoTag completes every info-tagged task whose due date is on or before today.
func sweepInfoTag(client *cup.Client, cfg *config.Config, today time.Time) (int, []string) {
	dueBefore := today.AddDate(0, 0, 1) // include "due today"
	tasks, err := client.Tasks(cup.TaskQuery{
		All:       true,
		SpaceID:   cfg.ClickUp.SpaceID,
		Tag:       "info",
		DueBefore: dueBefore,
	})
	if err != nil {
		return 0, []string{fmt.Sprintf("info sweep query: %v", err)}
	}

	var (
		count int
		errs  []string
	)
	for _, t := range tasks {
		if t.IsDone() {
			continue
		}
		if err := client.Update(t.ID, cup.UpdateOpts{Status: "complete"}); err != nil {
			errs = append(errs, fmt.Sprintf("info sweep: complete %s: %v", t.ID, err))
			continue
		}
		count++
	}
	return count, errs
}

// sweepRecurrence walks all closed tasks; for each one with a recur custom
// field and no idempotency marker, creates a successor task and posts the marker.
func sweepRecurrence(client *cup.Client, cfg *config.Config) (int, []string) {
	closed, err := client.Tasks(cup.TaskQuery{
		All:           true,
		SpaceID:       cfg.ClickUp.SpaceID,
		IncludeClosed: true,
	})
	if err != nil {
		return 0, []string{fmt.Sprintf("recurrence sweep query: %v", err)}
	}

	// Pre-filter to closed tasks with a due date — recurrence needs an anchor.
	var candidates []cup.Task
	for _, t := range closed {
		if !t.IsDone() {
			continue
		}
		if t.DueRaw == "" {
			continue
		}
		candidates = append(candidates, t)
	}

	type workResult struct {
		created bool
		errMsg  string
	}
	jobs := make(chan cup.Task)
	results := make(chan workResult)
	var wg sync.WaitGroup
	for i := 0; i < recurWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				results <- processRecurCandidate(client, cfg, t)
			}
		}()
	}

	go func() {
		for _, t := range candidates {
			jobs <- t
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	var (
		created int
		errs    []string
	)
	for r := range results {
		if r.created {
			created++
		}
		if r.errMsg != "" {
			errs = append(errs, r.errMsg)
		}
	}
	return created, errs
}

func processRecurCandidate(client *cup.Client, cfg *config.Config, t cup.Task) (out struct {
	created bool
	errMsg  string
}) {
	full, err := client.TaskGet(t.ID)
	if err != nil {
		out.errMsg = fmt.Sprintf("recurrence: TaskGet %s: %v", t.ID, err)
		return
	}
	recurVal := full.RecurValue()
	if recurVal == "" {
		return
	}

	hasMarker, err := client.HasNextInstanceComment(t.ID)
	if err != nil {
		out.errMsg = fmt.Sprintf("recurrence: comments %s: %v", t.ID, err)
		return
	}
	if hasMarker {
		return
	}

	interval, err := recur.Parse(recurVal)
	if err != nil {
		out.errMsg = fmt.Sprintf("recurrence: bad interval %q on %s: %v", recurVal, t.ID, err)
		return
	}
	due := t.DueTime()
	if due.IsZero() {
		out.errMsg = fmt.Sprintf("recurrence: %s has recur but unparseable dueRaw %q", t.ID, t.DueRaw)
		return
	}
	next, err := recur.Next(due, interval)
	if err != nil {
		out.errMsg = fmt.Sprintf("recurrence: Next(%s): %v", t.ID, err)
		return
	}

	listID := full.ListID()
	if listID == "" {
		listID = fallbackListID(full, cfg)
	}
	if listID == "" {
		out.errMsg = fmt.Sprintf("recurrence: cannot resolve list ID for %s (list=%+v)", t.ID, full.List)
		return
	}

	newID, err := client.Create(cup.CreateOpts{
		ListID:      listID,
		Name:        full.Name,
		Description: full.Description,
		DueDate:     next.Format("2006-01-02"),
		Priority:    full.PriorityName(),
		Tags:        full.TagNames(),
	})
	if err != nil {
		out.errMsg = fmt.Sprintf("recurrence: create successor for %s: %v", t.ID, err)
		return
	}

	if err := client.SetField(newID, "recur", recurVal); err != nil {
		out.errMsg = fmt.Sprintf("recurrence: set recur on %s (successor of %s): %v", newID, t.ID, err)
		// proceed to post marker anyway — the successor exists, just lacks the field
	}

	if err := client.PostComment(t.ID, "Next instance: "+newID); err != nil {
		out.errMsg = fmt.Sprintf("recurrence: marker comment on %s: %v (successor %s created)", t.ID, err, newID)
		return
	}

	out.created = true
	return
}

// fallbackListID resolves a list ID from the parent task's list NAME if the
// API didn't include the ID. In practice cup returns full.List.ID; this is
// just a backstop.
func fallbackListID(full *cup.TaskFull, cfg *config.Config) string {
	switch strings.ToLower(strings.TrimSpace(full.List.Name)) {
	case "admin", "tasks":
		return cfg.ClickUp.Lists.Admin
	case "birthdays":
		return cfg.ClickUp.Lists.Birthdays
	case "inbox":
		return cfg.ClickUp.Lists.Inbox
	}
	return ""
}
