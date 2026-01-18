package tui

import (
	"os"
	"strings"
	"sync"
)

// Terminal apps can't change the user's actual font. Instead, we can choose
// between Unicode and ASCII glyph sets for UI affordances (twisties, separators,
// arrows). This helps on terminals/fonts that don't render some glyphs cleanly.

type glyphSet int

const (
	glyphSetUnicode glyphSet = iota
	glyphSetASCII
)

var (
	glyphsMu      sync.RWMutex
	currentGlyphs = glyphSetUnicode
)

func applyGlyphPreference() {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CLARITY_TUI_GLYPHS")))
	switch v {
	case "", "unicode", "utf8":
		setGlyphs(glyphSetUnicode)
	case "ascii":
		setGlyphs(glyphSetASCII)
	default:
		// Unknown value: ignore.
	}
}

func setGlyphs(gs glyphSet) {
	glyphsMu.Lock()
	currentGlyphs = gs
	glyphsMu.Unlock()
}

func glyphs() glyphSet {
	glyphsMu.RLock()
	gs := currentGlyphs
	glyphsMu.RUnlock()
	return gs
}

func glyphsName(gs glyphSet) string {
	switch gs {
	case glyphSetASCII:
		return "ASCII"
	default:
		return "Unicode"
	}
}

func glyphTwistyCollapsed() string {
	if glyphs() == glyphSetASCII {
		return ">"
	}
	return "▸"
}

func glyphTwistyExpanded() string {
	if glyphs() == glyphSetASCII {
		return "v"
	}
	return "▾"
}

func glyphBullet() string {
	if glyphs() == glyphSetASCII {
		return "*"
	}
	return "•"
}

func glyphArrow() string {
	if glyphs() == glyphSetASCII {
		return "->"
	}
	return "→"
}

func glyphHRule() string {
	if glyphs() == glyphSetASCII {
		return "-"
	}
	return "─"
}
