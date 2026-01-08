package tui

import (
	"fmt"
	"strings"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	xansi "github.com/charmbracelet/x/ansi"
)

func commentThreadPreviewLine(db *store.DB, r commentThreadRow, width int, depthOffset int) string {
	depth := r.Depth + depthOffset
	if depth < 0 {
		depth = 0
	}
	if depth > 12 {
		depth = 12
	}
	indent := strings.Repeat("  ", depth)
	prefix := ""
	if depth > 0 {
		prefix = "↳ "
	}
	actor := actorLabel(db, r.Comment.AuthorID)
	snippetMax := maxInt(20, width-26-(2*depth))
	snippet := truncateInline(r.Comment.Body, snippetMax)
	line := fmt.Sprintf("%s%s%s  %s  %s", indent, prefix, fmtTS(r.Comment.CreatedAt), actor, snippet)
	return fixedWidthLine(line, width)
}

func commentThreadPreviewLines(db *store.DB, comments []model.Comment, width int, depthOffset int, maxRows int) []string {
	rows := buildCommentThreadRows(comments)
	if len(rows) == 0 || width <= 0 {
		return nil
	}

	trimmed := rows
	more := 0
	if maxRows > 0 && len(trimmed) > maxRows {
		trimmed = trimmed[:maxRows]
		more = len(rows) - maxRows
	}

	out := make([]string, 0, len(trimmed)+1)
	for _, r := range trimmed {
		out = append(out, commentThreadPreviewLine(db, r, width, depthOffset))
	}
	if more > 0 {
		out = append(out, fmt.Sprintf("… %d more comments", more))
	}
	return out
}

func fixedWidthLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if xansi.StringWidth(s) > width {
		// Ensure any open ANSI styling is terminated.
		return xansi.Cut(s, 0, width) + "\x1b[0m"
	}
	if pad := width - xansi.StringWidth(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}
