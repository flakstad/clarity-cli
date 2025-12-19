package tui

import (
        "fmt"

        "clarity-cli/internal/store"

        tea "github.com/charmbracelet/bubbletea"
)

type model struct {
        dir string
        db  *store.DB
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
        switch msg := msg.(type) {
        case tea.KeyMsg:
                switch msg.String() {
                case "q", "esc", "ctrl+c":
                        return m, tea.Quit
                }
        }
        return m, nil
}

func (m model) View() string {
        return fmt.Sprintf(
                "Clarity TUI (V1)\n\n- Dir: %s\n- Actors: %d\n- Projects: %d\n- Items: %d\n\nPress q to quit.\n",
                m.dir, len(m.db.Actors), len(m.db.Projects), len(m.db.Items),
        )
}

func Run(dir string, db *store.DB) error {
        _, err := tea.NewProgram(model{dir: dir, db: db}).Run()
        return err
}
