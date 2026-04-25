// Package prompts holds the embedded prompt templates used by `mw add` and
// `mw chat`. Files live under repo-root/prompts and are baked in at build time.
package prompts

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed add.md
var addRaw string

//go:embed chat-context.md
var chatRaw string

// AddVars is the substitution set for the add prompt.
type AddVars struct {
	Profile  string
	UserText string
	Today    string
}

// ChatVars is the substitution set for the chat-context prompt.
type ChatVars struct {
	Profile     string
	ID          string
	Name        string
	Status      string
	Priority    string
	DueDate     string
	ListName    string
	Tags        string
	Description string
}

// RenderAdd substitutes vars into the add prompt template.
func RenderAdd(v AddVars) (string, error) { return render(addRaw, v) }

// RenderChat substitutes vars into the chat-context template.
func RenderChat(v ChatVars) (string, error) { return render(chatRaw, v) }

func render(tmpl string, v any) (string, error) {
	t, err := template.New("p").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, v); err != nil {
		return "", err
	}
	return buf.String(), nil
}
