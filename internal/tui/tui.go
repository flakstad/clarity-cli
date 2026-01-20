package tui

import (
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func Run(dir string, db *store.DB) error {
	return RunWithWorkspace(dir, db, "")
}

func RunWithWorkspace(dir string, db *store.DB, workspace string) error {
	applyColorProfilePreference()
	applyThemePreference()
	applyGlyphPreference()
	applyAppearancePreference()
	applyListStylePreference()
	m := newAppModelWithWorkspace(dir, db, workspace)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
