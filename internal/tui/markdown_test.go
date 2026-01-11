package tui

import "testing"

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
