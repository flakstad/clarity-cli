package tui

import (
        "os"
        "strings"
        "sync"

        "github.com/charmbracelet/glamour"
)

var (
        mdRendererMu sync.Mutex
        // Cache renderers by wrap width + style. Creating a renderer with WithAutoStyle can trigger
        // terminal capability/background queries that may block on some terminals.
        // Using a fixed style + caching keeps split-preview rendering fast and predictable.
        mdRenderers = map[string]*glamour.TermRenderer{}
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
        style := markdownStyle()
        key := style + ":" + fmtInt(width)
        r := mdRenderers[key]
        mdRendererMu.Unlock()

        if r == nil {
                rr, err := glamour.NewTermRenderer(
                        // Avoid WithAutoStyle() here: it can block waiting on terminal queries in some setups.
                        glamour.WithStandardStyle(style),
                        glamour.WithWordWrap(width),
                )
                if err != nil {
                        return md
                }
                mdRendererMu.Lock()
                // Re-check in case a concurrent goroutine filled it.
                if existing := mdRenderers[key]; existing != nil {
                        r = existing
                } else {
                        mdRenderers[key] = rr
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

func markdownStyle() string {
        // Explicit override for debugging / accessibility.
        switch strings.ToLower(strings.TrimSpace(os.Getenv("CLARITY_TUI_MD_STYLE"))) {
        case "light":
                return "light"
        case "dark":
                return "dark"
        }
        // Heuristic: COLORFGBG is often "fg;bg" (e.g. "15;0" => dark bg).
        // Prefer this over term queries to avoid blocking.
        if v := strings.TrimSpace(os.Getenv("COLORFGBG")); v != "" {
                parts := strings.Split(v, ";")
                bgS := strings.TrimSpace(parts[len(parts)-1])
                bg := atoi(bgS)
                // Common xterm palette: 0-6 dark colors, 7-15 light colors.
                if bg >= 7 {
                        return "light"
                }
                if bg >= 0 {
                        return "dark"
                }
        }
        return "dark"
}

func atoi(s string) int {
        s = strings.TrimSpace(s)
        if s == "" {
                return -1
        }
        n := 0
        for _, r := range s {
                if r < '0' || r > '9' {
                        return -1
                }
                n = n*10 + int(r-'0')
        }
        return n
}

func fmtInt(n int) string {
        // tiny helper to avoid strconv import here
        if n == 0 {
                return "0"
        }
        neg := false
        if n < 0 {
                neg = true
                n = -n
        }
        var buf [32]byte
        i := len(buf)
        for n > 0 {
                i--
                buf[i] = byte('0' + (n % 10))
                n /= 10
        }
        if neg {
                i--
                buf[i] = '-'
        }
        return string(buf[i:])
}
