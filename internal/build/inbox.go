package build

import (
	"fmt"
	"strings"

	"github.com/plainlystated/my-week/internal/cup"
)

// RenderInboxBlock returns the markdown block for the inbox section, ready
// to splice into a cache body. Returns "" when there are no items so callers
// can use it directly with snapshot.SetSection (which treats "" as "remove").
func RenderInboxBlock(items []cup.Task) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Inbox (%d to triage)\n", len(items))
	for _, t := range items {
		fmt.Fprintf(&b, "- [ ] %s — **%s**\n", t.ID, t.Name)
	}
	return b.String()
}
