# my-week

A small Go CLI for the weekly "what's on my plate" review, backed by ClickUp.

`mw` keeps a markdown cache of this week's tasks and reads it instantly. A cron job (or systemd timer) refreshes the cache hourly, runs the daily info-tag and recurrence sweeps, and emails a Monday digest. Optional `mw add` and `mw chat` shell out to Claude Code for capture and one-task help.

## Why

The previous LLM-driven version was correct but slow — minutes per invocation, dominated by inference between tool calls. `mw` does the read path in pure Go: the slowest step is one `cup` (ClickUp CLI) bulk fetch, so reads are essentially `cat`-fast.

## Install

```sh
make install              # installs to ~/bin (override with PREFIX=...)
```

Requires the [`cup`](https://www.npmjs.com/package/clickup-cli) ClickUp CLI on `$PATH`. For `mw add` / `mw chat`, also requires the `claude` CLI (Claude Code).

## Configure

Copy `config.example.yml` to `~/.config/my-week/<profile>.yml` and fill in your ClickUp space ID, list IDs, and email addresses. Run `cup spaces` and `cup lists <space_id>` to discover the IDs.

```sh
mkdir -p ~/.config/my-week
cp config.example.yml ~/.config/my-week/personal.yml
$EDITOR ~/.config/my-week/personal.yml
```

`mw` accepts `-p <profile>` (default `personal`). For multiple profiles, alias to taste:

```sh
alias mwp='mw -p personal'
alias mwc='mw -p cjp'
```

## Commands

```
mw                          # cat the cache (auto-builds if missing or stale)
mw refresh                  # update the cache (auto-detects fresh build vs daily refresh)
mw done <id>                # mark complete + flip cache
mw snooze <id> <YYYY-MM-DD> # push due date
mw add "<text>"             # one-shot capture via Claude
mw add                      # bulk capture (interactive Claude session)
mw chat <id>                # interactive Claude with task body loaded
mw promote <id> [flags]     # move from inbox list to admin (or birthdays)
mw drop <id>                # complete an inbox item
mw digest                   # render digest markdown to stdout
mw send-digest              # render + email via Resend
```

## Refresh paths

`mw refresh` auto-picks one of three paths based on the cache file's frontmatter:

- **FRESH_BUILD** — cache missing or for a different ISO week. Runs sweeps, fetches buckets in parallel, builds the last-week recap, writes a new cache. Triggers the Monday digest gate on success.
- **REFRESH_WITH_SWEEPS** — cache from this week, but `swept_on` is from a previous day. Runs sweeps, then bulk-refreshes checkboxes and appends new items.
- **REFRESH_ONLY** — same-day refresh. Single bulk fetch, flip checkboxes, append new date-anchored items. No sweeps. Sub-second.

Recap, rotating-backlog selection, and the always-on / rotating split are produced by FRESH_BUILD and preserved by mid-week refreshes.

## Cron

Run `mw refresh` hourly. Add this line to your crontab (`crontab -e`):

```
0 * * * * /usr/bin/env -i HOME=$HOME PATH=/usr/bin:/usr/local/bin:$HOME/bin RESEND_API_KEY=... mw refresh -p personal
```

Tweak the `PATH` to wherever `mw` and `cup` actually live, and pass `RESEND_API_KEY` from wherever you keep secrets (mise, pass, etc.). The Monday digest send is gated by the cache's frontmatter and a small state file at `~/.local/state/my-week/<profile>-meta.yml` — missed runs are caught up automatically.

## Files on disk

- `~/.config/my-week/<profile>.yml` — config (you write this)
- `~/.local/state/my-week/<profile>-<iso-week>.md` — weekly cache
- `~/.local/state/my-week/<profile>-meta.yml` — small state file (`last_digest_sent`)

## Development

```sh
make build      # ./mw
make test
make fmt vet
```

Tests use a fake `cup` binary via `MW_CUP_BIN` for end-to-end exercise without hitting ClickUp.

## Spec

See `SPEC.md` for the design contract this implementation follows.
