You are helping capture a single task for the **{{.Profile}}** profile.

The user typed this one-liner:

```
{{.UserText}}
```

Infer the task's fields and respond with ONLY a JSON object — no commentary, no markdown fences. The JSON must match this schema exactly:

```json
{
  "name": "string — the core 'what' of the task",
  "list": "admin | birthdays",
  "due_date": "YYYY-MM-DD or null",
  "priority": "urgent | high | normal | low | none",
  "tags": ["string", ...],
  "recur": "daily | weekly | monthly | quarterly | semi-annual | annual or null",
  "description": "string — extra context, often empty"
}
```

Rules:
- Default `list` is `admin`. Use `birthdays` only if the input mentions birthday/anniversary/DOB.
- Recognized tags: `info` (awareness only — auto-clears on due date), `needs-lead-time` (include in 21-day lookahead — only for genuinely prep-heavy items). Infer conservatively. Don't invent new tags.
- `recur` triggers: "annual"/"every year"/"yearly", "monthly"/"each month", "quarterly", "biannual"/"semi-annual"/"every 6 months", "weekly", "daily".
- Default `priority` is `none`. Only set if the user signals it.
- For birthdays list: default to `recur: "annual"` and include `info` tag.
- If `recur` is set but the user gave no due date, leave `due_date` null — the user will be asked to fix it.
- December guard: if a computed due date lands in December and isn't externally fixed (tax deadline, renewal, birthday), set `due_date` to null and add a one-line note in `description`.

Today: {{.Today}}
