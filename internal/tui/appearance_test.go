package tui

import (
	"testing"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestAppearanceProfiles_KeepDefaultStable(t *testing.T) {
	oldProfile := lipgloss.ColorProfile()
	oldBG := lipgloss.HasDarkBackground()
	lipgloss.SetColorProfile(termenv.ANSI256)
	lipgloss.SetHasDarkBackground(true)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(oldProfile)
		lipgloss.SetHasDarkBackground(oldBG)
		setAppearanceProfile(appearanceDefault)
	})

	outline := model.Outline{StatusDefs: store.DefaultOutlineStatusDefs()}

	setAppearanceProfile(appearanceDefault)
	a := renderStatus(outline, "todo")

	setAppearanceProfile(appearanceNeon)
	b := renderStatus(outline, "todo")
	if a == b {
		t.Fatalf("expected neon profile to change rendered status output")
	}

	setAppearanceProfile(appearanceDefault)
	c := renderStatus(outline, "todo")
	if a != c {
		t.Fatalf("expected default profile to be stable across toggles")
	}
}
