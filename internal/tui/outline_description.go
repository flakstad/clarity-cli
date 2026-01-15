package tui

import (
	"strings"
)

func outlineDescriptionLinesMarkdown(desc string, width int) []string {
	desc = strings.TrimSpace(desc)
	if desc == "" || width <= 0 {
		return nil
	}

	rendered := renderMarkdownComment(desc, width)
	rendered = strings.TrimRight(rendered, "\n")
	if rendered == "" {
		return nil
	}

	lines := strings.Split(rendered, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Ensure markdown styling never bleeds into subsequent lines/rows.
		out = append(out, line+"\x1b[0m")
	}
	return out
}
