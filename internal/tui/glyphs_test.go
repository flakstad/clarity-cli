package tui

import "testing"

func TestGlyphs_FromEnv(t *testing.T) {
	t.Setenv("CLARITY_TUI_GLYPHS", "")
	setGlyphs(glyphSetUnicode)
	applyGlyphPreference()
	if got := glyphs(); got != glyphSetUnicode {
		t.Fatalf("expected unicode glyphs by default; got %v", got)
	}

	t.Setenv("CLARITY_TUI_GLYPHS", "ascii")
	applyGlyphPreference()
	if got := glyphs(); got != glyphSetASCII {
		t.Fatalf("expected ascii glyphs; got %v", got)
	}

	t.Setenv("CLARITY_TUI_GLYPHS", "unicode")
	applyGlyphPreference()
	if got := glyphs(); got != glyphSetUnicode {
		t.Fatalf("expected unicode glyphs; got %v", got)
	}

	// Unknown values should be ignored (keep current).
	setGlyphs(glyphSetASCII)
	t.Setenv("CLARITY_TUI_GLYPHS", "bogus")
	applyGlyphPreference()
	if got := glyphs(); got != glyphSetASCII {
		t.Fatalf("expected unknown to be ignored; got %v", got)
	}
}
