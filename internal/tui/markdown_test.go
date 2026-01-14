package tui

import (
	"testing"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

func TestMarkdownStyle_RespectsTUITheme(t *testing.T) {
	t.Setenv("CLARITY_TUI_MD_STYLE", "")
	t.Setenv("COLORFGBG", "")
	t.Setenv("CLARITY_TUI_DARKBG", "")

	t.Setenv("CLARITY_TUI_THEME", "light")
	if got := markdownStyle(); got != "light" {
		t.Fatalf("expected light; got %q", got)
	}

	t.Setenv("CLARITY_TUI_THEME", "dark")
	if got := markdownStyle(); got != "dark" {
		t.Fatalf("expected dark; got %q", got)
	}
}

func TestMarkdownStyle_MDStyleOverridesTheme(t *testing.T) {
	t.Setenv("COLORFGBG", "")
	t.Setenv("CLARITY_TUI_DARKBG", "")
	t.Setenv("CLARITY_TUI_THEME", "light")

	t.Setenv("CLARITY_TUI_MD_STYLE", "dark")
	if got := markdownStyle(); got != "dark" {
		t.Fatalf("expected dark; got %q", got)
	}
}

func TestMarkdownStyleConfig_DoesNotOverrideLinkStyles(t *testing.T) {
	t.Run("dark", func(t *testing.T) {
		got := markdownStyleConfig("dark")
		want := styles.DarkStyleConfig
		assertStylePrimitiveEqual(t, got.Link, want.Link)
		assertStylePrimitiveEqual(t, got.LinkText, want.LinkText)
	})

	t.Run("light", func(t *testing.T) {
		got := markdownStyleConfig("light")
		want := styles.LightStyleConfig
		assertStylePrimitiveEqual(t, got.Link, want.Link)
		assertStylePrimitiveEqual(t, got.LinkText, want.LinkText)
	})
}

func assertStylePrimitiveEqual(t *testing.T, got ansi.StylePrimitive, want ansi.StylePrimitive) {
	t.Helper()

	if strPtrValue(got.Color) != strPtrValue(want.Color) {
		t.Fatalf("Color: got %q want %q", strPtrValue(got.Color), strPtrValue(want.Color))
	}
	if strPtrValue(got.BackgroundColor) != strPtrValue(want.BackgroundColor) {
		t.Fatalf("BackgroundColor: got %q want %q", strPtrValue(got.BackgroundColor), strPtrValue(want.BackgroundColor))
	}
	if boolPtrValue(got.Bold) != boolPtrValue(want.Bold) {
		t.Fatalf("Bold: got %v want %v", boolPtrValue(got.Bold), boolPtrValue(want.Bold))
	}
	if boolPtrValue(got.Italic) != boolPtrValue(want.Italic) {
		t.Fatalf("Italic: got %v want %v", boolPtrValue(got.Italic), boolPtrValue(want.Italic))
	}
	if boolPtrValue(got.Underline) != boolPtrValue(want.Underline) {
		t.Fatalf("Underline: got %v want %v", boolPtrValue(got.Underline), boolPtrValue(want.Underline))
	}
	if boolPtrValue(got.CrossedOut) != boolPtrValue(want.CrossedOut) {
		t.Fatalf("CrossedOut: got %v want %v", boolPtrValue(got.CrossedOut), boolPtrValue(want.CrossedOut))
	}
	if got.Prefix != want.Prefix {
		t.Fatalf("Prefix: got %q want %q", got.Prefix, want.Prefix)
	}
	if got.Suffix != want.Suffix {
		t.Fatalf("Suffix: got %q want %q", got.Suffix, want.Suffix)
	}
	if got.BlockPrefix != want.BlockPrefix {
		t.Fatalf("BlockPrefix: got %q want %q", got.BlockPrefix, want.BlockPrefix)
	}
	if got.BlockSuffix != want.BlockSuffix {
		t.Fatalf("BlockSuffix: got %q want %q", got.BlockSuffix, want.BlockSuffix)
	}
}

func strPtrValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func boolPtrValue(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}
