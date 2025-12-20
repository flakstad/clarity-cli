package tui

import (
        "fmt"
        "os"
        "path/filepath"
        "sort"
        "strconv"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/bubbles/list"
        "github.com/charmbracelet/bubbles/textinput"
        tea "github.com/charmbracelet/bubbletea"
        "github.com/charmbracelet/lipgloss"
        xansi "github.com/charmbracelet/x/ansi"
)

type view int

const (
        viewProjects view = iota
        viewOutlines
        viewOutline
)

type reloadTickMsg struct{}

type escTimeoutMsg struct{}

type pane int

const (
        paneOutline pane = iota
        paneDetail
)

type modalKind int

const (
        modalNone modalKind = iota
        modalNewSibling
        modalNewChild
        modalConfirmArchive
        modalEditTitle
)

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
        selectedOutline   *model.Outline

        pane                pane
        collapsed           map[string]bool
        collapseInitialized bool

        modal      modalKind
        modalForID string
        input      textinput.Model

        pendingEsc bool

        lastDBModTime     time.Time
        lastEventsModTime time.Time

        minibufferText string
}

func newAppModel(dir string, db *store.DB) appModel {
        s := store.Store{Dir: dir}
        m := appModel{
                dir:   dir,
                store: s,
                db:    db,
                view:  viewProjects,
                pane:  paneOutline,
        }

        m.projectsList = newList("Projects", "Select a project", []list.Item{})
        m.outlinesList = newList("Outlines", "Select an outline", []list.Item{})
        m.itemsList = newList("Outline", "Navigate items (split view)", []list.Item{})
        m.itemsList.SetFilteringEnabled(false)
        m.itemsList.SetShowFilter(false)

        m.input = textinput.New()
        m.input.Placeholder = "Title"
        m.input.CharLimit = 200
        m.input.Width = 40

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

        case escTimeoutMsg:
                if m.view == viewOutline && m.pendingEsc && m.modal == modalNone {
                        // Treat a lone ESC as "back" in the outline view.
                        m.pendingEsc = false
                        m.view = viewOutlines
                        m.refreshOutlines(m.selectedProjectID)
                        return m, nil
                }
                return m, nil

        case tea.KeyMsg:
                // If a modal is open in the outline view, route all keys to the modal handler
                // so text inputs behave normally (e.g. backspace edits).
                if m.view == viewOutline && m.modal != modalNone {
                        return m.updateOutline(msg)
                }
                switch msg.String() {
                case "ctrl+c", "q":
                        return m, tea.Quit
                case "backspace":
                        if m.view == viewOutline && m.modal == modalNone && m.pane == paneDetail {
                                m.pane = paneOutline
                                return m, nil
                        }
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
                case "esc":
                        if m.view == viewOutline && m.modal == modalNone && m.pane == paneDetail {
                                m.pane = paneOutline
                                return m, nil
                        }
                        if m.view == viewOutline && m.modal == modalNone {
                                // Some terminals send Alt+<key> as ESC then <key>.
                                // Delay treating ESC as "back" so we can interpret ESC+<key> as Alt+<key>.
                                m.pendingEsc = true
                                return m, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return escTimeoutMsg{} })
                        }
                        // Non-outline views: ESC goes back immediately.
                        switch m.view {
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
                                        m.captureStoreModTimes()
                                        m.view = viewOutlines
                                        m.refreshOutlines(it.project.ID)
                                        return m, nil
                                }
                        case viewOutlines:
                                if it, ok := m.outlinesList.SelectedItem().(outlineItem); ok {
                                        m.selectedOutlineID = it.outline.ID
                                        m.view = viewOutline
                                        m.pane = paneOutline
                                        m.collapsed = map[string]bool{}
                                        m.collapseInitialized = false
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
                return m.updateOutline(msg)
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

        footer := m.footerBlock()
        return strings.Join([]string{header, body, footer}, "\n\n")
}

func (m appModel) footerText() string {
        base := "enter: select  backspace/esc: back  q: quit"
        if m.view != viewOutline {
                return base
        }
        focus := "focus=outline"
        if m.pane == paneDetail {
                focus = "focus=detail"
        }
        if m.modal != modalNone {
                if m.modal == modalConfirmArchive {
                        return "archive: y/enter: confirm  n/esc: cancel  " + focus
                }
                if m.modal == modalEditTitle {
                        return "edit title: type, enter: save, esc: cancel  " + focus
                }
                return "new item: type title, enter: save, esc: cancel  " + focus
        }
        return "enter: toggle detail  tab: toggle focus  arrows/jk/ctrl+n/p/h/l/ctrl+b/f: navigate  alt+arrows: move/indent/outdent  z/Z: collapse  n/N: add  e: edit title  r: archive  " + focus
}

func (m appModel) footerBlock() string {
        keyHelp := lipgloss.NewStyle().Faint(true).Render(m.footerText())
        return m.minibufferView() + "\n" + keyHelp
}

func (m appModel) minibufferView() string {
        w := m.width
        if w <= 0 {
                w = 80
        }
        // Replace newlines so we always render a single-line minibuffer.
        txt := strings.TrimSpace(strings.ReplaceAll(m.minibufferText, "\n", " "))
        if txt == "" {
                txt = " "
        }
        innerW := w - 2
        if innerW < 10 {
                innerW = 10
        }
        if xansi.StringWidth(txt) > innerW {
                txt = xansi.Cut(txt, 0, innerW-1) + "…"
        }
        return lipgloss.NewStyle().
                Width(w).
                Padding(0, 1).
                Background(lipgloss.Color("236")).
                Foreground(lipgloss.Color("255")).
                Render(txt)
}

func (m *appModel) showMinibuffer(text string) {
        m.minibufferText = text
}

func (m *appModel) resizeLists() {
        // Leave room for header/footer.
        h := m.height - 6
        if h < 8 {
                h = 8
        }
        w := m.width
        if w < 10 {
                w = 10
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
        m.selectedOutline = &outline
        curID := ""
        switch it := m.itemsList.SelectedItem().(type) {
        case outlineRowItem:
                curID = it.row.item.ID
        case addItemRow:
                curID = "__add__"
        }
        var its []model.Item
        for _, it := range m.db.Items {
                if it.OutlineID == outline.ID && !it.Archived {
                        its = append(its, it)
                }
        }
        if !m.collapseInitialized {
                childrenCount := map[string]int{}
                for _, it := range its {
                        if it.ParentID == nil || *it.ParentID == "" {
                                continue
                        }
                        childrenCount[*it.ParentID]++
                }
                for id, n := range childrenCount {
                        if n > 0 {
                                m.collapsed[id] = true
                        }
                }
                m.collapseInitialized = true
        }

        flat := flattenOutline(its, m.collapsed)
        var items []list.Item
        for _, row := range flat {
                items = append(items, outlineRowItem{row: row, outline: outline})
        }
        // Always-present affordance for adding an item (useful for empty outlines).
        items = append(items, addItemRow{})
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

        w := m.width
        if w < 10 {
                w = 10
        }
        gapW := 2
        leftWidth := (w - gapW) / 2
        rightWidth := w - gapW - leftWidth

        m.itemsList.SetSize(leftWidth, bodyHeight)

        leftBox := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
        if m.pane == paneOutline {
                leftBox = leftBox.BorderForeground(lipgloss.Color("62"))
        }
        left := leftBox.Width(leftWidth).Height(bodyHeight).Render(m.itemsList.View())

        var detail string
        switch it := m.itemsList.SelectedItem().(type) {
        case outlineRowItem:
                detail = renderItemDetail(m.db, it.outline, it.row.item, rightWidth, bodyHeight, m.pane == paneDetail)
        case addItemRow:
                detailBox := lipgloss.NewStyle().
                        Width(rightWidth).
                        Height(bodyHeight).
                        Padding(0, 1).
                        Border(lipgloss.RoundedBorder()).
                        BorderForeground(lipgloss.Color("240"))
                if m.pane == paneDetail {
                        detailBox = detailBox.BorderForeground(lipgloss.Color("62"))
                }
                detail = detailBox.Render(strings.Join([]string{
                        "(no item selected)",
                        "",
                        "Press enter to add a new item, or press n (sibling) / N (child).",
                }, "\n"))
        default:
                detail = lipgloss.NewStyle().Width(rightWidth).Height(bodyHeight).Render("No item selected.")
        }

        gap := lipgloss.NewStyle().Width(gapW).Render("")
        main := lipgloss.JoinHorizontal(lipgloss.Top, left, gap, detail)
        if m.modal == modalNone {
                return main
        }

        bg := dimBackground(main)
        fg := m.renderModal()
        return overlayCenter(bg, fg, w, bodyHeight)
}

func (m *appModel) renderModal() string {
        switch m.modal {
        case modalNewSibling, modalNewChild:
                title := "New item"
                if m.modal == modalNewChild {
                        title = "New subitem"
                }
                return renderModalBox(m.width, title, m.input.View()+"\n\nenter: save   esc: cancel")
        case modalEditTitle:
                return renderModalBox(m.width, "Edit title", m.input.View()+"\n\nenter: save   esc: cancel")
        case modalConfirmArchive:
                title := "this item"
                if it, ok := m.db.FindItem(m.modalForID); ok {
                        if strings.TrimSpace(it.Title) != "" {
                                title = fmt.Sprintf("%q", it.Title)
                        }
                }
                extra := countUnarchivedDescendants(m.db, m.modalForID)
                cascade := "This will archive this item."
                if extra == 1 {
                        cascade = "This will archive this item and 1 subitem."
                } else if extra > 1 {
                        cascade = fmt.Sprintf("This will archive this item and %d subitems.", extra)
                }
                body := strings.Join([]string{
                        "Archive " + title + "?",
                        cascade,
                        "You can unarchive later via the CLI.",
                }, "\n")
                return renderModalBox(m.width, "Confirm", body+"\n\nenter/y: archive   esc/n: cancel")
        default:
                return ""
        }
}

func tickReload() tea.Cmd {
        return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg { return reloadTickMsg{} })
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
                case addItemRow:
                        if id == "__add__" {
                                l.Select(i)
                                return
                        }
                }
        }
}

func (m appModel) updateOutline(msg tea.Msg) (tea.Model, tea.Cmd) {
        // Modal input takes over all keys.
        if m.modal != modalNone {
                if m.modal == modalConfirmArchive {
                        if km, ok := msg.(tea.KeyMsg); ok {
                                switch km.String() {
                                case "esc", "n":
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        return m, nil
                                case "enter", "y":
                                        target := m.modalForID
                                        prevIdx := m.itemsList.Index()
                                        nextID := m.nearestSelectableItemID(prevIdx)
                                        archived, err := m.archiveItemTree(target)
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        if m.selectedOutline != nil {
                                                m.refreshItems(*m.selectedOutline)
                                                selectListItemByID(&m.itemsList, nextID)
                                        }
                                        if err != nil {
                                                m.showMinibuffer("Archive failed: " + err.Error())
                                        } else if archived > 0 {
                                                m.showMinibuffer(fmt.Sprintf("Archived %d item(s)", archived))
                                        }
                                        return m, nil
                                }
                        }
                        return m, nil
                }

                switch km := msg.(type) {
                case tea.KeyMsg:
                        switch km.String() {
                        case "esc":
                                m.modal = modalNone
                                m.modalForID = ""
                                m.input.Blur()
                                return m, nil
                        case "enter":
                                val := strings.TrimSpace(m.input.Value())
                                if val == "" {
                                        return m, nil
                                }
                                switch m.modal {
                                case modalEditTitle:
                                        _ = m.setTitleFromModal(val)
                                default:
                                        _ = m.createItemFromModal(val)
                                }
                                m.modal = modalNone
                                m.modalForID = ""
                                m.input.SetValue("")
                                m.input.Blur()
                                return m, nil
                        }
                }
                var cmd tea.Cmd
                m.input, cmd = m.input.Update(msg)
                return m, cmd
        }

        switch msg := msg.(type) {
        case tea.KeyMsg:
                // Handle ESC-prefix Alt sequences (ESC then key).
                if m.pendingEsc {
                        m.pendingEsc = false
                        // ESC + navigation keys => treat as Alt+...
                        switch msg.String() {
                        case "up", "k", "p":
                                _ = m.moveSelected("up")
                                return m, nil
                        case "down", "j", "n":
                                _ = m.moveSelected("down")
                                return m, nil
                        case "right", "l", "f":
                                _ = m.indentSelected()
                                return m, nil
                        case "left", "h", "b":
                                _ = m.outdentSelected()
                                return m, nil
                        }
                        // Otherwise: fall through and handle the key normally.
                }

                // Focus handling.
                if msg.String() == "tab" {
                        if m.pane == paneOutline {
                                m.pane = paneDetail
                        } else {
                                m.pane = paneOutline
                        }
                        return m, nil
                }

                // Open item / create items.
                switch msg.String() {
                case "enter":
                        if m.pane == paneOutline {
                                switch m.itemsList.SelectedItem().(type) {
                                case outlineRowItem:
                                        m.pane = paneDetail
                                        return m, nil
                                case addItemRow:
                                        m.modal = modalNewSibling
                                        m.modalForID = ""
                                        m.input.SetValue("")
                                        m.input.Focus()
                                        return m, nil
                                }
                        }
                case "o":
                        if m.pane == paneOutline {
                                if _, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                        m.pane = paneDetail
                                        return m, nil
                                }
                        }
                case "e":
                        // Edit title for selected item.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                m.modal = modalEditTitle
                                m.modalForID = it.row.item.ID
                                m.input.SetValue(it.row.item.Title)
                                m.input.Focus()
                                return m, nil
                        }
                case "n":
                        // New sibling (after selected) in outline pane.
                        if m.pane == paneOutline {
                                m.modal = modalNewSibling
                                m.modalForID = ""
                                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                        m.modalForID = it.row.item.ID
                                }
                                m.input.SetValue("")
                                m.input.Focus()
                                return m, nil
                        }
                case "N":
                        // New child (under selected) in either pane. If "+ Add item" selected, fall back to root.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                m.modal = modalNewChild
                                m.modalForID = it.row.item.ID
                        } else {
                                m.modal = modalNewSibling
                                m.modalForID = ""
                        }
                        m.input.SetValue("")
                        m.input.Focus()
                        return m, nil
                case "r":
                        // Archive/remove selected item (with confirmation).
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                m.modal = modalConfirmArchive
                                m.modalForID = it.row.item.ID
                                m.input.Blur()
                                return m, nil
                        }
                }

                // When focused on detail, don't let navigation keys change the outline cursor.
                // (The detail pane is read-only for now.)
                if m.pane == paneDetail {
                        return m, nil
                }

                // Collapse toggles.
                if msg.String() == "z" {
                        m.toggleCollapseSelected()
                        return m, nil
                }
                if msg.String() == "Z" {
                        m.toggleCollapseAll()
                        return m, nil
                }

                // Outline navigation.
                if m.navOutline(msg) {
                        return m, nil
                }

                // Outline structural operations (left pane only).
                if m.pane == paneOutline && m.mutateOutlineByKey(msg) {
                        return m, nil
                }
        }

        // Allow list to handle incidental keys (help paging, etc).
        var cmd tea.Cmd
        m.itemsList, cmd = m.itemsList.Update(msg)
        return m, cmd
}

func (m *appModel) nearestSelectableItemID(fromIdx int) string {
        items := m.itemsList.Items()
        if fromIdx < 0 {
                fromIdx = 0
        }
        for i := fromIdx + 1; i < len(items); i++ {
                if it, ok := items[i].(outlineRowItem); ok {
                        return it.row.item.ID
                }
        }
        for i := fromIdx - 1; i >= 0; i-- {
                if it, ok := items[i].(outlineRowItem); ok {
                        return it.row.item.ID
                }
        }
        return "__add__"
}

func (m *appModel) archiveItem(itemID string) error {
        actorID := strings.TrimSpace(m.db.CurrentActorID)
        if actorID == "" {
                return nil
        }

        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db

        t, ok := m.db.FindItem(itemID)
        if !ok {
                return nil
        }
        if !canEditItem(m.db, actorID, t) {
                return nil
        }

        t.Archived = true
        t.UpdatedAt = time.Now().UTC()
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        _ = m.store.AppendEvent(actorID, "item.archive", t.ID, map[string]any{"archived": t.Archived})
        m.captureStoreModTimes()
        return nil
}

func (m *appModel) archiveItemTree(rootID string) (int, error) {
        actorID := strings.TrimSpace(m.db.CurrentActorID)
        if actorID == "" {
                return 0, nil
        }

        db, err := m.store.Load()
        if err != nil {
                return 0, err
        }
        m.db = db

        ids := subtreeItemIDs(m.db, rootID)
        if len(ids) == 0 {
                return 0, nil
        }

        now := time.Now().UTC()
        archived := 0
        for _, id := range ids {
                t, ok := m.db.FindItem(id)
                if !ok {
                        continue
                }
                if !canEditItem(m.db, actorID, t) {
                        continue
                }
                if t.Archived {
                        continue
                }
                t.Archived = true
                t.UpdatedAt = now
                _ = m.store.AppendEvent(actorID, "item.archive", t.ID, map[string]any{"archived": true})
                archived++
        }

        if err := m.store.Save(m.db); err != nil {
                return archived, err
        }
        m.captureStoreModTimes()
        return archived, nil
}

func countUnarchivedDescendants(db *store.DB, rootID string) int {
        if db == nil || strings.TrimSpace(rootID) == "" {
                return 0
        }
        ids := subtreeItemIDs(db, rootID)
        if len(ids) <= 1 {
                return 0
        }
        n := 0
        for _, id := range ids[1:] {
                it, ok := db.FindItem(id)
                if !ok {
                        continue
                }
                if !it.Archived {
                        n++
                }
        }
        return n
}

func subtreeItemIDs(db *store.DB, rootID string) []string {
        if db == nil || strings.TrimSpace(rootID) == "" {
                return nil
        }

        children := map[string][]string{}
        for _, it := range db.Items {
                if it.ParentID == nil || strings.TrimSpace(*it.ParentID) == "" {
                        continue
                }
                pid := strings.TrimSpace(*it.ParentID)
                if pid == "" {
                        continue
                }
                children[pid] = append(children[pid], it.ID)
        }

        seen := map[string]bool{}
        var out []string
        queue := []string{rootID}
        for len(queue) > 0 {
                id := queue[0]
                queue = queue[1:]
                if seen[id] {
                        continue
                }
                seen[id] = true
                out = append(out, id)
                for _, ch := range children[id] {
                        if !seen[ch] {
                                queue = append(queue, ch)
                        }
                }
        }
        return out
}

func overlayCenter(bg, fg string, w, h int) string {
        bgLines := splitLinesN(bg, h)
        fgLines := strings.Split(fg, "\n")
        fgH := len(fgLines)
        fgW := 0
        for _, ln := range fgLines {
                if n := xansi.StringWidth(ln); n > fgW {
                        fgW = n
                }
        }
        if fgW <= 0 || fgH <= 0 {
                return strings.Join(bgLines, "\n")
        }
        if fgW > w {
                fgW = w
        }
        if fgH > h {
                fgH = h
        }

        x := (w - fgW) / 2
        y := (h - fgH) / 2
        if x < 0 {
                x = 0
        }
        if y < 0 {
                y = 0
        }

        // Shadow to give the modal depth.
        shadowStyle := lipgloss.NewStyle().Background(lipgloss.Color("236"))
        shadowLine := shadowStyle.Render(strings.Repeat(" ", fgW))
        shadow := make([]string, 0, fgH)
        for i := 0; i < fgH; i++ {
                shadow = append(shadow, shadowLine)
        }
        overlayAt(bgLines, shadow, w, x+1, y+1, fgW)
        overlayAt(bgLines, fgLines, w, x, y, fgW)
        return strings.Join(bgLines, "\n")
}

func overlayAt(bgLines []string, fgLines []string, w, x, y, fgW int) {
        if fgW <= 0 {
                return
        }
        if x < 0 {
                x = 0
        }
        if y < 0 {
                y = 0
        }
        for i := 0; i < len(fgLines) && y+i < len(bgLines); i++ {
                bgLine := bgLines[y+i]
                left := xansi.Cut(bgLine, 0, x)
                right := xansi.Cut(bgLine, x+fgW, w)

                fgLine := fgLines[i]
                if n := xansi.StringWidth(fgLine); n < fgW {
                        fgLine += strings.Repeat(" ", fgW-n)
                } else if n > fgW {
                        fgLine = xansi.Cut(fgLine, 0, fgW)
                }

                bgLines[y+i] = left + fgLine + right
        }
}

func dimBackground(s string) string {
        // A simple "scrim" effect: desaturate + faint. This keeps layout identical and
        // makes the modal feel closer without destroying the context behind it.
        return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Faint(true).Render(s)
}

func renderModalBox(screenWidth int, title, body string) string {
        w := screenWidth - 12
        if w > screenWidth-4 {
                w = screenWidth - 4
        }
        if w < 20 {
                w = 20
        }
        if w > 96 {
                w = 96
        }

        header := lipgloss.NewStyle().Bold(true).Render(title)
        content := header + "\n\n" + body

        box := lipgloss.NewStyle().
                Width(w).
                Padding(1, 2).
                Border(lipgloss.RoundedBorder()).
                BorderForeground(lipgloss.Color("62")).
                Background(lipgloss.Color("235"))
        return box.Render(content)
}

func splitLinesN(s string, n int) []string {
        lines := strings.Split(s, "\n")
        if len(lines) >= n {
                return lines[:n]
        }
        out := make([]string, 0, n)
        out = append(out, lines...)
        for len(out) < n {
                out = append(out, "")
        }
        return out
}

func isAltEnter(msg tea.KeyMsg) bool {
        if msg.Alt && msg.Type == tea.KeyEnter {
                return true
        }
        // Different terminals report this differently.
        switch msg.String() {
        case "alt+enter", "alt+return", "alt+\r":
                return true
        default:
                return false
        }
}

func (m *appModel) navOutline(msg tea.KeyMsg) bool {
        switch msg.String() {
        case "down", "j", "ctrl+n":
                m.itemsList.CursorDown()
                return true
        case "up", "k", "ctrl+p":
                m.itemsList.CursorUp()
                return true
        case "right", "l", "ctrl+f":
                m.navIntoFirstChild()
                return true
        case "left", "h", "ctrl+b":
                m.navToParent()
                return true
        default:
                return false
        }
}

func (m *appModel) navIntoFirstChild() {
        it, ok := m.itemsList.SelectedItem().(outlineRowItem)
        if !ok {
                return
        }
        if !it.row.hasChildren {
                return
        }
        if it.row.collapsed {
                m.collapsed[it.row.item.ID] = false
                m.refreshItems(it.outline)
        }
        idx := m.itemsList.Index()
        // In our flattening, the first child (if visible) is the next row with depth+1.
        items := m.itemsList.Items()
        if idx+1 >= len(items) {
                return
        }
        if next, ok := items[idx+1].(outlineRowItem); ok && next.row.depth == it.row.depth+1 {
                m.itemsList.Select(idx + 1)
        }
}

func (m *appModel) navToParent() {
        it, ok := m.itemsList.SelectedItem().(outlineRowItem)
        if !ok {
                return
        }
        idx := m.itemsList.Index()
        if idx <= 0 || it.row.depth <= 0 {
                return
        }
        wantDepth := it.row.depth - 1
        items := m.itemsList.Items()
        for i := idx - 1; i >= 0; i-- {
                prev, ok := items[i].(outlineRowItem)
                if !ok {
                        continue
                }
                if prev.row.depth == wantDepth {
                        m.itemsList.Select(i)
                        return
                }
        }
}

func (m *appModel) toggleCollapseSelected() {
        it, ok := m.itemsList.SelectedItem().(outlineRowItem)
        if !ok {
                return
        }
        if !it.row.hasChildren {
                return
        }
        m.collapsed[it.row.item.ID] = !m.collapsed[it.row.item.ID]
        m.refreshItems(it.outline)
}

func (m *appModel) toggleCollapseAll() {
        if m.selectedOutline == nil {
                return
        }

        // If anything with children is expanded, collapse all; otherwise expand all.
        childrenCount := map[string]int{}
        for _, it := range m.db.Items {
                if it.Archived || it.OutlineID != m.selectedOutline.ID {
                        continue
                }
                if it.ParentID == nil || *it.ParentID == "" {
                        continue
                }
                childrenCount[*it.ParentID]++
        }

        anyExpanded := false
        for id, n := range childrenCount {
                if n <= 0 {
                        continue
                }
                if !m.collapsed[id] {
                        anyExpanded = true
                        break
                }
        }

        for id, n := range childrenCount {
                if n <= 0 {
                        continue
                }
                m.collapsed[id] = anyExpanded
        }
        m.refreshItems(*m.selectedOutline)
}

func (m *appModel) mutateOutlineByKey(msg tea.KeyMsg) bool {
        // Move item down/up.
        if isAltDown(msg) {
                _ = m.moveSelected("down")
                return true
        }
        if isAltUp(msg) {
                _ = m.moveSelected("up")
                return true
        }
        // Indent/outdent.
        if isAltRight(msg) {
                _ = m.indentSelected()
                return true
        }
        if isAltLeft(msg) {
                _ = m.outdentSelected()
                return true
        }
        return false
}

func isAltDown(msg tea.KeyMsg) bool {
        if msg.Alt && msg.Type == tea.KeyDown {
                return true
        }
        return msg.String() == "alt+down" || msg.String() == "alt+j" || msg.String() == "alt+n"
}

func isAltUp(msg tea.KeyMsg) bool {
        if msg.Alt && msg.Type == tea.KeyUp {
                return true
        }
        return msg.String() == "alt+up" || msg.String() == "alt+k" || msg.String() == "alt+p"
}

func isAltRight(msg tea.KeyMsg) bool {
        if msg.Alt && msg.Type == tea.KeyRight {
                return true
        }
        return msg.String() == "alt+right" || msg.String() == "alt+l" || msg.String() == "alt+f"
}

func isAltLeft(msg tea.KeyMsg) bool {
        if msg.Alt && msg.Type == tea.KeyLeft {
                return true
        }
        return msg.String() == "alt+left" || msg.String() == "alt+h" || msg.String() == "alt+b"
}

func (m *appModel) createItemFromModal(title string) error {
        if m.selectedOutline == nil {
                return nil
        }
        outline := *m.selectedOutline
        actorID := strings.TrimSpace(m.db.CurrentActorID)
        if actorID == "" {
                return nil
        }

        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db

        // Keep outline fresh.
        if o, ok := m.db.FindOutline(outline.ID); ok {
                outline = *o
                m.selectedOutline = o
        }

        var parentID *string
        if m.modal == modalNewChild {
                if strings.TrimSpace(m.modalForID) != "" {
                        tmp := m.modalForID
                        parentID = &tmp
                }
        } else if m.modal == modalNewSibling {
                if strings.TrimSpace(m.modalForID) != "" {
                        // sibling => same parent as current item
                        if cur, ok := m.db.FindItem(m.modalForID); ok {
                                parentID = cur.ParentID
                        }
                }
        }

        // Determine insertion rank.
        rank := nextSiblingRank(m.db, outline.ID, parentID)
        if m.modal == modalNewSibling && strings.TrimSpace(m.modalForID) != "" {
                // Insert after current item among its siblings.
                if cur, ok := m.db.FindItem(m.modalForID); ok {
                        sibs := siblingItems(m.db, outline.ID, parentID)
                        sibs = filterItems(sibs, func(x *model.Item) bool { return !x.Archived })
                        idx := indexOfItem(sibs, cur.ID)
                        if idx >= 0 {
                                lower := cur.Rank
                                upper := ""
                                if idx+1 < len(sibs) {
                                        upper = sibs[idx+1].Rank
                                }
                                if r, err := store.RankBetween(lower, upper); err == nil {
                                        rank = r
                                }
                        }
                }
        }

        assigned := defaultAssignedActorID(m.db, actorID)
        now := time.Now().UTC()
        newItem := model.Item{
                ID:                 m.store.NextID(m.db, "item"),
                ProjectID:          outline.ProjectID,
                OutlineID:          outline.ID,
                ParentID:           parentID,
                Rank:               rank,
                Title:              title,
                Description:        "",
                StatusID:           "todo",
                Priority:           false,
                OnHold:             false,
                Due:                nil,
                Schedule:           nil,
                LegacyDueAt:        nil,
                LegacyScheduledAt:  nil,
                Tags:               nil,
                Archived:           false,
                OwnerActorID:       actorID,
                AssignedActorID:    assigned,
                OwnerDelegatedFrom: nil,
                OwnerDelegatedAt:   nil,
                CreatedBy:          actorID,
                CreatedAt:          now,
                UpdatedAt:          now,
        }
        m.db.Items = append(m.db.Items, newItem)

        if err := m.store.Save(m.db); err != nil {
                return err
        }
        _ = m.store.AppendEvent(actorID, "item.create", newItem.ID, newItem)
        m.captureStoreModTimes()
        m.showMinibuffer("Created " + newItem.ID)

        // Expand parent if we created a child.
        if parentID != nil {
                m.collapsed[*parentID] = false
        }

        m.refreshItems(outline)
        selectListItemByID(&m.itemsList, newItem.ID)
        return nil
}

func (m *appModel) setTitleFromModal(title string) error {
        itemID := strings.TrimSpace(m.modalForID)
        if itemID == "" {
                return nil
        }
        actorID := strings.TrimSpace(m.db.CurrentActorID)
        if actorID == "" {
                return nil
        }

        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db

        t, ok := m.db.FindItem(itemID)
        if !ok {
                return nil
        }
        if !canEditItem(m.db, actorID, t) {
                return nil
        }

        t.Title = strings.TrimSpace(title)
        t.UpdatedAt = time.Now().UTC()
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        _ = m.store.AppendEvent(actorID, "item.set_title", t.ID, map[string]any{"title": t.Title})
        m.captureStoreModTimes()

        if m.selectedOutline != nil {
                if o, ok := m.db.FindOutline(m.selectedOutline.ID); ok {
                        m.selectedOutline = o
                }
                m.refreshItems(*m.selectedOutline)
                selectListItemByID(&m.itemsList, t.ID)
        }
        return nil
}

func defaultAssignedActorID(db *store.DB, actorID string) *string {
        act, ok := db.FindActor(actorID)
        if !ok {
                return nil
        }
        if act.Kind == model.ActorKindAgent {
                tmp := actorID
                return &tmp
        }
        return nil
}

func (m *appModel) moveSelected(dir string) error {
        it, ok := m.itemsList.SelectedItem().(outlineRowItem)
        if !ok {
                return nil
        }
        actorID := strings.TrimSpace(m.db.CurrentActorID)
        if actorID == "" {
                return nil
        }

        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db
        t, ok := m.db.FindItem(it.row.item.ID)
        if !ok {
                return nil
        }
        if !canEditItem(m.db, actorID, t) {
                return nil
        }

        // We need the moved item included for finding current position; build full list.
        full := siblingItems(m.db, t.OutlineID, t.ParentID)
        full = filterItems(full, func(x *model.Item) bool { return !x.Archived })
        idx := indexOfItem(full, t.ID)
        if idx < 0 {
                return nil
        }
        switch dir {
        case "up":
                if idx == 0 {
                        return nil
                }
                ref := full[idx-1]
                return m.reorderItem(t, "", ref.ID)
        case "down":
                if idx+1 >= len(full) {
                        return nil
                }
                ref := full[idx+1]
                return m.reorderItem(t, ref.ID, "")
        default:
                return nil
        }
}

func (m *appModel) reorderItem(t *model.Item, afterID, beforeID string) error {
        // Compute rank in sibling set excluding t.
        sibs := siblingItems(m.db, t.OutlineID, t.ParentID)
        sibs = filterItems(sibs, func(x *model.Item) bool { return x.ID != t.ID && !x.Archived })

        var lower string
        var upper string
        if beforeID != "" {
                refIdx := indexOfItem(sibs, beforeID)
                if refIdx < 0 {
                        return nil
                }
                upper = sibs[refIdx].Rank
                if refIdx > 0 {
                        lower = sibs[refIdx-1].Rank
                }
        } else if afterID != "" {
                refIdx := indexOfItem(sibs, afterID)
                if refIdx < 0 {
                        return nil
                }
                lower = sibs[refIdx].Rank
                if refIdx+1 < len(sibs) {
                        upper = sibs[refIdx+1].Rank
                }
        } else {
                return nil
        }

        r, err := store.RankBetween(lower, upper)
        if err != nil {
                return err
        }
        t.Rank = r
        t.UpdatedAt = time.Now().UTC()
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        actorID := strings.TrimSpace(m.db.CurrentActorID)
        _ = m.store.AppendEvent(actorID, "item.move", t.ID, map[string]any{"before": beforeID, "after": afterID, "rank": t.Rank})
        m.captureStoreModTimes()
        m.showMinibuffer("Moved " + t.ID)
        if m.selectedOutline != nil {
                m.refreshItems(*m.selectedOutline)
                selectListItemByID(&m.itemsList, t.ID)
        }
        return nil
}

func (m *appModel) indentSelected() error {
        it, ok := m.itemsList.SelectedItem().(outlineRowItem)
        if !ok {
                return nil
        }
        actorID := strings.TrimSpace(m.db.CurrentActorID)
        if actorID == "" {
                return nil
        }
        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db
        t, ok := m.db.FindItem(it.row.item.ID)
        if !ok {
                return nil
        }
        if !canEditItem(m.db, actorID, t) {
                return nil
        }
        // Indent => become child of the previous sibling (same parent).
        sibs := siblingItems(m.db, t.OutlineID, t.ParentID)
        sibs = filterItems(sibs, func(x *model.Item) bool { return !x.Archived })
        idx := indexOfItem(sibs, t.ID)
        if idx <= 0 {
                return nil
        }
        newParentID := sibs[idx-1].ID
        if isAncestor(m.db, t.ID, newParentID) || newParentID == t.ID {
                return nil
        }
        tmp := newParentID
        t.ParentID = &tmp
        t.Rank = nextSiblingRank(m.db, t.OutlineID, t.ParentID)
        t.UpdatedAt = time.Now().UTC()
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        _ = m.store.AppendEvent(actorID, "item.set_parent", t.ID, map[string]any{"parent": newParentID, "rank": t.Rank})
        m.captureStoreModTimes()
        m.showMinibuffer("Indented " + t.ID)
        // Expand new parent so the moved item stays visible.
        m.collapsed[newParentID] = false
        if m.selectedOutline != nil {
                m.refreshItems(*m.selectedOutline)
                selectListItemByID(&m.itemsList, t.ID)
        }
        return nil
}

func (m *appModel) outdentSelected() error {
        it, ok := m.itemsList.SelectedItem().(outlineRowItem)
        if !ok {
                return nil
        }
        actorID := strings.TrimSpace(m.db.CurrentActorID)
        if actorID == "" {
                return nil
        }

        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db
        t, ok := m.db.FindItem(it.row.item.ID)
        if !ok {
                return nil
        }
        if !canEditItem(m.db, actorID, t) {
                return nil
        }
        if t.ParentID == nil || strings.TrimSpace(*t.ParentID) == "" {
                return nil
        }
        parent, ok := m.db.FindItem(*t.ParentID)
        if !ok {
                return nil
        }

        // Destination parent is parent's parent (may be nil/root).
        destParentID := parent.ParentID

        // Compute rank after the parent item in destination siblings.
        sibs := siblingItems(m.db, t.OutlineID, destParentID)
        sibs = filterItems(sibs, func(x *model.Item) bool { return x.ID != t.ID && !x.Archived })
        // Find parent in destination siblings (it should be there).
        refIdx := indexOfItem(sibs, parent.ID)
        if refIdx < 0 {
                // fallback: append
                t.ParentID = destParentID
                t.Rank = nextSiblingRank(m.db, t.OutlineID, destParentID)
        } else {
                lower := sibs[refIdx].Rank
                upper := ""
                if refIdx+1 < len(sibs) {
                        upper = sibs[refIdx+1].Rank
                }
                r, err := store.RankBetween(lower, upper)
                if err != nil {
                        return err
                }
                t.ParentID = destParentID
                t.Rank = r
        }
        t.UpdatedAt = time.Now().UTC()
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        payload := map[string]any{"rank": t.Rank}
        if destParentID == nil {
                payload["parent"] = "none"
        } else {
                payload["parent"] = *destParentID
        }
        _ = m.store.AppendEvent(actorID, "item.set_parent", t.ID, payload)
        m.captureStoreModTimes()
        m.showMinibuffer("Outdented " + t.ID)
        if m.selectedOutline != nil {
                m.refreshItems(*m.selectedOutline)
                selectListItemByID(&m.itemsList, t.ID)
        }
        return nil
}

func canEditItem(db *store.DB, actorID string, t *model.Item) bool {
        if t.OwnerActorID == actorID {
                return true
        }

        // Human override: a human user can edit items owned by their own agents.
        if actorHuman, ok := db.HumanUserIDForActor(actorID); ok {
                if ownerHuman, ok := db.HumanUserIDForActor(t.OwnerActorID); ok && actorHuman == ownerHuman {
                        owner, _ := db.FindActor(t.OwnerActorID)
                        if owner != nil && owner.Kind == model.ActorKindAgent {
                                return true
                        }
                }
        }

        if t.OwnerDelegatedFrom == nil || t.OwnerDelegatedAt == nil {
                return false
        }
        if *t.OwnerDelegatedFrom != actorID {
                return false
        }
        return time.Now().UTC().Before(t.OwnerDelegatedAt.Add(assignGraceDuration()))
}

func assignGraceDuration() time.Duration {
        // Default 1 hour. Override with CLARITY_ASSIGN_GRACE_SECONDS.
        if s := os.Getenv("CLARITY_ASSIGN_GRACE_SECONDS"); s != "" {
                if n, err := strconv.Atoi(s); err == nil && n >= 0 {
                        return time.Duration(n) * time.Second
                }
        }
        return 1 * time.Hour
}

func sameParent(a, b *string) bool {
        if a == nil && b == nil {
                return true
        }
        if a == nil || b == nil {
                return false
        }
        return *a == *b
}

func siblingItems(db *store.DB, outlineID string, parentID *string) []*model.Item {
        var out []*model.Item
        for i := range db.Items {
                it := &db.Items[i]
                if it.OutlineID != outlineID {
                        continue
                }
                if !sameParent(it.ParentID, parentID) {
                        continue
                }
                out = append(out, it)
        }
        sort.Slice(out, func(i, j int) bool { return compareOutlineItems(*out[i], *out[j]) < 0 })
        return out
}

func filterItems(in []*model.Item, keep func(*model.Item) bool) []*model.Item {
        out := make([]*model.Item, 0, len(in))
        for _, it := range in {
                if keep(it) {
                        out = append(out, it)
                }
        }
        return out
}

func indexOfItem(items []*model.Item, id string) int {
        for i := range items {
                if items[i].ID == id {
                        return i
                }
        }
        return -1
}

func isAncestor(db *store.DB, id, maybeAncestor string) bool {
        cur := id
        for i := 0; i < 256; i++ {
                it, ok := db.FindItem(cur)
                if !ok || it.ParentID == nil || strings.TrimSpace(*it.ParentID) == "" {
                        return false
                }
                if *it.ParentID == maybeAncestor {
                        return true
                }
                cur = *it.ParentID
        }
        return true
}

func nextSiblingRank(db *store.DB, outlineID string, parentID *string) string {
        // Append to end of sibling list.
        max := ""
        for _, t := range db.Items {
                if t.OutlineID != outlineID {
                        continue
                }
                if !sameParent(t.ParentID, parentID) {
                        continue
                }
                r := strings.TrimSpace(t.Rank)
                if r != "" && r > max {
                        max = r
                }
        }
        if max == "" {
                r, err := store.RankInitial()
                if err != nil {
                        return "h"
                }
                return r
        }
        r, err := store.RankAfter(max)
        if err != nil {
                return max + "0"
        }
        return r
}
