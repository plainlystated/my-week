# my-week — spec

A Go reimplementation of the personal-admin "my-week" workflow, built so that the read path never blocks on Claude. This document is self-contained: an implementer reading it cold should have everything needed to start.

## Why this exists

There is an existing system at `/home/patrick/Sync/ai_workspaces/personal/personal-admin/` with a Claude-driven slash command (`/my-week-personal`, `/my-week-cjp`) that reads tasks from ClickUp, runs sweeps, and renders a weekly digest. It works — but each invocation takes 8–18 minutes, dominated by LLM inference between tool calls (the `cup` queries themselves are sub-second). The pain isn't network; it's Claude in the read path.

This repo replaces the read/sync surface with pure Go and demotes Claude to two opt-in commands (`mw add`, `mw chat`). The read commands are `cat`-fast.

Timing log evidence (`personal-admin/state/personal-timing.log`):
- Fresh weekly build: ~15 min, of which `bulk_fetch` was 1 sec.
- First-of-day rerun with sweeps: ~18 min, of which `bulk_fetch` was 0.8 sec.
- Same-day rerun (which already delegates to a Python helper): 18 sec.

The whole budget is LLM round-trips. Pure Go should land everything in single-digit seconds.

## Decisions log (locked during design)

These are not up for debate during implementation. If you think one is wrong, surface it explicitly before changing it.

1. **Architecture**: cron-maintained local cache + thin `mw` CLI. Claude is invoked only on `mw add` and `mw chat`.
2. **Read shape**: `mw` reads a markdown file. No daemon, no TUI. Same on-disk format as the existing personal-admin snapshot.
3. **Single command `mw refresh`** auto-detects daily refresh vs weekly fresh-build by comparing the cache file's `iso_week` frontmatter to today's ISO week.
4. **Cron cadence**: hourly via systemd timer with `Persistent=true`. One entry per profile.
5. **Staleness handling**: read commands print `Refreshed Nm ago.` at top of output. If cache is >90 min old, auto-run `mw refresh` synchronously before display.
6. **Done items stay visible** in the cache file until the week rolls. Refresh flips `[x]` but never deletes lines. Carries the "what did I get done this week" signal.
7. **Rotating backlog**: mechanical pick — sort open admin-list tasks with no due date by `date_updated` ascending, take 3. Locked at fresh-build, not re-rotated mid-week.
8. **Recurrence**: pure Go calendar-date arithmetic with leap-year clamping. Successor creation is idempotent via a `Next instance: <id>` comment on the parent.
9. **Capture (`mw add`)**: `mw add "<text>"` shells out to `claude -p` for one-shot inference; `mw add` (no args) opens an interactive `claude` session for multi-item capture.
10. **Help (`mw chat <id>`)**: opens an interactive `claude` session with the task body pre-loaded.
11. **Digest email**: `mw digest` renders markdown and POSTs directly to `https://api.resend.com/emails`. No piping, no Resend CLI (it doesn't exist).
12. **One cron, state-aware digest**: hourly `mw refresh` also handles the Monday digest send. State (`last_digest_sent: <iso_week>`) lives in `~/.local/state/my-week/<profile>-meta.yml`. Self-correcting on missed runs.
13. **Config moves to `~/.config/my-week/<profile>.yml`** so the repo is shareable. Repo ships `config.example.yml`.
14. **Cache moves to `~/.local/state/my-week/<profile>-<iso-week>.md`** (XDG state dir, not in repo).
15. **Profiles**: one binary `mw -p <profile>`. Patrick handles shell aliases (`mwp`, `mwc`) outside this repo.
16. **Install target**: `Makefile` builds and installs to `/home/patrick/Sync/system_config.arch/bin/mw` (already on `$PATH`).
17. **Email-to-task ingest**: forwarded emails land in a separate `Inbox` ClickUp list (not the curated Admin list). `mw promote <id>` and `mw drop <id>` triage them.
18. **Last-week recap** included at top of fresh-build cache file (and therefore in the Monday digest email). Walks last week's `Overdue`, `This Week`, `Heads Up` sections only — backlog items are skipped per design call. Closed recurring items show `(recurs <interval>)` annotation; no exact next-due date in recap.
19. **Migration**: build new tool, run alongside existing personal-admin for one week, then delete the old skills/scripts.
20. **Tests**: table tests for date math, recurrence successor calculation, cache parser, status mapping. The existing Python had no tests; do not repeat that.

## Project layout

```
my-week/
├── cmd/mw/main.go              # CLI entry point, command dispatch
├── internal/
│   ├── config/                 # load/validate ~/.config/my-week/<profile>.yml
│   ├── cup/                    # subprocess wrapper around `cup` CLI
│   ├── snapshot/               # parse + render the cache file
│   ├── sweep/                  # info-tag sweep + recurrence sweep
│   ├── build/                  # weekly fresh build (rotation, buckets, recap)
│   ├── refresh/                # hourly path (status refresh, new items, sweeps)
│   ├── digest/                 # markdown render + Resend POST
│   ├── recur/                  # calendar-date arithmetic with clamping
│   └── meta/                   # ~/.local/state/my-week/<profile>-meta.yml
├── prompts/
│   ├── add.md                  # one-liner parsing prompt for `mw add "..."`
│   └── chat-context.md         # context loaded for `mw chat <id>`
├── systemd/
│   ├── mw-refresh@.service     # template unit, instantiated per profile
│   └── mw-refresh@.timer       # template timer (hourly, Persistent=true)
├── config.example.yml
├── Makefile
├── go.mod
├── README.md
└── SPEC.md                     # this file
```

`prompts/` is embedded into the binary via `//go:embed`. No external prompt files at runtime.

## Command surface

```
mw                          # cat the cache file (default)
mw refresh                  # do the work; auto-detects daily vs weekly
mw done <id>                # cup update --status complete; also flip [x] in cache
mw snooze <id> <date>       # cup update --due-date <date>; refresh that line
mw add "<text>"             # claude -p one-shot; show inference; confirm; cup create
mw add                      # interactive claude session for bulk capture
mw chat <id>                # interactive claude with task body loaded
mw promote <id> [flags]     # move from Inbox to Admin (or Birthdays) with metadata
mw drop <id>                # delete from Inbox (cup update --status complete is fine)
mw digest                   # render digest markdown to stdout (no email send)
mw send-digest              # render + POST to Resend (used by cron internally)
```

All commands accept `-p <profile>` (default: `personal`).

`mw send-digest` is internal: `mw refresh` calls it after a fresh build when the digest-due conditions are met. It can also be invoked manually if needed.

## Cache file format

Path: `~/.local/state/my-week/<profile>-<iso-week>.md` — e.g. `personal-2026-W18.md`.

```markdown
---
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
- [x] 86b9j9an8 — **Return Simplisoda cylinders**
- [x] 86b9aaxxx — **Replace HVAC filter** (recurs monthly)
- [ ] 86b9bbxxx — **Renew passport** — still open, now 5 days late

## Inbox (2 to triage)
- [ ] 86b9ix0001 — Re: Your Amazon order is delayed (received Mon)
- [ ] 86b9ix0002 — Calendar invite: Q4 planning (received Tue)

## Overdue
- [ ] 86b9ovxx1 — **Schedule oil change** — 3 days late

## This Week
- [ ] 86b9twxx1 — **Renew passport** — due Thu

## Heads Up (next 21 days)
- [ ] 86b9huxx1 — **File state taxes** — due 2026-05-15 (18 days out)

## Info
- 3 awareness items auto-cleared this week.
- Upcoming birthdays: Alice (Apr 30), Bob (May 12).

## Backlog — always-on (High/Urgent)
- [ ] 86b9bk001 — **Get life insurance set up** — High

## Backlog — rotating (locked for this week)
- [ ] 86b9bk010 — **Order laptop chargers**
- [ ] 86b9bk011 — **Tree health check**
- [ ] 86b9bk012 — **Get termite inspection**

## New since snapshot
- [ ] 86b9nz001 — **Return Costco rotisserie** — due Fri

---

Open Claude Code in `my-week/` and run `mw chat <id>` for help on any item.
```

Task line shape is `- [<status>] <clickup_id> — **<name>** — <nudge>` (the trailing `— <nudge>` is optional). The ID is the durable handle for refresh; treat it as case-sensitive opaque text.

Frontmatter fields:
- `profile` — name of the profile.
- `iso_week` — ISO year + week, e.g. `2026-W18`. Used to detect week rollover.
- `generated_at` — when the fresh build wrote this file (preserved on rerun).
- `refreshed_at` — last time `mw refresh` touched it (updated every refresh).
- `swept_on` — last YYYY-MM-DD on which the daily sweeps ran. Used to skip sweeps on same-day refreshes (cron may fire many times per day).

## Meta state file

Path: `~/.local/state/my-week/<profile>-meta.yml`.

```yaml
last_digest_sent: 2026-W18   # ISO week of most recent digest send
```

Survives weekly cache rollover. Add new fields as needed.

## Config file

Path: `~/.config/my-week/<profile>.yml`.

```yaml
profile: personal
cup_profile: personal           # passed as `cup -p <value>`

clickup:
  space_id: "90145142380"
  lists:
    admin: "901415541248"
    birthdays: "901415541256"
    inbox: "<TBD — create new ClickUp list>"

email:
  to: "patrick.schless@gmail.com"
  from: "admin@plainlystated.com"
  subject_prefix: "Personal"

digest:
  send_on: monday               # day-of-week
  send_after: "08:00"           # local time

lookahead_days: 7
lookahead_multiplier: 3         # 21-day scan for needs-lead-time-tagged items

staleness_threshold_minutes: 90
```

Validation: refuse to start if any required field is empty. Specifically: `cup_profile`, `clickup.space_id`, `clickup.lists.admin`, `clickup.lists.birthdays`, `email.to`, `email.from`. Inbox list ID is optional — when missing, `mw refresh` skips the inbox section.

Look at the existing `personal-admin/config/personal.yml` and `personal-admin/config/work.yml` for current values to copy.

## `mw refresh` algorithm

```
1. Load config.
2. Read cache file (if it exists). Parse frontmatter to get iso_week and swept_on.
3. Determine path:
   - cache missing OR iso_week != current_iso_week  →  FRESH_BUILD
   - swept_on != today                              →  REFRESH_WITH_SWEEPS
   - else                                           →  REFRESH_ONLY
4. Run the chosen path (see below).
5. Write the new cache file.
6. If FRESH_BUILD just completed, check digest gate:
     today.weekday >= config.digest.send_on
     AND today.time >= config.digest.send_after
     AND meta.last_digest_sent != current_iso_week
   If all true: render digest, POST to Resend, update meta.last_digest_sent.
```

### FRESH_BUILD

1. Run info-tag sweep (Step 4 of legacy `lib/my-week.md`).
2. Run recurrence sweep (Step 4b).
3. Pull buckets in parallel (Step 5a):
   - Overdue: `cup tasks --all --space <space_id> --due-before <today>`, exclude `info` tag.
   - This Week: `--due-after <today-1> --due-before <week_end+1>`, exclude `info`.
   - Heads Up: `--tag needs-lead-time --due-after <week_end> --due-before <lead_end+1>`.
   - Birthdays: `cup tasks --all --list <birthdays_list_id> --due-after <today-1> --due-before <lead_end+1>`.
   - Backlog: `cup tasks --all --list <admin_list_id>`, filter in-memory to those with no due date. Split: priority Urgent/High → always-on; everything else → sort by `date_updated` ascending, take 3.
   - Inbox: `cup tasks --all --list <inbox_list_id>` (skip if list ID missing).
4. Build last-week recap from `<profile>-<prev-iso-week>.md` if present:
   - Walk Overdue, This Week, Heads Up sections only.
   - For each ID, look up current status from the bulk fetch already in flight.
   - Closed → `[x]`; still-open → `[ ] — still open, now N days late` (if applicable).
   - Closed recurring items → annotate `(recurs <interval>)`, lookup `recur` from custom fields.
5. Render the cache file in the format above.

### REFRESH_WITH_SWEEPS

Same as FRESH_BUILD but skip the rotation pick (preserve the existing locked rotating backlog from the cache). Refresh checkbox states for all known IDs from the bulk fetch. Detect new date-anchored items created since `generated_at` and append to `## New since snapshot`.

### REFRESH_ONLY

Single bulk fetch (`cup tasks --all --space <space_id> --include-closed --json`). Refresh checkbox states. Detect new date-anchored items. No sweeps. Update `refreshed_at`. This is the same shape as the existing `scripts/rerun_refresh.py` and should run in 1–3 seconds.

## Recurrence semantics

Custom field name: `recur`. Values: `daily`, `weekly`, `monthly`, `quarterly`, `semi-annual`, `annual`.

Successor calculation (`internal/recur`):
- `daily` → +1 day.
- `weekly` → +7 days.
- `monthly` → +1 calendar month.
- `quarterly` → +3 calendar months.
- `semi-annual` → +6 calendar months.
- `annual` → +1 calendar year.

For month/quarter/semi-annual/annual: keep the same day-of-month; if target month doesn't have that day (Jan 31 + 1 month, Feb 29 + 1 year in non-leap), clamp to the last day of the target month. This makes annual recurrences drift-free: a Feb 29 birthday rolls to Feb 28 in non-leap years and recovers to Feb 29 the next leap year.

Successor creation:
1. Compute `next_due` from old due date and `recur` value.
2. `cup create -l <list_id> -n "<name>" -d "<description>" --priority <p> --due-date <next_due> --tags <tags>`.
3. `cup field <new_id> --set recur <recur_value>`.
4. `cup comment <old_id> -m "Next instance: <new_id>"` — this comment is the idempotency marker.

Sweep finds completed tasks with non-empty `recur` and no comment starting with `Next instance:`. Idempotent across runs.

## Email digest

`mw digest` renders the cache file's body to markdown (essentially `cat` minus the frontmatter, plus a subject line and footer). `mw send-digest` calls digest internally and POSTs to Resend.

Resend payload shape (lift from `personal-admin/scripts/weekly-digest.sh`):
```json
{
  "from": "<email.from>",
  "to": ["<email.to>"],
  "subject": "<subject_prefix> weekly review — <Mon Apr 27>",
  "text": "<raw markdown>",
  "html": "<pre style=\"font-family:ui-monospace,monospace;white-space:pre-wrap;font-size:14px;line-height:1.5\">...</pre>"
}
```

HTML body is HTML-escaped markdown wrapped in `<pre>`. Preserves whitespace + monospace alignment without bringing in a markdown→HTML library.

Resend API key: `RESEND_API_KEY` env var. The current setup loads it via `mise` from `personal-admin/.mise.local.toml`. Do the same in this repo's `.mise.local.toml` (gitignored). systemd unit should source mise env before running.

Endpoint: `POST https://api.resend.com/emails`, `Authorization: Bearer $RESEND_API_KEY`. Treat 200 and 202 as success.

## `mw add` and `mw chat`

`mw add "<text>"`:
1. Spawn `claude -p` with the embedded `prompts/add.md` template, substituting the user text and the current profile's list/tag/recur conventions.
2. Claude returns structured JSON (use `--output-format json` or instruct in prompt; see Anthropic Go SDK docs if you switch later). Required fields: `name`, `list` (admin|birthdays), `due_date` (YYYY-MM-DD or null), `priority` (urgent|high|normal|low|none), `tags` (array), `recur` (one of the dropdown values or null), `description` (optional).
3. Print the inferred fields. Prompt: `[y]es / [e]dit / [c]ancel`. On edit, drop into a key-value prompt to change individual fields.
4. On `y`: `cup create` + `cup field --set recur ...` if recur is set.

`mw add` (no args): just exec `claude` (interactive). The implementer can pre-load context via a system prompt that explains "you're in bulk capture mode for `<profile>`; user will describe multiple items; for each, infer fields and create via cup."

`mw chat <id>`:
1. `cup task <id> --json` to get the body.
2. Format a context message (use `prompts/chat-context.md` as template).
3. Exec `claude` interactive with the context as the first message.

If `claude -p` startup is too slow for `mw add "<text>"`, swap that one path to direct Anthropic Go SDK calls with Haiku 4.5. The CLI shape doesn't change. Don't preemptively optimize — measure first.

## Inbox / triage

Inbox is a ClickUp list configured by `clickup.lists.inbox`. Patrick sets up Gmail filters that forward selected emails to ClickUp's per-list email-ingest address. ClickUp creates tasks in that list.

`mw refresh` queries the inbox list (no due-date filter, no priority filter — just "all open in inbox") and surfaces them in `## Inbox`. They appear in the cache file and the Monday digest.

`mw promote <id> [--name <new>] [--list admin|birthdays] [--due <YYYY-MM-DD>] [--priority urgent|high|normal|low] [--tags tag1,tag2] [--recur daily|weekly|...]`:
1. `cup task <id> --json` to get current name/description.
2. `cup update <id> --list <new_list_id>` to move (cup may need to re-create; if so, replicate task and delete original).
3. Apply any flag updates.
4. Refresh the cache (incremental — re-pull this task and update relevant sections).

`mw drop <id>`: `cup update <id> --status complete`. (Effectively delete from view; ClickUp keeps history.) Refresh cache line.

If `cup` doesn't support `--list` move directly, fall back to: read full task via `cup task --json`, `cup create` a copy in the target list with same fields, then `cup update <old_id> --status complete`. Test what cup actually does before committing.

## Freshness UX

Every read command (`mw`, `mw promote`, etc.) prints at the top:
```
Refreshed 14m ago.
```

If `(now - refreshed_at) > config.staleness_threshold_minutes`:
- Print `Cache stale, refreshing...`.
- Run `mw refresh` synchronously.
- Then display.

If cache file doesn't exist: print `Building cache, one moment...`, run `mw refresh` (which will take the FRESH_BUILD path), display.

## systemd timer

```ini
# mw-refresh@.service
[Unit]
Description=my-week refresh for profile %i

[Service]
Type=oneshot
ExecStart=/home/patrick/Sync/system_config.arch/bin/mw refresh -p %i
Environment=PATH=/home/patrick/.local/bin:/usr/bin:/usr/local/bin
# ANTHROPIC_API_KEY and RESEND_API_KEY come from mise; either source them here
# or invoke via a wrapper that does `mise env`.
```

```ini
# mw-refresh@.timer
[Unit]
Description=Hourly mw refresh for profile %i

[Timer]
OnCalendar=hourly
Persistent=true
RandomizedDelaySec=120

[Install]
WantedBy=timers.target
```

Enable per-profile:
```
systemctl --user enable --now mw-refresh@personal.timer
systemctl --user enable --now mw-refresh@cjp.timer
```

Persistent=true catches up missed runs after wake-from-sleep, so the first refresh after waking happens within seconds of resume.

## Testing

Table tests are mandatory for:
- `internal/recur` — every recurrence interval, including:
  - Jan 31 + 1 month → Feb 28/29 (test both leap and non-leap years).
  - Feb 29 + 1 year → Feb 28 (non-leap) and Feb 29 (next leap).
  - Mar 31 + 1 month → Apr 30.
  - Dec 31 + 1 quarter → Mar 31.
- `internal/snapshot` — parse + roundtrip a fixture cache file. Round-tripping must preserve everything: every line, every checkbox, every ID, every nudge.
- `internal/refresh` — given a fixture bulk JSON and a fixture cache, verify status flips and new-item detection.
- `internal/build` — given fixture bulk + previous-week cache, verify the recap section renders correctly (closed/carried-over/recurring annotations).
- `cup` subprocess wrapper — fake the binary by setting `MW_CUP_BIN=/path/to/test-cup` env var pointing at a script that emits canned JSON.

`cup tasks --json` shape gotchas (lift from `personal-admin/lib/my-week.md` Step 5b):
- `.status` is a plain string (`"to do"`, `"in progress"`, `"complete"`), NOT an object.
- `.priority` is a plain string (`"urgent"`, `"high"`, `"normal"`, `"low"`, `"none"`).
- `.dueRaw` is a string of milliseconds since epoch. Convert to int before comparison.
- The response does NOT include `tags`, `custom_fields`, or `date_created`. If you need those, separate query.

Done statuses to treat as complete: `complete`, `closed`, `done` (case-insensitive).

## Migration plan

Build new tool in this repo. Wire systemd timers. Run for one full week alongside the existing personal-admin skills.

Comparison check during the week:
- Cache file content matches what `/my-week-personal` would have produced (modulo formatting differences).
- Monday digest email lands as expected.
- `mw add` and `mw chat` are usable.

After one clean week, in `personal-admin/`:
- Delete `lib/my-week.md`, `lib/create-task.md`.
- Delete `scripts/rerun_refresh.py`, `scripts/weekly-digest.sh`.
- Delete `skills/my-week-personal/`, `skills/my-week-cjp/`, `skills/create-task-personal/`, `skills/create-task-work/`.
- Update `personal-admin/CLAUDE.md` to point at the new repo.
- Disable the old crontab entry.
- `lib/knowledge/` stays where it is — it's reference material, separate concern.

## Open work outside this spec

Not implementing in v1; revisit if signal warrants:
- Claude-assisted inbox triage (`mw promote` parses email body for due/priority).
- Opinionated rotating-backlog pick (Claude chooses 3 instead of mechanical sort).
- TUI front-end. Dropped during design — shell commands proved sufficient.
- Switching `mw add "<text>"` from `claude -p` to direct Anthropic Go SDK if startup latency is too high.
- Markdown → richer HTML for digest emails (currently `<pre>`-wrapped; pandoc or templating possible later).

## References

- Existing implementation: `/home/patrick/Sync/ai_workspaces/personal/personal-admin/`
- Source-of-truth for current behavior: `personal-admin/lib/my-week.md`
- Existing fast-rerun reference: `personal-admin/scripts/rerun_refresh.py`
- Resend integration reference: `personal-admin/scripts/weekly-digest.sh`
- Existing config (values to copy): `personal-admin/config/personal.yml`, `personal-admin/config/work.yml`
- Design rationale (older system): `personal-admin/DESIGN.md`, `personal-admin/CLAUDE.md`
