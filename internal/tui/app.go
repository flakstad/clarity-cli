package tui

import (
        "fmt"
        "os"
        "path/filepath"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/bubbles/list"
        tea "github.com/charmbracelet/bubbletea"
        "github.com/charmbracelet/lipgloss"
)

type view int

const (
        viewProjects view = iota
        viewOutlines
        viewOutline
)

type reloadTickMsg struct{}

type appModel struct {
        dir   string
        store store.Store
        db    *store.DB

        width  int
        height int

        view view

        projectsList list.Model
        outlinesList list.Model
        itemsList    list.Model

        selectedProjectID string
        selectedOutlineID string

        lastDBModTime     time.Time
        lastEventsModTime time.Time
}

func newAppModel(dir string, db *store.DB) appModel {
        s := store.Store{Dir: dir}
        m := appModel{
                dir:   dir,
                store: s,
                db:    db,
                view:  viewProjects,
        }

        m.projectsList = newList("Projects", "Select a project", []list.Item{})
        m.outlinesList = newList("Outlines", "Select an outline", []list.Item{})
        m.itemsList = newList("Items", "Navigate items (read-only for now)", []list.Item{})

        m.refreshProjects()
        m.captureStoreModTimes()
        return m
}

func (m appModel) Init() tea.Cmd { return tickReload() }

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
        switch msg := msg.(type) {
        case tea.WindowSizeMsg:
                m.width = msg.Width
                m.height = msg.Height
                m.resizeLists()
                return m, nil

        case reloadTickMsg:
                if m.storeChanged() {
                        _ = m.reloadFromDisk()
                }
                return m, tickReload()

        case tea.KeyMsg:
                switch msg.String() {
                case "ctrl+c", "q":
                        return m, tea.Quit
                case "r":
                        // Reload from disk (so running CLI commands in another terminal is reflected).
                        _ = m.reloadFromDisk()
                        return m, nil
                case "esc", "backspace":
                        switch m.view {
                        case viewOutline:
                                m.view = viewOutlines
                                m.refreshOutlines(m.selectedProjectID)
                                return m, nil
                        case viewOutlines:
                                m.view = viewProjects
                                m.refreshProjects()
                                return m, nil
                        }
                case "enter":
                        switch m.view {
                        case viewProjects:
                                if it, ok := m.projectsList.SelectedItem().(projectItem); ok {
                                        m.selectedProjectID = it.project.ID
                                        m.db.CurrentProjectID = it.project.ID
                                        _ = m.store.Save(m.db)
                                        m.view = viewOutlines
                                        m.refreshOutlines(it.project.ID)
                                        return m, nil
                                }
                        case viewOutlines:
                                if it, ok := m.outlinesList.SelectedItem().(outlineItem); ok {
                                        m.selectedOutlineID = it.outline.ID
                                        m.view = viewOutline
                                        m.refreshItems(it.outline)
                                        return m, nil
                                }
                        }
                }
        }

        // Let the active list handle navigation keys.
        switch m.view {
        case viewProjects:
                var cmd tea.Cmd
                m.projectsList, cmd = m.projectsList.Update(msg)
                return m, cmd
        case viewOutlines:
                var cmd tea.Cmd
                m.outlinesList, cmd = m.outlinesList.Update(msg)
                return m, cmd
        case viewOutline:
                var cmd tea.Cmd
                m.itemsList, cmd = m.itemsList.Update(msg)
                return m, cmd
        default:
                return m, nil
        }
}

func (m appModel) View() string {
        header := lipgloss.NewStyle().
                Bold(true).
                Render(fmt.Sprintf("Clarity TUI  Dir=%s  Actor=%s  Project=%s",
                        m.dir,
                        emptyAsDash(m.db.CurrentActorID),
                        emptyAsDash(m.db.CurrentProjectID),
                ))

        var body string
        switch m.view {
        case viewProjects:
                body = m.projectsList.View()
        case viewOutlines:
                body = m.outlinesList.View()
        case viewOutline:
                body = m.viewOutline()
        default:
                body = ""
        }

        footer := lipgloss.NewStyle().Faint(true).Render("enter: select  backspace/esc: back  r: reload  q: quit")
        return strings.Join([]string{header, body, footer}, "\n\n")
}

func (m *appModel) resizeLists() {
        // Leave room for header/footer.
        h := m.height - 6
        if h < 8 {
                h = 8
        }
        w := m.width
        if w < 40 {
                w = 40
        }
        m.projectsList.SetSize(w, h)
        m.outlinesList.SetSize(w, h)
        // Outline view is split.
        m.itemsList.SetSize(w/2, h)
}

func emptyAsDash(s string) string {
        if strings.TrimSpace(s) == "" {
                return "-"
        }
        return s
}

func (m *appModel) refreshProjects() {
        curID := ""
        if it, ok := m.projectsList.SelectedItem().(projectItem); ok {
                curID = it.project.ID
        }
        var items []list.Item
        for _, p := range m.db.Projects {
                items = append(items, projectItem{project: p, current: p.ID == m.db.CurrentProjectID})
        }
        m.projectsList.SetItems(items)
        if curID != "" {
                selectListItemByID(&m.projectsList, curID)
        }
        if len(items) == 0 {
                m.projectsList.SetStatusBarItemName("project", "projects")
        }
}

func (m *appModel) refreshOutlines(projectID string) {
        curID := ""
        if it, ok := m.outlinesList.SelectedItem().(outlineItem); ok {
                curID = it.outline.ID
        }
        var items []list.Item
        for _, o := range m.db.Outlines {
                if o.ProjectID == projectID {
                        items = append(items, outlineItem{outline: o})
                }
        }
        m.outlinesList.SetItems(items)
        if curID != "" {
                selectListItemByID(&m.outlinesList, curID)
        }
}

func (m *appModel) refreshItems(outline model.Outline) {
        curID := ""
        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                curID = it.row.item.ID
        }
        var its []model.Item
        for _, it := range m.db.Items {
                if it.OutlineID == outline.ID && !it.Archived {
                        its = append(its, it)
                }
        }
        flat := flattenOutline(its)
        var items []list.Item
        for _, row := range flat {
                items = append(items, outlineRowItem{row: row, outline: outline})
        }
        m.itemsList.SetItems(items)
        if curID != "" {
                selectListItemByID(&m.itemsList, curID)
        }
}

func (m *appModel) viewOutline() string {
        bodyHeight := m.height - 6
        if bodyHeight < 8 {
                bodyHeight = 8
        }

        leftWidth := m.width / 2
        if leftWidth < 40 {
                leftWidth = 40
        }
        rightWidth := m.width - leftWidth - 2
        if rightWidth < 30 {
                rightWidth = 30
        }

        m.itemsList.SetSize(leftWidth, bodyHeight)

        left := m.itemsList.View()

        var detail string
        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                detail = renderItemDetail(m.db, it.outline, it.row.item, rightWidth, bodyHeight)
        } else {
                detail = lipgloss.NewStyle().Width(rightWidth).Height(bodyHeight).Render("No item selected.")
        }

        return lipgloss.JoinHorizontal(lipgloss.Top, left, detail)
}

func tickReload() tea.Cmd {
        return tea.Tick(750*time.Millisecond, func(time.Time) tea.Msg { return reloadTickMsg{} })
}

func (m *appModel) captureStoreModTimes() {
        m.lastDBModTime = fileModTime(filepath.Join(m.dir, "db.json"))
        m.lastEventsModTime = fileModTime(filepath.Join(m.dir, "events.jsonl"))
}

func (m *appModel) storeChanged() bool {
        dbMT := fileModTime(filepath.Join(m.dir, "db.json"))
        evMT := fileModTime(filepath.Join(m.dir, "events.jsonl"))
        return dbMT.After(m.lastDBModTime) || evMT.After(m.lastEventsModTime)
}

func fileModTime(path string) time.Time {
        st, err := os.Stat(path)
        if err != nil {
                return time.Time{}
        }
        return st.ModTime()
}

func (m *appModel) reloadFromDisk() error {
        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db
        m.captureStoreModTimes()

        // Refresh current view (and keep selection if possible).
        switch m.view {
        case viewProjects:
                m.refreshProjects()
        case viewOutlines:
                m.refreshOutlines(m.selectedProjectID)
        case viewOutline:
                if o, ok := m.db.FindOutline(m.selectedOutlineID); ok {
                        m.refreshItems(*o)
                }
        }
        return nil
}

func selectListItemByID(l *list.Model, id string) {
        for i := 0; i < len(l.Items()); i++ {
                switch it := l.Items()[i].(type) {
                case projectItem:
                        if it.project.ID == id {
                                l.Select(i)
                                return
                        }
                case outlineItem:
                        if it.outline.ID == id {
                                l.Select(i)
                                return
                        }
                case outlineRowItem:
                        if it.row.item.ID == id {
                                l.Select(i)
                                return
                        }
                }
        }
}
