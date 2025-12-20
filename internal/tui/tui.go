package tui

import (
        "clarity-cli/internal/store"

        tea "github.com/charmbracelet/bubbletea"
)

func Run(dir string, db *store.DB) error {
        m := newAppModel(dir, db)
        _, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
        return err
}
