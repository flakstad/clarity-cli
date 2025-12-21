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
