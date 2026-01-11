package tui

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
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

func renderMarkdownNoMargin(md string, width int) string {
	md = strings.TrimSpace(md)
	if md == "" {
		return ""
	}
	if width < 10 {
		width = 10
	}

	mdRendererMu.Lock()
	styleName := markdownStyle()
	key := styleName + ":nomargin:" + fmtInt(width)
	r := mdRenderers[key]
	mdRendererMu.Unlock()

	if r == nil {
		cfg := markdownStyleConfig(styleName)
		zero := uint(0)
		cfg.Document.Margin = &zero
		rr, err := glamour.NewTermRenderer(
			glamour.WithStyles(cfg),
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

// renderMarkdownCompact renders markdown without block margins. This is intended for
// dense, inline listings (e.g. item detail comment bodies) where vertical spacing
// from paragraph/list margins feels too "airy".
func renderMarkdownCompact(md string, width int) string {
	md = strings.TrimSpace(md)
	if md == "" {
		return ""
	}
	if width < 10 {
		width = 10
	}

	mdRendererMu.Lock()
	styleName := markdownStyle()
	key := styleName + ":compact:" + fmtInt(width)
	r := mdRenderers[key]
	mdRendererMu.Unlock()

	if r == nil {
		cfg := markdownStyleConfig(styleName)
		zero := uint(0)
		// Remove vertical "air" between block elements.
		cfg.Document.Margin = &zero
		cfg.Paragraph.Margin = &zero
		cfg.BlockQuote.Margin = &zero
		cfg.List.Margin = &zero
		cfg.Heading.Margin = &zero
		cfg.H1.Margin = &zero
		cfg.H2.Margin = &zero
		cfg.H3.Margin = &zero
		cfg.H4.Margin = &zero
		cfg.H5.Margin = &zero
		cfg.H6.Margin = &zero
		cfg.Code.Margin = &zero
		cfg.CodeBlock.Margin = &zero
		cfg.Table.Margin = &zero
		cfg.DefinitionList.Margin = &zero
		cfg.HTMLBlock.Margin = &zero
		cfg.HTMLSpan.Margin = &zero

		rr, err := glamour.NewTermRenderer(
			glamour.WithStyles(cfg),
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

func markdownStyleConfig(styleName string) ansi.StyleConfig {
	switch strings.ToLower(strings.TrimSpace(styleName)) {
	case "light":
		cfg := styles.LightStyleConfig
		applyClarityMarkdownPalette(&cfg, "light")
		return cfg
	default:
		cfg := styles.DarkStyleConfig
		applyClarityMarkdownPalette(&cfg, "dark")
		return cfg
	}
}

func markdownStyle() string {
	// Explicit override for debugging / accessibility.
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLARITY_TUI_MD_STYLE"))) {
	case "light":
		return "light"
	case "dark":
		return "dark"
	}
	// Keep markdown styling aligned with the TUI theme preference. Without this,
	// markdown can render with a dark palette even when the TUI is forced to
	// light mode, making description text unreadable on light terminals.
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLARITY_TUI_THEME"))) {
	case "light":
		return "light"
	case "dark":
		return "dark"
	case "auto":
		// fallthrough
	}
	if v := strings.TrimSpace(os.Getenv("CLARITY_TUI_DARKBG")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			if b {
				return "dark"
			}
			return "light"
		}
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
	// Final fallback: align markdown with Lip Gloss's current background detection so
	// description text doesn't end up using a dark palette on light terminals (or vice versa).
	if lipgloss.HasDarkBackground() {
		return "dark"
	}
	return "light"
}

func applyClarityMarkdownPalette(cfg *ansi.StyleConfig, styleName string) {
	if cfg == nil {
		return
	}

	// Headings: keep them high-contrast and aligned with the normal text palette
	// (no bright blue).
	headingColor := mdColor(colorSurfaceFg, styleName)
	cfg.Heading.Color = headingColor
	cfg.H1.Color = headingColor
	cfg.H2.Color = headingColor
	cfg.H3.Color = headingColor
	cfg.H4.Color = headingColor
	cfg.H5.Color = headingColor
	cfg.H6.Color = headingColor

	// Links: avoid red; use Clarity accent with underline.
	linkColor := mdColor(colorAccent, styleName)
	cfg.Link.Color = linkColor
	cfg.Link.Underline = mdBoolPtr(true)
	cfg.LinkText.Color = linkColor
	cfg.LinkText.Underline = mdBoolPtr(true)

	// Inline code: avoid bright red; keep readable and distinct.
	cfg.Code.Color = mdColor(colorSurfaceFg, styleName)
	cfg.CodeBlock.Color = mdColor(colorSurfaceFg, styleName)
	if cfg.CodeBlock.BackgroundColor == nil {
		cfg.CodeBlock.BackgroundColor = mdColor(colorControlBg, styleName)
	}

	// Ensure base text remains aligned with the surface foreground.
	cfg.Text.Color = mdColor(colorSurfaceFg, styleName)

	// This also makes emphasis/strong inherit the base text color, preventing
	// surprising “keyword” colors in some styles.
	cfg.Strong.Color = nil
	cfg.Emph.Color = nil

	// Avoid faint markdown elements becoming too hard to read.
	// (Some default styles use faint for blockquotes and similar.)
	cfg.BlockQuote.Faint = mdBoolPtr(false)
}

func mdColor(c lipgloss.AdaptiveColor, styleName string) *string {
	if strings.TrimSpace(strings.ToLower(styleName)) == "light" {
		return mdStrPtr(c.Light)
	}
	return mdStrPtr(c.Dark)
}

func mdStrPtr(s string) *string { return &s }
func mdBoolPtr(b bool) *bool    { return &b }

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
