package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/plainlystated/my-week/internal/config"
	"github.com/plainlystated/my-week/internal/cup"
)

// cmdDone marks a task complete and flips its cache line.
func cmdDone(profile string, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: mw done <id-or-suffix>")
	}
	cfg, err := config.Load(profile)
	if err != nil {
		return err
	}
	id, err := resolveID(cfg, args[0])
	if err != nil {
		return err
	}
	if err := cup.New(cfg.CupProfile).Update(id, cup.UpdateOpts{Status: "complete"}); err != nil {
		return err
	}
	if err := flipCacheLine(cfg.Profile, id, true, time.Now()); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "Marked %s complete.\n", id)
	return nil
}

// cmdSnooze sets a task's due date.
func cmdSnooze(profile string, args []string) error {
	if len(args) < 2 {
		return errors.New("usage: mw snooze <id-or-suffix> <date>  (YYYY-MM-DD or 'wednesday' / 'tomorrow' / '3 days')")
	}
	cfg, err := config.Load(profile)
	if err != nil {
		return err
	}
	id, err := resolveID(cfg, args[0])
	if err != nil {
		return err
	}
	date, err := parseDate(strings.Join(args[1:], " "))
	if err != nil {
		return err
	}
	if err := cup.New(cfg.CupProfile).Update(id, cup.UpdateOpts{DueDate: date}); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "Snoozed %s to %s. Run `mw refresh` to update the cache.\n", id, formatParsedDate(date))
	return nil
}

// cmdDrop marks an inbox item complete and flips its cache line.
func cmdDrop(profile string, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: mw drop <id-or-suffix>")
	}
	cfg, err := config.Load(profile)
	if err != nil {
		return err
	}
	id, err := resolveID(cfg, args[0])
	if err != nil {
		return err
	}
	if err := cup.New(cfg.CupProfile).Update(id, cup.UpdateOpts{Status: "complete"}); err != nil {
		return err
	}
	if err := flipCacheLine(cfg.Profile, id, true, time.Now()); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "Dropped %s.\n", id)
	return nil
}

// promoteFlags holds the optional flags for `mw promote`.
type promoteFlags struct {
	name     string
	list     string // "admin" or "birthdays"
	due      string // YYYY-MM-DD
	priority string
	tags     []string
	recur    string
}

// cmdPromote moves a task from the inbox list to admin (or birthdays) and
// applies any optional metadata.
func cmdPromote(profile string, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: mw promote <id-or-suffix> [--name X] [--list admin|birthdays] [--due DATE] [--priority P] [--tags a,b] [--recur INT]")
	}
	cfg, err := config.Load(profile)
	if err != nil {
		return err
	}
	id, err := resolveID(cfg, args[0])
	if err != nil {
		return err
	}
	flags, err := parsePromoteFlags(args[1:])
	if err != nil {
		return err
	}
	client := cup.New(cfg.CupProfile)

	target := flags.list
	if target == "" {
		target = "admin"
	}
	var listID string
	switch strings.ToLower(target) {
	case "admin":
		listID = cfg.ClickUp.Lists.Admin
	case "birthdays":
		listID = cfg.ClickUp.Lists.Birthdays
	default:
		return fmt.Errorf("invalid --list %q (expected admin|birthdays)", flags.list)
	}
	if listID == "" {
		return fmt.Errorf("config has no list ID for %q", target)
	}

	// ClickUp tasks can live in multiple lists, so a true "move" needs both
	// --to (add to target) and --remove (drop from inbox). If inbox is unset
	// in config, just add to the target.
	moveOpts := cup.MoveOpts{To: listID, Remove: cfg.ClickUp.Lists.Inbox}
	if err := client.Move(id, moveOpts); err != nil {
		return fmt.Errorf("move: %w", err)
	}

	if flags.due != "" {
		parsed, err := parseDate(flags.due)
		if err != nil {
			return err
		}
		flags.due = parsed
	}

	if flags.name != "" || flags.due != "" || flags.priority != "" {
		opts := cup.UpdateOpts{Name: flags.name, DueDate: flags.due, Priority: flags.priority}
		if err := client.Update(id, opts); err != nil {
			return fmt.Errorf("update metadata: %w", err)
		}
	}
	if flags.recur != "" {
		if err := client.SetField(id, "recur", flags.recur); err != nil {
			return fmt.Errorf("set recur: %w", err)
		}
	}

	// Drop the now-stale inbox line from the cache so `mw` shows it gone.
	// REFRESH_ONLY doesn't re-categorize, so without this the line would
	// linger under ## Inbox until the next FRESH_BUILD.
	if err := removeCacheLine(cfg.Profile, id, time.Now()); err != nil {
		fmt.Fprintf(stderr, "warn: removed task in cup but cache update failed: %v\n", err)
	}

	fmt.Fprintf(stderr, "Promoted %s to %s.\n", id, target)
	return nil
}

func parsePromoteFlags(args []string) (promoteFlags, error) {
	var f promoteFlags
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("flag %s expects a value", a)
			}
			i++
			return args[i], nil
		}
		var err error
		switch a {
		case "--name":
			f.name, err = next()
		case "--list":
			f.list, err = next()
		case "--due":
			f.due, err = next()
		case "--priority":
			f.priority, err = next()
		case "--tags":
			v, e := next()
			err = e
			if err == nil {
				f.tags = strings.Split(v, ",")
			}
		case "--recur":
			f.recur, err = next()
		default:
			return f, fmt.Errorf("unknown flag %s", a)
		}
		if err != nil {
			return f, err
		}
	}
	return f, nil
}
