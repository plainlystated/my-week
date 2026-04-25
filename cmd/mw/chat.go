package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/plainlystated/my-week/internal/config"
	"github.com/plainlystated/my-week/internal/cup"
	"github.com/plainlystated/my-week/prompts"
)

// cmdChat opens an interactive Claude session pre-loaded with the task body.
func cmdChat(profile string, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: mw chat <id-or-suffix>")
	}
	id, err := resolveID(profile, args[0])
	if err != nil {
		return err
	}
	cfg, err := config.Load(profile)
	if err != nil {
		return err
	}
	full, err := cup.New(cfg.CupProfile).TaskGet(id)
	if err != nil {
		return err
	}
	dueText := "(none)"
	if d := full.DueTime(); !d.IsZero() {
		dueText = d.Format("2006-01-02")
	}
	priority := full.PriorityName()
	if priority == "" {
		priority = "(none)"
	}
	tags := strings.Join(full.TagNames(), ", ")
	if tags == "" {
		tags = "(none)"
	}
	desc := strings.TrimSpace(full.Description)
	if desc == "" {
		desc = "(none)"
	}

	prompt, err := prompts.RenderChat(prompts.ChatVars{
		Profile:     cfg.Profile,
		ID:          full.ID,
		Name:        full.Name,
		Status:      full.Status.Status,
		Priority:    priority,
		DueDate:     dueText,
		ListName:    full.List.Name,
		Tags:        tags,
		Description: desc,
	})
	if err != nil {
		return err
	}

	cmd := exec.Command("claude", prompt)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
