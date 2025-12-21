package tui

import (
        "strings"

        "github.com/charmbracelet/glamour"
)

func renderMarkdown(md string, width int) string {
        md = strings.TrimSpace(md)
        if md == "" {
                return ""
        }
        if width < 10 {
                width = 10
        }

        r, err := glamour.NewTermRenderer(
                glamour.WithAutoStyle(),
                glamour.WithWordWrap(width),
        )
        if err != nil {
                return md
        }
        out, err := r.Render(md)
        if err != nil {
                return md
        }
        return strings.TrimRight(out, "\n")
}
