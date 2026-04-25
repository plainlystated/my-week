// Package digest renders the weekly digest from the cache and POSTs it to Resend.
package digest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/plainlystated/my-week/internal/config"
	"github.com/plainlystated/my-week/internal/snapshot"
)

const resendEndpoint = "https://api.resend.com/emails"

// Render returns the markdown body for the digest. Currently identical to the
// cache body — the body already includes the H1 title and footer.
func Render(snap *snapshot.Snapshot) string {
	return strings.TrimLeft(snap.Body, "\n")
}

// Subject returns "<prefix> weekly review — Mon Apr 27".
func Subject(cfg *config.Config, monday time.Time) string {
	day := monday.Format("Mon Jan 2")
	if cfg.Email.SubjectPrefix == "" {
		return "Weekly review — " + day
	}
	return cfg.Email.SubjectPrefix + " weekly review — " + day
}

// Send posts the digest to Resend. Returns nil on 200/202.
func Send(cfg *config.Config, subject, markdown string) error {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		return errors.New("RESEND_API_KEY is not set")
	}

	payload := map[string]any{
		"from":    cfg.Email.From,
		"to":      []string{cfg.Email.To},
		"subject": subject,
		"text":    markdown,
		"html":    htmlWrap(markdown),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, resendEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

// htmlWrap escapes the markdown and wraps it in a <pre> block so Resend
// renders it with monospace + preserved whitespace, no markdown library needed.
func htmlWrap(markdown string) string {
	return `<pre style="font-family:ui-monospace,monospace;white-space:pre-wrap;font-size:14px;line-height:1.5">` +
		html.EscapeString(markdown) +
		`</pre>`
}
