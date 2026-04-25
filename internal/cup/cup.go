// Package cup is a thin subprocess wrapper around the `cup` ClickUp CLI.
//
// The binary path can be overridden via MW_CUP_BIN — used by tests to point at
// a fake script that emits canned JSON.
package cup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Client wraps the cup CLI for a single profile.
type Client struct {
	Binary  string
	Profile string
}

// New returns a Client for the given cup profile.
func New(profile string) *Client {
	bin := os.Getenv("MW_CUP_BIN")
	if bin == "" {
		bin = "cup"
	}
	return &Client{Binary: bin, Profile: profile}
}

// Task is the slim shape returned by `cup tasks --json` (a flat array).
//
// Notes from the spec:
//   - Status is a plain string ("to do", "in progress", "complete"), NOT an object.
//   - Priority is a plain string ("urgent", "high", "normal", "low", "none").
//   - DueRaw is a string of milliseconds since epoch.
//   - The bulk shape does NOT include tags, custom_fields, or date_created.
type Task struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	TaskType string `json:"task_type"`
	Priority string `json:"priority"`
	DueDate  string `json:"due_date"`
	DueRaw   string `json:"dueRaw"`
	List     string `json:"list"`
	URL      string `json:"url"`
}

// DueTime parses DueRaw (milliseconds since epoch) as a time.Time. Returns
// zero time if DueRaw is empty or unparseable.
func (t Task) DueTime() time.Time {
	if t.DueRaw == "" {
		return time.Time{}
	}
	var ms int64
	if _, err := fmt.Sscan(t.DueRaw, &ms); err != nil {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// IsDone reports whether the task is in a complete/closed/done status.
func (t Task) IsDone() bool {
	switch strings.ToLower(t.Status) {
	case "complete", "closed", "done":
		return true
	}
	return false
}

// TaskFull is the shape returned by `cup task <id> --json` — the raw ClickUp
// API shape, with status/priority/list/tags/custom_fields all as objects.
//
// Use the helper methods (IsDone, ListID, TagNames, PriorityName, RecurValue,
// DueTime) instead of poking at the struct fields directly.
type TaskFull struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Status      struct {
		Status string `json:"status"`
		Type   string `json:"type"`
	} `json:"status"`
	Priority *struct {
		Priority string `json:"priority"`
	} `json:"priority"`
	DueDate string `json:"due_date"` // ms since epoch as string
	List    struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"list"`
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
	CustomFields []CustomField `json:"custom_fields"`
	DateUpdated  string        `json:"date_updated"`
	DateCreated  string        `json:"date_created"`
	DateClosed   string        `json:"date_closed"`

	Raw json.RawMessage `json:"-"`
}

// IsDone reports whether the task is in a complete/closed/done status.
func (t *TaskFull) IsDone() bool {
	switch strings.ToLower(t.Status.Status) {
	case "complete", "closed", "done":
		return true
	}
	return false
}

// PriorityName returns the priority name ("urgent","high","normal","low") or "" if unset.
func (t *TaskFull) PriorityName() string {
	if t.Priority == nil {
		return ""
	}
	return t.Priority.Priority
}

// ListID returns the parent list's ID.
func (t *TaskFull) ListID() string { return t.List.ID }

// TagNames returns the tag names attached to the task.
func (t *TaskFull) TagNames() []string {
	out := make([]string, 0, len(t.Tags))
	for _, tg := range t.Tags {
		out = append(out, tg.Name)
	}
	return out
}

// DueTime parses DueDate (milliseconds since epoch) as a time.Time.
func (t *TaskFull) DueTime() time.Time {
	if t.DueDate == "" {
		return time.Time{}
	}
	var ms int64
	if _, err := fmt.Sscan(t.DueDate, &ms); err != nil {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// RecurValue resolves the "recur" custom field to its human-readable name
// (e.g. "annual"). Returns "" if unset or unrecognized.
func (t *TaskFull) RecurValue() string {
	for _, f := range t.CustomFields {
		if strings.EqualFold(f.Name, "recur") {
			return f.ResolvedValue()
		}
	}
	return ""
}

// CustomField is a single ClickUp custom-field definition + (optional) value.
//
// For drop_down fields the .Value is the orderindex of the selected option;
// resolve to the option name via type_config.options. Use ResolvedValue.
type CustomField struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Value      any             `json:"value"`
	TypeConfig customFieldType `json:"type_config"`
}

type customFieldType struct {
	Options []customFieldOption `json:"options"`
}

type customFieldOption struct {
	Name       string `json:"name"`
	Orderindex int    `json:"orderindex"`
}

// ResolvedValue returns the human-readable value of the field. For drop_down
// fields, that means looking up the option name by orderindex. For other
// types, returns the stringified raw value.
func (f CustomField) ResolvedValue() string {
	if f.Value == nil {
		return ""
	}
	if f.Type == "drop_down" {
		idx, ok := toInt(f.Value)
		if !ok {
			return ""
		}
		for _, o := range f.TypeConfig.Options {
			if o.Orderindex == idx {
				return o.Name
			}
		}
		return ""
	}
	return fmt.Sprint(f.Value)
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	case int64:
		return int(x), true
	case string:
		var n int
		if _, err := fmt.Sscan(x, &n); err == nil {
			return n, true
		}
	}
	return 0, false
}

// Comment is one task comment.
type Comment struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// TaskQuery is the set of filters supported by `cup tasks`.
type TaskQuery struct {
	All           bool
	IncludeClosed bool
	SpaceID       string
	ListID        string
	Tag           string
	DueBefore     time.Time
	DueAfter      time.Time
	CreatedAfter  time.Time
}

// Tasks runs `cup tasks --json` with the given filters.
func (c *Client) Tasks(q TaskQuery) ([]Task, error) {
	args := []string{"tasks", "--json"}
	if q.All {
		args = append(args, "--all")
	}
	if q.IncludeClosed {
		args = append(args, "--include-closed")
	}
	if q.SpaceID != "" {
		args = append(args, "--space", q.SpaceID)
	}
	if q.ListID != "" {
		args = append(args, "--list", q.ListID)
	}
	if q.Tag != "" {
		args = append(args, "--tag", q.Tag)
	}
	if !q.DueBefore.IsZero() {
		args = append(args, "--due-before", q.DueBefore.Format("2006-01-02"))
	}
	if !q.DueAfter.IsZero() {
		args = append(args, "--due-after", q.DueAfter.Format("2006-01-02"))
	}
	if !q.CreatedAfter.IsZero() {
		args = append(args, "--created-after", q.CreatedAfter.Format("2006-01-02"))
	}
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, nil
	}
	var tasks []Task
	if err := json.Unmarshal(out, &tasks); err != nil {
		return nil, fmt.Errorf("decoding cup tasks: %w (head: %s)", err, head(out))
	}
	return tasks, nil
}

// TaskGet runs `cup task <id> --json` for a single task with full detail.
func (c *Client) TaskGet(id string) (*TaskFull, error) {
	out, err := c.run("task", id, "--json")
	if err != nil {
		return nil, err
	}
	var t TaskFull
	if err := json.Unmarshal(out, &t); err != nil {
		return nil, fmt.Errorf("decoding cup task %s: %w (head: %s)", id, err, head(out))
	}
	t.Raw = append([]byte(nil), out...)
	return &t, nil
}

// Comments runs `cup comments <id> --json`.
func (c *Client) Comments(id string) ([]Comment, error) {
	out, err := c.run("comments", id, "--json")
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, nil
	}
	var comments []Comment
	if err := json.Unmarshal(out, &comments); err != nil {
		return nil, fmt.Errorf("decoding cup comments %s: %w (head: %s)", id, err, head(out))
	}
	return comments, nil
}

// HasNextInstanceComment reports whether the task has an idempotency-marker
// comment ("Next instance: ...") from a prior recurrence sweep.
func (c *Client) HasNextInstanceComment(id string) (bool, error) {
	comments, err := c.Comments(id)
	if err != nil {
		return false, err
	}
	for _, cm := range comments {
		if strings.HasPrefix(strings.TrimSpace(cm.Text), "Next instance:") {
			return true, nil
		}
	}
	return false, nil
}

// UpdateOpts captures the subset of `cup update` flags we use.
type UpdateOpts struct {
	Status      string // fuzzy-matched by cup
	DueDate     string // YYYY-MM-DD, "none" or "clear" to remove
	Description string
	Priority    string
}

// Update runs `cup update <id>` with the given flags.
func (c *Client) Update(id string, o UpdateOpts) error {
	args := []string{"update", id}
	if o.Status != "" {
		args = append(args, "--status", o.Status)
	}
	if o.DueDate != "" {
		args = append(args, "--due-date", o.DueDate)
	}
	if o.Description != "" {
		args = append(args, "--description", o.Description)
	}
	if o.Priority != "" {
		args = append(args, "--priority", o.Priority)
	}
	_, err := c.run(args...)
	return err
}

// CreateOpts captures the subset of `cup create` flags we use.
type CreateOpts struct {
	ListID      string
	Name        string
	Description string
	DueDate     string // YYYY-MM-DD
	Priority    string
	Tags        []string
}

// Create runs `cup create --json` and returns the new task ID.
func (c *Client) Create(o CreateOpts) (string, error) {
	args := []string{"create", "--json", "-l", o.ListID, "-n", o.Name}
	if o.Description != "" {
		args = append(args, "-d", o.Description)
	}
	if o.DueDate != "" {
		args = append(args, "--due-date", o.DueDate)
	}
	if o.Priority != "" && o.Priority != "normal" {
		args = append(args, "--priority", o.Priority)
	}
	if len(o.Tags) > 0 {
		args = append(args, "--tags", strings.Join(o.Tags, ","))
	}
	out, err := c.run(args...)
	if err != nil {
		return "", err
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("decoding cup create: %w (head: %s)", err, head(out))
	}
	return resp.ID, nil
}

// SetField runs `cup field <id> --set "<name>" <value>`.
func (c *Client) SetField(id, name, value string) error {
	_, err := c.run("field", id, "--set", name, value)
	return err
}

// PostComment runs `cup comment <id> -m <message>`.
func (c *Client) PostComment(id, message string) error {
	_, err := c.run("comment", id, "-m", message)
	return err
}

// Move runs `cup move <id> --to <listID>`.
func (c *Client) Move(id, toListID string) error {
	_, err := c.run("move", id, "--to", toListID)
	return err
}

func (c *Client) run(args ...string) ([]byte, error) {
	full := append([]string{"-p", c.Profile}, args...)
	cmd := exec.Command(c.Binary, full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cup %s: %w: %s", strings.Join(full, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func head(b []byte) string {
	const max = 200
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
