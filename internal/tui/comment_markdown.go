package tui

import (
	"fmt"
	"strings"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func commentMarkdownWithAttachments(db *store.DB, c model.Comment) string {
	body := strings.TrimSpace(c.Body)
	if db == nil {
		return body
	}

	att := db.AttachmentsForComment(strings.TrimSpace(c.ID))
	if len(att) == 0 {
		return body
	}

	lines := []string{}
	if body != "" {
		lines = append(lines, body, "")
	}
	lines = append(lines, "Attachments:")
	for _, a := range att {
		name := strings.TrimSpace(a.Title)
		if name == "" {
			name = strings.TrimSpace(a.OriginalName)
		}
		if name == "" {
			name = strings.TrimSpace(a.ID)
		}
		id := strings.TrimSpace(a.ID)
		alt := strings.TrimSpace(a.Alt)
		if alt != "" {
			lines = append(lines, fmt.Sprintf("- %s — %s (%s)", name, alt, id))
		} else {
			lines = append(lines, fmt.Sprintf("- %s (%s)", name, id))
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
