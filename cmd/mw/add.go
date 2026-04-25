package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/plainlystated/my-week/internal/config"
	"github.com/plainlystated/my-week/internal/cup"
	"github.com/plainlystated/my-week/prompts"
)

// taskDraft is the schema we expect Claude to return for `mw add "..."`.
type taskDraft struct {
	Name        string   `json:"name"`
	List        string   `json:"list"`
	DueDate     *string  `json:"due_date"`
	Priority    string   `json:"priority"`
	Tags        []string `json:"tags"`
	Recur       *string  `json:"recur"`
	Description string   `json:"description"`
}

// cmdAdd dispatches between one-shot (text given) and bulk (no args) capture.
func cmdAdd(profile string, args []string) error {
	cfg, err := config.Load(profile)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return runBulkCapture(cfg)
	}
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		return runBulkCapture(cfg)
	}
	return runOneShotAdd(cfg, text)
}

// runOneShotAdd: parse user text via `claude -p`, confirm with user, create.
func runOneShotAdd(cfg *config.Config, text string) error {
	prompt, err := prompts.RenderAdd(prompts.AddVars{
		Profile:  cfg.Profile,
		UserText: text,
		Today:    time.Now().Format("2006-01-02"),
	})
	if err != nil {
		return err
	}

	raw, err := claudeOneShot(prompt)
	if err != nil {
		return fmt.Errorf("claude: %w", err)
	}
	jsonText := stripFences(raw)

	var draft taskDraft
	if err := json.Unmarshal([]byte(jsonText), &draft); err != nil {
		return fmt.Errorf("decode claude output: %w\nraw:\n%s", err, raw)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		printDraft(draft)
		fmt.Print("Create? [y]es / [e]dit / [c]ancel: ")
		ans, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		switch strings.TrimSpace(strings.ToLower(ans)) {
		case "y", "yes":
			return createDraft(cfg, draft)
		case "c", "cancel", "n", "no":
			fmt.Println("Cancelled.")
			return nil
		case "e", "edit":
			if err := editDraft(reader, &draft); err != nil {
				return err
			}
			// Loop to re-show + reconfirm.
		default:
			fmt.Println("Type y, e, or c.")
		}
	}
}

// runBulkCapture just execs `claude` interactively. The user has full Claude
// Code at their disposal; cup is a tool Claude can drive via Bash.
func runBulkCapture(cfg *config.Config) error {
	systemNote := fmt.Sprintf(
		"You are in bulk-capture mode for the %q profile of my-week. "+
			"The user will describe one or more tasks. For each, infer the fields "+
			"(name, list=admin|birthdays, due date, priority, tags, recur, description) "+
			"and create the task with `cup -p %s create -l <list_id> ...` followed by "+
			"`cup -p %s field <new_id> --set recur <value>` if recur is set. "+
			"Confirm with the user before each create. The admin list ID is %q and "+
			"the birthdays list ID is %q.",
		cfg.Profile, cfg.CupProfile, cfg.CupProfile,
		cfg.ClickUp.Lists.Admin, cfg.ClickUp.Lists.Birthdays,
	)
	cmd := exec.Command("claude", "--append-system-prompt", systemNote)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// claudeOneShot runs `claude -p <prompt>` and returns trimmed stdout.
func claudeOneShot(prompt string) (string, error) {
	cmd := exec.Command("claude", "-p", prompt)
	out, err := cmd.Output()
	if err != nil {
		// surface stderr if available
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// stripFences removes ```json ... ``` markdown fences if Claude wrapped the JSON.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func printDraft(d taskDraft) {
	fmt.Println()
	fmt.Println("Inferred:")
	fmt.Printf("  name:        %s\n", d.Name)
	fmt.Printf("  list:        %s\n", d.List)
	fmt.Printf("  due_date:    %s\n", strOrNone(d.DueDate))
	fmt.Printf("  priority:    %s\n", d.Priority)
	fmt.Printf("  tags:        %s\n", strings.Join(d.Tags, ", "))
	fmt.Printf("  recur:       %s\n", strOrNone(d.Recur))
	fmt.Printf("  description: %s\n", oneLine(d.Description))
}

func strOrNone(p *string) string {
	if p == nil || *p == "" {
		return "(none)"
	}
	return *p
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if s == "" {
		return "(none)"
	}
	return s
}

// editDraft: simple field-by-field prompt. Blank input keeps current value.
func editDraft(reader *bufio.Reader, d *taskDraft) error {
	fields := []struct {
		name string
		get  func() string
		set  func(string)
	}{
		{"name", func() string { return d.Name }, func(s string) { d.Name = s }},
		{"list", func() string { return d.List }, func(s string) { d.List = s }},
		{"due_date", func() string { return strOrNone(d.DueDate) }, func(s string) {
			if s == "none" || s == "" {
				d.DueDate = nil
			} else {
				d.DueDate = &s
			}
		}},
		{"priority", func() string { return d.Priority }, func(s string) { d.Priority = s }},
		{"tags", func() string { return strings.Join(d.Tags, ",") }, func(s string) {
			if s == "" {
				d.Tags = nil
			} else {
				d.Tags = strings.Split(s, ",")
			}
		}},
		{"recur", func() string { return strOrNone(d.Recur) }, func(s string) {
			if s == "none" || s == "" {
				d.Recur = nil
			} else {
				d.Recur = &s
			}
		}},
		{"description", func() string { return d.Description }, func(s string) { d.Description = s }},
	}
	fmt.Println("\nEdit (press Enter to keep current value; type 'none' to clear nullable fields):")
	for _, f := range fields {
		fmt.Printf("  %s [%s]: ", f.name, f.get())
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f.set(line)
	}
	return nil
}

// createDraft: cup create + cup field --set recur if needed.
func createDraft(cfg *config.Config, d taskDraft) error {
	listID := ""
	switch strings.ToLower(d.List) {
	case "admin", "":
		listID = cfg.ClickUp.Lists.Admin
	case "birthdays":
		listID = cfg.ClickUp.Lists.Birthdays
	default:
		return fmt.Errorf("invalid list %q (expected admin|birthdays)", d.List)
	}
	if listID == "" {
		return fmt.Errorf("config missing list ID for %s", d.List)
	}

	due := ""
	if d.DueDate != nil {
		due = *d.DueDate
	}

	client := cup.New(cfg.CupProfile)
	id, err := client.Create(cup.CreateOpts{
		ListID:      listID,
		Name:        d.Name,
		Description: d.Description,
		DueDate:     due,
		Priority:    d.Priority,
		Tags:        d.Tags,
	})
	if err != nil {
		return fmt.Errorf("cup create: %w", err)
	}
	if d.Recur != nil && *d.Recur != "" {
		if err := client.SetField(id, "recur", *d.Recur); err != nil {
			fmt.Fprintf(os.Stderr, "warn: created %s but failed to set recur=%s: %v\n", id, *d.Recur, err)
		}
	}
	fmt.Printf("Created %s — https://app.clickup.com/t/%s\n", id, id)
	return nil
}
