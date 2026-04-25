// Command mw is the my-week CLI.
package main

import (
	"fmt"
	"os"
	"strings"
)

const usage = `mw — weekly task review CLI

Usage:
  mw [-p profile] [command] [args...]

Commands (default is to print the cache):
  (none)              cat the weekly cache file
  refresh             update the cache (auto-detects fresh build vs daily refresh)
  done <id>           mark a task complete and flip the cache line
  snooze <id> <date>  push a task's due date and refresh the line
  add ["text"]        capture a new task via Claude
  chat <id>           open a Claude session with the task body loaded
  promote <id>        move a task from inbox to admin (or birthdays)
  drop <id>           drop an inbox item
  digest              render the digest markdown to stdout
  send-digest         render and email via Resend (used by cron)

Flags:
  -p, --profile NAME  profile to use (default "personal")
  -h, --help          show this help
`

func main() {
	profile, args := splitGlobalFlags(os.Args[1:])
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	var err error
	switch cmd {
	case "", "read":
		err = cmdRead(profile)
	case "refresh":
		err = cmdRefresh(profile)
	case "done":
		err = cmdDone(profile, args)
	case "snooze":
		err = cmdSnooze(profile, args)
	case "promote":
		err = cmdPromote(profile, args)
	case "drop":
		err = cmdDrop(profile, args)
	case "digest":
		err = cmdDigest(profile)
	case "send-digest":
		err = cmdSendDigest(profile)
	case "add":
		err = cmdAdd(profile, args)
	case "chat":
		err = cmdChat(profile, args)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "mw: %v\n", err)
		os.Exit(1)
	}
}

// splitGlobalFlags pulls -p/--profile out of args wherever they appear and
// returns the remaining positional args plus the resolved profile.
func splitGlobalFlags(in []string) (profile string, rest []string) {
	profile = "personal"
	rest = make([]string, 0, len(in))
	for i := 0; i < len(in); i++ {
		a := in[i]
		switch {
		case a == "-p" || a == "--profile":
			if i+1 < len(in) {
				profile = in[i+1]
				i++
			}
		case strings.HasPrefix(a, "-p="):
			profile = a[len("-p="):]
		case strings.HasPrefix(a, "--profile="):
			profile = a[len("--profile="):]
		default:
			rest = append(rest, a)
		}
	}
	return profile, rest
}
