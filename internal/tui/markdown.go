package tui

import (
        "strings"
        "sync"

        "github.com/charmbracelet/glamour"
)

var (
        mdRendererMu sync.Mutex
        // Cache renderers by wrap width. Creating a renderer with WithAutoStyle can trigger
        // terminal capability/background queries that may block on some terminals.
        // Using a fixed style + caching keeps split-preview rendering fast and predictable.
        mdRenderers = map[int]*glamour.TermRenderer{}
)

func renderMarkdown(md string, width int) string {
        md = strings.TrimSpace(md)
        if md == "" {
                return ""
        }
        if width < 10 {
                width = 10
        }

        mdRendererMu.Lock()
        r := mdRenderers[width]
        mdRendererMu.Unlock()

        if r == nil {
                rr, err := glamour.NewTermRenderer(
                        // Avoid WithAutoStyle() here: it can block waiting on terminal queries in some setups.
                        glamour.WithStandardStyle("dark"),
                        glamour.WithWordWrap(width),
                )
                if err != nil {
                        return md
                }
                mdRendererMu.Lock()
                // Re-check in case a concurrent goroutine filled it.
                if existing := mdRenderers[width]; existing != nil {
                        r = existing
                } else {
                        mdRenderers[width] = rr
                        r = rr
                }
                mdRendererMu.Unlock()
        }

        out, err := r.Render(md)
        if err != nil {
                return md
        }
        return strings.TrimRight(out, "\n")
}
