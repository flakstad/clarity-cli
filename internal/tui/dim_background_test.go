package tui

import (
        "strings"
        "testing"

        "github.com/charmbracelet/lipgloss"
        "github.com/muesli/termenv"
)

func TestDimBackground_StripsInnerANSIStyles(t *testing.T) {
        oldProfile := lipgloss.ColorProfile()
        oldBG := lipgloss.HasDarkBackground()
        lipgloss.SetColorProfile(termenv.ANSI256)
        t.Cleanup(func() {
                lipgloss.SetColorProfile(oldProfile)
                lipgloss.SetHasDarkBackground(oldBG)
        })
        lipgloss.SetHasDarkBackground(true)

        // Give the inner content a strong color. If dimBackground does not strip ANSI
        // codes first, the inner style can override the scrim.
        in := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("HELLO")
        out := dimBackground(in)

        // Expect the scrim color to be present and the inner one to be absent.
        if !strings.Contains(out, "38;5;241") {
                t.Fatalf("expected dimmed foreground (38;5;241) in output; got %q", out)
        }
        if strings.Contains(out, "38;5;196") {
                t.Fatalf("expected inner foreground (38;5;196) to be stripped; got %q", out)
        }
}
