package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestRenderModalBox_UsesLightBackground_WhenThemeForcedLight(t *testing.T) {
	oldProfile := lipgloss.ColorProfile()
	oldBG := lipgloss.HasDarkBackground()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(oldProfile)
		lipgloss.SetHasDarkBackground(oldBG)
	})

	oldTheme := os.Getenv("CLARITY_TUI_THEME")
	oldDarkBG := os.Getenv("CLARITY_TUI_DARKBG")
	t.Cleanup(func() {
		_ = os.Setenv("CLARITY_TUI_THEME", oldTheme)
		_ = os.Setenv("CLARITY_TUI_DARKBG", oldDarkBG)
	})

	_ = os.Setenv("CLARITY_TUI_THEME", "light")
	_ = os.Setenv("CLARITY_TUI_DARKBG", "")
	applyThemePreference()
	if lipgloss.HasDarkBackground() {
		t.Fatalf("expected HasDarkBackground=false after forcing light theme")
	}

	out := renderModalBox(80, "Title", "Body")

	// With a forced light theme, we expect the light background variant to be used.
	// colorSurfaceBg is ac("255","235") so the light bg should appear in the ANSI output.
	if !strings.Contains(out, "48;5;255") {
		t.Fatalf("expected modal to include light background (48;5;255); got: %q", out)
	}
}

func TestRenderModalBox_HasNoBorderGlyphs(t *testing.T) {
	oldProfile := lipgloss.ColorProfile()
	oldBG := lipgloss.HasDarkBackground()
	lipgloss.SetColorProfile(termenv.ANSI256)
	lipgloss.SetHasDarkBackground(false)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(oldProfile)
		lipgloss.SetHasDarkBackground(oldBG)
	})

	out := renderModalBox(80, "Title", "Body")

	// Flat modals should not render a rounded border.
	if strings.Contains(out, "╭") || strings.Contains(out, "╮") || strings.Contains(out, "╰") || strings.Contains(out, "╯") {
		t.Fatalf("expected no border glyphs; got: %q", out)
	}
}

func TestOverlayCenter_NoModalDropShadow(t *testing.T) {
	oldProfile := lipgloss.ColorProfile()
	oldBG := lipgloss.HasDarkBackground()
	lipgloss.SetColorProfile(termenv.ANSI256)
	lipgloss.SetHasDarkBackground(true)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(oldProfile)
		lipgloss.SetHasDarkBackground(oldBG)
	})

	bg := strings.Join([]string{
		strings.Repeat(".", 40),
		strings.Repeat(".", 40),
		strings.Repeat(".", 40),
		strings.Repeat(".", 40),
		strings.Repeat(".", 40),
	}, "\n")
	fg := "X"

	out := overlayCenter(bg, fg, 40, 5)

	// Previously overlayCenter added a shadow using colorShadow = ac("255","236").
	// When dark background is forced, the shadow background would include 48;5;236.
	if strings.Contains(out, "48;5;236") {
		t.Fatalf("expected no modal drop-shadow background (48;5;236); got: %q", out)
	}
}

func TestRenderModalBox_ReappliesSurfaceAfterReset(t *testing.T) {
	oldProfile := lipgloss.ColorProfile()
	oldBG := lipgloss.HasDarkBackground()
	lipgloss.SetColorProfile(termenv.ANSI256)
	lipgloss.SetHasDarkBackground(false)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(oldProfile)
		lipgloss.SetHasDarkBackground(oldBG)
	})

	// Simulate nested lipgloss rendering that emits a hard reset mid-body.
	body := "Hello" + "\x1b[0m" + "World"
	out := renderModalBox(80, "Title", body)

	// Expect the modal surface fg/bg to be re-applied immediately after the reset.
	want := "\x1b[0m" + modalSurfaceANSI()
	if modalSurfaceANSI() == "" {
		t.Fatalf("expected modalSurfaceANSI to return a non-empty prefix")
	}
	if !strings.Contains(out, want) {
		t.Fatalf("expected modal to re-apply surface styling after reset; got: %q", out)
	}
}
