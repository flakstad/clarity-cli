package tui

import (
	"strings"
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

	setAppearanceProfile(appearanceTerminal)
	b := renderStatus(outline, "todo")
	if a == b {
		t.Fatalf("expected terminal profile to change rendered status output")
	}
	// Terminal profile should use ANSI 0-15 theme colors (30-37 / 90-97),
	// not fixed xterm palette indices (38;5;...).
	if strings.Contains(b, "38;5;") {
		t.Fatalf("expected terminal profile to avoid ANSI256 escapes; got: %q", b)
	}
	if !strings.Contains(b, ";35m") && !strings.Contains(b, ";95m") && !strings.Contains(b, "[35m") && !strings.Contains(b, "[95m") {
		t.Fatalf("expected terminal profile to use magenta ANSI escapes; got: %q", b)
	}

	setAppearanceProfile(appearanceNeon)
	c := renderStatus(outline, "todo")
	if a == c {
		t.Fatalf("expected neon profile to change rendered status output")
	}

	setAppearanceProfile(appearanceDefault)
	d := renderStatus(outline, "todo")
	if a != d {
		t.Fatalf("expected default profile to be stable across toggles")
	}
}

func TestAppearanceProfiles_Dracula_UsesDifferentPaletteOnLightVsDark(t *testing.T) {
	oldProfile := lipgloss.ColorProfile()
	oldBG := lipgloss.HasDarkBackground()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(oldProfile)
		lipgloss.SetHasDarkBackground(oldBG)
		setAppearanceProfile(appearanceDefault)
	})

	setAppearanceProfile(appearanceDracula)

	lipgloss.SetHasDarkBackground(false)
	light := lipgloss.NewStyle().Foreground(colorSurfaceFg).Render("X")

	lipgloss.SetHasDarkBackground(true)
	dark := lipgloss.NewStyle().Foreground(colorSurfaceFg).Render("X")

	if light == dark {
		t.Fatalf("expected dracula to render different surface fg on light vs dark backgrounds")
	}
}

func TestAppearanceProfiles_Alabaster_UsesDifferentPaletteOnLightVsDark(t *testing.T) {
	oldProfile := lipgloss.ColorProfile()
	oldBG := lipgloss.HasDarkBackground()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(oldProfile)
		lipgloss.SetHasDarkBackground(oldBG)
		setAppearanceProfile(appearanceDefault)
	})

	setAppearanceProfile(appearanceAlabaster)

	lipgloss.SetHasDarkBackground(false)
	light := lipgloss.NewStyle().Foreground(colorSurfaceFg).Render("X")

	lipgloss.SetHasDarkBackground(true)
	dark := lipgloss.NewStyle().Foreground(colorSurfaceFg).Render("X")

	if light == dark {
		t.Fatalf("expected alabaster to render different surface fg on light vs dark backgrounds")
	}
}
