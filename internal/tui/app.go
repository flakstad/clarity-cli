package tui

import (
        "errors"
        "fmt"
        "os"
        "path/filepath"
        "sort"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/perm"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/bubbles/list"
        "github.com/charmbracelet/bubbles/textarea"
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
        viewItem
)

type reloadTickMsg struct{}

type escTimeoutMsg struct{}

type resizeDoneMsg struct{ seq int }

type flashDoneMsg struct{ seq int }

type previewComputeMsg struct {
        seq    int
        itemID string
        w      int
        h      int
}

type previewDebug struct {
        lastAt        time.Time
        lastItemID    string
        lastW         int
        lastH         int
        lastDur       time.Duration
        lastCacheLen  int
        lastTitleLen  int
        lastDescLen   int
        lastChildN    int
        lastCommentN  int
        lastWorklogN  int
        lastErr       string
        lastReason    string
        lastTickSkips int
}

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
        modalNewProject
        modalRenameProject
        modalNewOutline
        modalEditTitle
        modalEditOutlineName
        modalPickStatus
        modalAddComment
        modalAddWorklog
        modalActionPanel
)

type actionPanelKind int

const (
        actionPanelContext actionPanelKind = iota
        actionPanelNav
        actionPanelAgenda
        actionPanelCapture
)

type actionPanelAction struct {
        // label is displayed in the panel.
        label string
        // kind indicates whether the action executes something or navigates to a subpanel.
        kind actionPanelActionKind
        // next is used when kind == actionPanelActionNav.
        next actionPanelKind
        // handler runs when kind == actionPanelActionExec and the action is not a simple
        // "forward to existing key handler" action.
        handler func(appModel) (appModel, tea.Cmd)
}

type actionPanelActionKind int

const (
        actionPanelActionExec actionPanelActionKind = iota
        actionPanelActionNav
)

type archiveTarget int

const (
        archiveTargetItem archiveTarget = iota
        archiveTargetOutline
        archiveTargetProject
)

type textModalFocus int

const (
        textFocusBody textModalFocus = iota
        textFocusSave
        textFocusCancel
)

type appModel struct {
        dir   string
        store store.Store
        db    *store.DB

        width  int
        height int

        // We treat the very first WindowSizeMsg as "initial sizing" rather than a user-driven
        // resize. Otherwise we briefly render the full-height "Resizing…" overlay on startup.
        seenWindowSize bool

        view view

        projectsList list.Model
        outlinesList list.Model
        itemsList    list.Model
        statusList   list.Model

        selectedProjectID string
        selectedOutlineID string
        selectedOutline   *model.Outline

        pane                pane
        showPreview         bool
        openItemID          string
        collapsed           map[string]bool
        collapseInitialized bool

        modal      modalKind
        modalForID string
        archiveFor archiveTarget
        input      textinput.Model
        textarea   textarea.Model
        textFocus  textModalFocus

        actionPanelStack []actionPanelKind

        pendingEsc bool

        resizing  bool
        resizeSeq int

        flashItemID string
        flashKind   string
        flashSeq    int

        previewSeq        int
        previewCacheForID string
        previewCacheW     int
        previewCacheH     int
        previewCache      string

        debugEnabled bool
        debugOverlay bool
        debugLogPath string
        previewDbg   previewDebug

        lastDBModTime     time.Time
        lastEventsModTime time.Time

        minibufferText string
}

func (m *appModel) openActionPanel(kind actionPanelKind) {
        if m == nil {
                return
        }
        m.modal = modalActionPanel
        m.actionPanelStack = []actionPanelKind{kind}
        m.pendingEsc = false
}

func (m *appModel) closeActionPanel() {
        if m == nil {
                return
        }
        if m.modal == modalActionPanel {
                m.modal = modalNone
                m.actionPanelStack = nil
                m.pendingEsc = false
        }
}

func (m *appModel) pushActionPanel(kind actionPanelKind) {
        if m == nil {
                return
        }
        if m.modal != modalActionPanel {
                return
        }
        m.actionPanelStack = append(m.actionPanelStack, kind)
}

func (m *appModel) popActionPanel() {
        if m == nil {
                return
        }
        if m.modal != modalActionPanel {
                return
        }
        if len(m.actionPanelStack) <= 1 {
                m.closeActionPanel()
                return
        }
        m.actionPanelStack = m.actionPanelStack[:len(m.actionPanelStack)-1]
}

func (m appModel) curActionPanelKind() actionPanelKind {
        if len(m.actionPanelStack) == 0 {
                return actionPanelContext
        }
        return m.actionPanelStack[len(m.actionPanelStack)-1]
}

type actionPanelEntry struct {
        key   string
        label string
}

func actionPanelDisplayKey(k string) string {
        switch k {
        case " ":
                return "SPACE"
        default:
                return k
        }
}

func (m appModel) actionPanelTitle() string {
        switch m.curActionPanelKind() {
        case actionPanelNav:
                return "Navigate"
        case actionPanelAgenda:
                return "Agenda"
        case actionPanelCapture:
                return "Capture"
        default:
                return "Actions"
        }
}

func (m appModel) actionPanelActions() map[string]actionPanelAction {
        cur := m.curActionPanelKind()
        actions := map[string]actionPanelAction{}

        // Always-available global actions.
        actions["a"] = actionPanelAction{label: "Agenda…", kind: actionPanelActionNav, next: actionPanelAgenda}
        actions["c"] = actionPanelAction{label: "Capture…", kind: actionPanelActionNav, next: actionPanelCapture}

        switch cur {
        case actionPanelNav:
                // Nav destinations are only shown when they can work "right now".
                actions["p"] = actionPanelAction{
                        label: "Projects",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                mm.view = viewProjects
                                mm.showPreview = false
                                mm.openItemID = ""
                                mm.pane = paneOutline
                                mm.refreshProjects()
                                return mm, nil
                        },
                }

                // Outlines (requires a project context).
                projID := strings.TrimSpace(m.selectedProjectID)
                if projID == "" && m.db != nil {
                        projID = strings.TrimSpace(m.db.CurrentProjectID)
                }
                if projID != "" {
                        actions["o"] = actionPanelAction{
                                label: "Outlines (current project)",
                                kind:  actionPanelActionExec,
                                handler: func(mm appModel) (appModel, tea.Cmd) {
                                        target := strings.TrimSpace(mm.selectedProjectID)
                                        if target == "" && mm.db != nil {
                                                target = strings.TrimSpace(mm.db.CurrentProjectID)
                                        }
                                        if target == "" {
                                                mm.showMinibuffer("No project selected")
                                                return mm, nil
                                        }
                                        mm.view = viewOutlines
                                        mm.showPreview = false
                                        mm.openItemID = ""
                                        mm.pane = paneOutline
                                        mm.selectedProjectID = target
                                        mm.refreshOutlines(target)
                                        return mm, nil
                                },
                        }
                }

                // Outline (requires an outline context).
                outID := strings.TrimSpace(m.selectedOutlineID)
                if outID != "" {
                        actions["l"] = actionPanelAction{
                                label: "Outline (current)",
                                kind:  actionPanelActionExec,
                                handler: func(mm appModel) (appModel, tea.Cmd) {
                                        target := strings.TrimSpace(mm.selectedOutlineID)
                                        if target == "" {
                                                mm.showMinibuffer("No outline selected")
                                                return mm, nil
                                        }
                                        mm.view = viewOutline
                                        mm.showPreview = false
                                        mm.openItemID = ""
                                        mm.pane = paneOutline
                                        if mm.db != nil {
                                                if o, ok := mm.db.FindOutline(target); ok {
                                                        mm.selectedOutline = o
                                                        mm.refreshItems(*o)
                                                }
                                        }
                                        return mm, nil
                                },
                        }
                }

                // Full-screen item (requires an open item).
                itemID := strings.TrimSpace(m.openItemID)
                if itemID != "" {
                        actions["i"] = actionPanelAction{
                                label: "Item (open)",
                                kind:  actionPanelActionExec,
                                handler: func(mm appModel) (appModel, tea.Cmd) {
                                        if strings.TrimSpace(mm.openItemID) == "" {
                                                mm.showMinibuffer("No item open")
                                                return mm, nil
                                        }
                                        mm.view = viewItem
                                        mm.showPreview = false
                                        mm.pane = paneOutline
                                        return mm, nil
                                },
                        }
                }

        case actionPanelAgenda:
                actions["g"] = actionPanelAction{label: "Navigate…", kind: actionPanelActionNav, next: actionPanelNav}
                actions["t"] = actionPanelAction{
                        label: "Today's agenda (coming soon)",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                mm.showMinibuffer("Agenda: coming soon")
                                return mm, nil
                        },
                }
                actions["w"] = actionPanelAction{
                        label: "This week (coming soon)",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                mm.showMinibuffer("Agenda: coming soon")
                                return mm, nil
                        },
                }
                actions["s"] = actionPanelAction{
                        label: "Search (coming soon)",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                mm.showMinibuffer("Agenda: coming soon")
                                return mm, nil
                        },
                }

        case actionPanelCapture:
                actions["g"] = actionPanelAction{label: "Navigate…", kind: actionPanelActionNav, next: actionPanelNav}
                actions["q"] = actionPanelAction{
                        label: "Quick capture (coming soon)",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                mm.showMinibuffer("Capture: coming soon")
                                return mm, nil
                        },
                }
                actions["t"] = actionPanelAction{
                        label: "Templates… (coming soon)",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                mm.showMinibuffer("Capture templates: coming soon")
                                return mm, nil
                        },
                }

        default:
                // Contextual (depends on current view/pane).
                actions["g"] = actionPanelAction{label: "Navigate…", kind: actionPanelActionNav, next: actionPanelNav}

                switch m.view {
                case viewProjects:
                        actions["enter"] = actionPanelAction{label: "Select project", kind: actionPanelActionExec}
                        actions["n"] = actionPanelAction{label: "New project", kind: actionPanelActionExec}
                        actions["e"] = actionPanelAction{label: "Rename project", kind: actionPanelActionExec}
                        actions["r"] = actionPanelAction{label: "Archive project", kind: actionPanelActionExec}
                        actions["q"] = actionPanelAction{label: "Quit", kind: actionPanelActionExec}
                case viewOutlines:
                        actions["enter"] = actionPanelAction{label: "Select outline", kind: actionPanelActionExec}
                        actions["n"] = actionPanelAction{label: "New outline", kind: actionPanelActionExec}
                        actions["e"] = actionPanelAction{label: "Rename outline", kind: actionPanelActionExec}
                        actions["r"] = actionPanelAction{label: "Archive outline", kind: actionPanelActionExec}
                        actions["q"] = actionPanelAction{label: "Quit", kind: actionPanelActionExec}
                case viewItem:
                        actions["y"] = actionPanelAction{label: "Copy item ID", kind: actionPanelActionExec}
                        actions["Y"] = actionPanelAction{label: "Copy CLI show command", kind: actionPanelActionExec}
                        actions["r"] = actionPanelAction{label: "Archive item", kind: actionPanelActionExec}
                        actions["q"] = actionPanelAction{label: "Quit", kind: actionPanelActionExec}
                case viewOutline:
                        actions["enter"] = actionPanelAction{label: "Open item", kind: actionPanelActionExec}
                        actions["o"] = actionPanelAction{label: "Toggle preview", kind: actionPanelActionExec}
                        if m.splitPreviewVisible() {
                                actions["tab"] = actionPanelAction{label: "Toggle focus (outline/detail)", kind: actionPanelActionExec}
                        }
                        actions["z"] = actionPanelAction{label: "Toggle collapse", kind: actionPanelActionExec}
                        actions["Z"] = actionPanelAction{label: "Collapse/expand all", kind: actionPanelActionExec}
                        actions["y"] = actionPanelAction{label: "Copy item ID", kind: actionPanelActionExec}
                        actions["Y"] = actionPanelAction{label: "Copy CLI show command", kind: actionPanelActionExec}
                        actions["C"] = actionPanelAction{label: "Add comment", kind: actionPanelActionExec}
                        actions["w"] = actionPanelAction{label: "Add worklog", kind: actionPanelActionExec}
                        actions["r"] = actionPanelAction{label: "Archive item", kind: actionPanelActionExec}
                        actions["q"] = actionPanelAction{label: "Quit", kind: actionPanelActionExec}

                        // Mutations are outline-pane only today.
                        if m.pane == paneOutline {
                                actions["e"] = actionPanelAction{label: "Edit title", kind: actionPanelActionExec}
                                actions["n"] = actionPanelAction{label: "New sibling", kind: actionPanelActionExec}
                                actions["N"] = actionPanelAction{label: "New child", kind: actionPanelActionExec}
                                actions[" "] = actionPanelAction{label: "Change status", kind: actionPanelActionExec}
                                actions["shift+left"] = actionPanelAction{label: "Cycle status (prev)", kind: actionPanelActionExec}
                                actions["shift+right"] = actionPanelAction{label: "Cycle status (next)", kind: actionPanelActionExec}
                        }
                }
        }

        return actions
}

func (m appModel) actionPanelEntries() []actionPanelEntry {
        actions := m.actionPanelActions()
        entries := make([]actionPanelEntry, 0, len(actions))
        for k, a := range actions {
                entries = append(entries, actionPanelEntry{key: k, label: a.label})
        }
        sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
        return entries
}

const (
        topPadLines      = 1
        breadcrumbGap    = 1
        maxContentW      = 96
        minSplitPreviewW = 80
        splitGapW        = 2
        splitOuterMargin = 2
)

func newAppModel(dir string, db *store.DB) appModel {
        s := store.Store{Dir: dir}
        m := appModel{
                dir:   dir,
                store: s,
                db:    db,
                view:  viewProjects,
                pane:  paneOutline,
        }

        if strings.TrimSpace(os.Getenv("CLARITY_TUI_DEBUG")) != "" {
                m.debugEnabled = true
                m.debugOverlay = true
        }
        m.debugLogPath = strings.TrimSpace(os.Getenv("CLARITY_TUI_DEBUG_LOG"))

        m.projectsList = newList("Projects", "Select a project", []list.Item{})
        m.projectsList.SetDelegate(newCompactItemDelegate())
        m.outlinesList = newList("Outlines", "Select an outline", []list.Item{})
        m.outlinesList.SetDelegate(newCompactItemDelegate())
        m.itemsList = newList("Outline", "Navigate items (split view)", []list.Item{})
        m.itemsList.SetDelegate(newOutlineItemDelegate())
        m.itemsList.SetFilteringEnabled(false)
        m.itemsList.SetShowFilter(false)

        m.statusList = newList("Status", "Select a status", []list.Item{})
        m.statusList.SetDelegate(newCompactItemDelegate())
        m.statusList.SetFilteringEnabled(false)
        m.statusList.SetShowFilter(false)
        m.statusList.SetShowHelp(false)
        m.statusList.SetShowStatusBar(false)
        m.statusList.SetShowPagination(false)

        m.input = textinput.New()
        m.input.Placeholder = "Title"
        m.input.CharLimit = 200
        m.input.Width = 40

        m.textarea = textarea.New()
        m.textarea.Placeholder = "Write…"
        m.textarea.CharLimit = 0
        m.textarea.SetWidth(72)
        m.textarea.SetHeight(10)
        m.textarea.ShowLineNumbers = false

        m.refreshProjects()
        m.captureStoreModTimes()
        return m
}

func (m *appModel) debugLogf(format string, args ...any) {
        if m == nil || !m.debugEnabled {
                return
        }
        path := strings.TrimSpace(m.debugLogPath)
        if path == "" {
                return
        }
        line := fmt.Sprintf(format, args...)
        ts := time.Now().Format("15:04:05.000")
        _ = os.MkdirAll(filepath.Dir(path), 0o755)
        f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
        if err != nil {
                return
        }
        defer f.Close()
        _, _ = fmt.Fprintln(f, ts+" "+line)
}

func (m appModel) debugOverlayView() string {
        if !m.debugEnabled || !m.debugOverlay {
                return ""
        }
        p := m.previewDbg
        if p.lastAt.IsZero() {
                return ""
        }
        body := strings.Join([]string{
                "DEBUG (toggle with D)",
                fmt.Sprintf("last preview: %s  dur=%s  item=%s  size=%dx%d",
                        p.lastAt.Format("15:04:05.000"), p.lastDur, p.lastItemID, p.lastW, p.lastH),
                fmt.Sprintf("lens: title=%d desc=%d cache=%d", p.lastTitleLen, p.lastDescLen, p.lastCacheLen),
                fmt.Sprintf("counts: children=%d comments=%d worklog=%d", p.lastChildN, p.lastCommentN, p.lastWorklogN),
                func() string {
                        if strings.TrimSpace(p.lastErr) == "" {
                                return "err: (none)"
                        }
                        return "err: " + p.lastErr
                }(),
                func() string {
                        if strings.TrimSpace(p.lastReason) == "" {
                                return ""
                        }
                        return "note: " + p.lastReason
                }(),
        }, "\n")
        box := lipgloss.NewStyle().
                Border(lipgloss.RoundedBorder()).
                BorderForeground(lipgloss.Color("240")).
                Background(lipgloss.Color("235")).
                Foreground(lipgloss.Color("255")).
                Padding(1, 2)
        return box.Render(body)
}

func (m appModel) Init() tea.Cmd { return tickReload() }

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
        switch msg := msg.(type) {
        case tea.WindowSizeMsg:
                m.width = msg.Width
                m.height = msg.Height
                m.resizeLists()
                // Don't show the resize overlay on startup; only after we've seen an initial size.
                if !m.seenWindowSize {
                        m.seenWindowSize = true
                        m.resizing = false
                        return m, nil
                }
                m.resizing = true
                m.resizeSeq++
                seq := m.resizeSeq
                return m, tea.Batch(
                        tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return resizeDoneMsg{seq: seq} }),
                        m.schedulePreviewCompute(),
                )

        case resizeDoneMsg:
                // Debounce: only clear if this corresponds to the latest resize seq.
                if msg.seq == m.resizeSeq {
                        m.resizing = false
                }
                return m, nil

        case flashDoneMsg:
                if msg.seq == m.flashSeq {
                        m.flashItemID = ""
                        m.flashKind = ""
                        if m.view == viewOutline && m.selectedOutline != nil {
                                m.refreshItems(*m.selectedOutline)
                        }
                }
                return m, nil

        case previewComputeMsg:
                // Debounced detail-pane rendering for split preview.
                if msg.seq != m.previewSeq {
                        m.previewDbg.lastTickSkips++
                        return m, nil
                }
                if !m.splitPreviewVisible() {
                        return m, nil
                }
                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                        if strings.TrimSpace(it.row.item.ID) != strings.TrimSpace(msg.itemID) {
                                m.previewDbg.lastReason = "selection changed"
                                return m, nil
                        }
                        start := time.Now()
                        m.previewCacheForID = strings.TrimSpace(msg.itemID)
                        m.previewCacheW = msg.w
                        m.previewCacheH = msg.h
                        m.previewCache = renderItemDetail(m.db, it.outline, it.row.item, msg.w, msg.h, m.pane == paneDetail)
                        dur := time.Since(start)

                        if m.debugEnabled {
                                m.previewDbg.lastAt = time.Now()
                                m.previewDbg.lastItemID = m.previewCacheForID
                                m.previewDbg.lastW = msg.w
                                m.previewDbg.lastH = msg.h
                                m.previewDbg.lastDur = dur
                                m.previewDbg.lastCacheLen = len(m.previewCache)
                                m.previewDbg.lastTitleLen = len(it.row.item.Title)
                                m.previewDbg.lastDescLen = len(it.row.item.Description)
                                m.previewDbg.lastChildN = len(m.db.ChildrenOf(it.row.item.ID))
                                m.previewDbg.lastCommentN = len(m.db.CommentsForItem(it.row.item.ID))
                                m.previewDbg.lastWorklogN = len(m.db.WorklogForItem(it.row.item.ID))
                                m.previewDbg.lastErr = ""
                                m.previewDbg.lastReason = ""

                                // Emit a log line for slow renders.
                                if dur > 250*time.Millisecond {
                                        m.debugLogf("slow preview dur=%s item=%s size=%dx%d lens(title=%d desc=%d cache=%d) counts(child=%d cmt=%d wl=%d)",
                                                dur, m.previewCacheForID, msg.w, msg.h,
                                                m.previewDbg.lastTitleLen, m.previewDbg.lastDescLen, m.previewDbg.lastCacheLen,
                                                m.previewDbg.lastChildN, m.previewDbg.lastCommentN, m.previewDbg.lastWorklogN)
                                        m.showMinibuffer(fmt.Sprintf("Slow preview render: %s (item %s)", dur, m.previewCacheForID))
                                }
                        }
                }
                return m, nil

        case reloadTickMsg:
                if m.storeChanged() {
                        _ = m.reloadFromDisk()
                }
                // Also drive debounced preview rendering so it can't get "stuck loading"
                // after list refreshes that don't originate from a navigation key.
                return m, tea.Batch(tickReload(), m.schedulePreviewCompute())

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
                // If a modal is open, route all keys to the modal handler so text inputs behave
                // normally (e.g. backspace edits).
                if m.modal != modalNone {
                        return m.updateOutline(msg)
                }
                switch msg.String() {
                case "ctrl+c", "q":
                        return m, tea.Quit
                case "x":
                        m.openActionPanel(actionPanelContext)
                        return m, nil
                case "g":
                        m.openActionPanel(actionPanelNav)
                        return m, nil
                case "a":
                        m.openActionPanel(actionPanelAgenda)
                        return m, nil
                case "c":
                        m.openActionPanel(actionPanelCapture)
                        return m, nil
                case "y":
                        if m.view == viewItem && strings.TrimSpace(m.openItemID) != "" {
                                id := strings.TrimSpace(m.openItemID)
                                if err := copyToClipboard(id); err != nil {
                                        m.showMinibuffer("Clipboard error: " + err.Error())
                                } else {
                                        m.showMinibuffer("Copied item ID " + id)
                                }
                                return m, nil
                        }
                case "Y":
                        if m.view == viewItem && strings.TrimSpace(m.openItemID) != "" {
                                id := strings.TrimSpace(m.openItemID)
                                cmd := "clarity items show " + id
                                if err := copyToClipboard(cmd); err != nil {
                                        m.showMinibuffer("Clipboard error: " + err.Error())
                                } else {
                                        m.showMinibuffer("Copied: " + cmd)
                                }
                                return m, nil
                        }
                case "backspace":
                        if m.view == viewItem {
                                m.view = viewOutline
                                m.openItemID = ""
                                m.showPreview = false
                                m.pane = paneOutline
                                if o, ok := m.db.FindOutline(m.selectedOutlineID); ok {
                                        m.refreshItems(*o)
                                }
                                return m, nil
                        }
                        switch m.view {
                        case viewOutline:
                                m.view = viewOutlines
                                m.refreshOutlines(m.selectedProjectID)
                                m.showPreview = false
                                return m, nil
                        case viewOutlines:
                                m.view = viewProjects
                                m.refreshProjects()
                                return m, nil
                        }
                case "esc":
                        if m.view == viewItem {
                                m.view = viewOutline
                                m.openItemID = ""
                                m.showPreview = false
                                m.pane = paneOutline
                                if o, ok := m.db.FindOutline(m.selectedOutlineID); ok {
                                        m.refreshItems(*o)
                                }
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
                                        m.showPreview = false
                                        m.openItemID = ""
                                        m.collapsed = map[string]bool{}
                                        m.collapseInitialized = false
                                        m.refreshItems(it.outline)
                                        return m, nil
                                }
                        }
                case "r":
                        // Archive selected item/project/outline (with confirm; depends on screen).
                        //
                        // Note: item view is otherwise read-only, but archiving is a safe global action.
                        if m.view == viewItem && strings.TrimSpace(m.openItemID) != "" {
                                m.modal = modalConfirmArchive
                                m.modalForID = strings.TrimSpace(m.openItemID)
                                m.archiveFor = archiveTargetItem
                                m.input.Blur()
                                return m, nil
                        }
                        // Archive selected project/outline (with confirm), similar to archiving items.
                        if m.view == viewProjects {
                                if it, ok := m.projectsList.SelectedItem().(projectItem); ok {
                                        m.modal = modalConfirmArchive
                                        m.modalForID = it.project.ID
                                        m.archiveFor = archiveTargetProject
                                        m.input.Blur()
                                        return m, nil
                                }
                        }
                        if m.view == viewOutlines {
                                if it, ok := m.outlinesList.SelectedItem().(outlineItem); ok {
                                        m.modal = modalConfirmArchive
                                        m.modalForID = it.outline.ID
                                        m.archiveFor = archiveTargetOutline
                                        m.input.Blur()
                                        return m, nil
                                }
                        }
                case "e":
                        if m.view == viewProjects {
                                // Rename project.
                                if it, ok := m.projectsList.SelectedItem().(projectItem); ok {
                                        m.modal = modalRenameProject
                                        m.modalForID = it.project.ID
                                        m.input.Placeholder = "Project name"
                                        m.input.SetValue(strings.TrimSpace(it.project.Name))
                                        m.input.Focus()
                                        return m, nil
                                }
                        }
                        if m.view == viewOutlines {
                                // Rename outline.
                                if it, ok := m.outlinesList.SelectedItem().(outlineItem); ok {
                                        m.modal = modalEditOutlineName
                                        m.modalForID = it.outline.ID
                                        name := ""
                                        if it.outline.Name != nil {
                                                name = strings.TrimSpace(*it.outline.Name)
                                        }
                                        m.input.Placeholder = "Outline name (optional)"
                                        m.input.SetValue(name)
                                        m.input.Focus()
                                        return m, nil
                                }
                        }
                case "n":
                        if m.view == viewProjects {
                                // New project.
                                m.modal = modalNewProject
                                m.modalForID = ""
                                m.input.Placeholder = "Project name"
                                m.input.SetValue("")
                                m.input.Focus()
                                return m, nil
                        }
                        if m.view == viewOutlines {
                                // New outline (name optional).
                                m.modal = modalNewOutline
                                m.modalForID = ""
                                m.input.Placeholder = "Outline name (optional)"
                                m.input.SetValue("")
                                m.input.Focus()
                                return m, nil
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
        case viewItem:
                // Read-only for now. Back/quit handled in the root key handler.
                return m, nil
        default:
                return m, nil
        }
}

func (m appModel) View() string {
        if m.resizing {
                // Stable full-height overlay during terminal resize to avoid flicker/layout jumps.
                // Render a single centered "Resizing…" line (instead of repeating it on every row).
                h := m.height
                if h < 0 {
                        h = 0
                }
                w := m.width
                if w < 0 {
                        w = 0
                }

                lines := make([]string, h)
                blank := strings.Repeat(" ", w)
                for i := 0; i < h; i++ {
                        lines[i] = blank
                }
                if h > 0 {
                        mid := h / 2
                        lines[mid] = lipgloss.NewStyle().Width(w).Align(lipgloss.Center).Render("Resizing…")
                }
                return strings.Join(lines, "\n")
        }

        var body string
        switch m.view {
        case viewProjects:
                body = m.viewProjects()
        case viewOutlines:
                body = m.viewOutlines()
        case viewOutline:
                body = m.viewOutline()
        case viewItem:
                body = m.viewItem()
        default:
                body = ""
        }

        footer := m.footerBlock()
        return strings.Join([]string{body, footer}, "\n\n")
}

func (m *appModel) breadcrumbText() string {
        parts := []string{"projects"}
        if m.view == viewProjects {
                return strings.Join(parts, " > ")
        }

        if m.selectedProjectID != "" {
                if p, ok := m.db.FindProject(m.selectedProjectID); ok {
                        if t := strings.TrimSpace(p.Name); t != "" {
                                parts = append(parts, t)
                        } else {
                                parts = append(parts, p.ID)
                        }
                }
        }

        if m.view == viewOutlines {
                return strings.Join(parts, " > ")
        }

        if m.selectedOutlineID != "" {
                if o, ok := m.db.FindOutline(m.selectedOutlineID); ok {
                        name := ""
                        if o.Name != nil {
                                name = strings.TrimSpace(*o.Name)
                        }
                        if name != "" {
                                parts = append(parts, name)
                        } else {
                                parts = append(parts, o.ID)
                        }
                }
        }

        if m.view == viewOutline {
                return strings.Join(parts, " > ")
        }

        if m.view == viewItem {
                if itemID := strings.TrimSpace(m.openItemID); itemID != "" {
                        if it, ok := m.db.FindItem(itemID); ok {
                                if t := strings.TrimSpace(it.Title); t != "" {
                                        parts = append(parts, t)
                                } else {
                                        parts = append(parts, it.ID)
                                }
                        } else {
                                parts = append(parts, itemID)
                        }
                }
        }

        return strings.Join(parts, " > ")
}

func (m *appModel) viewProjects() string {
        frameH := m.height - 6
        if frameH < 8 {
                frameH = 8
        }
        bodyHeight := frameH - (topPadLines + breadcrumbGap + 2)
        if bodyHeight < 6 {
                bodyHeight = 6
        }

        w := m.width
        if w < 10 {
                w = 10
        }

        contentW := w
        if contentW > maxContentW {
                contentW = maxContentW
        }
        m.projectsList.SetSize(contentW, bodyHeight)

        crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
        main := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + m.projectsList.View()
        main = lipgloss.PlaceHorizontal(w, lipgloss.Center, main)
        if m.modal == modalNone {
                return main
        }
        bg := dimBackground(main)
        fg := m.renderModal()
        return overlayCenter(bg, fg, w, frameH)
}

func (m *appModel) viewOutlines() string {
        frameH := m.height - 6
        if frameH < 8 {
                frameH = 8
        }
        bodyHeight := frameH - (topPadLines + breadcrumbGap + 2)
        if bodyHeight < 6 {
                bodyHeight = 6
        }

        w := m.width
        if w < 10 {
                w = 10
        }

        contentW := w
        if contentW > maxContentW {
                contentW = maxContentW
        }
        m.outlinesList.SetSize(contentW, bodyHeight)

        crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
        main := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + m.outlinesList.View()
        main = lipgloss.PlaceHorizontal(w, lipgloss.Center, main)
        if m.modal == modalNone {
                return main
        }
        bg := dimBackground(main)
        fg := m.renderModal()
        return overlayCenter(bg, fg, w, frameH)
}

func (m appModel) footerText() string {
        base := "enter: select  backspace/esc: back  q: quit"
        if m.modal == modalActionPanel {
                return "action: type a key  backspace/esc: back  ctrl+g: close"
        }
        if m.view == viewItem {
                return "backspace/esc: back  q: quit  y/Y: copy"
        }
        if m.view == viewProjects && m.modal == modalNone {
                return "enter: select  n: new  e: rename  r: archive  q: quit"
        }
        if m.view == viewOutlines && m.modal == modalNone {
                return "enter: select  n: new  e: rename  r: archive  backspace/esc: back  q: quit"
        }
        if m.modal == modalNewProject {
                return "new project: type, enter: save, esc: cancel"
        }
        if m.modal == modalRenameProject {
                return "rename project: type, enter: save, esc: cancel"
        }
        if m.modal == modalNewOutline {
                return "new outline: type (optional), enter: save, esc: cancel"
        }
        if m.modal == modalEditOutlineName {
                return "rename outline: type, enter: save, esc: cancel"
        }
        if m.view != viewOutline {
                return base
        }
        if m.modal != modalNone {
                if m.modal == modalConfirmArchive {
                        return "archive: y/enter: confirm  n/esc: cancel"
                }
                if m.modal == modalPickStatus {
                        return "status: enter: set  esc: cancel"
                }
                if m.modal == modalEditTitle {
                        return "edit title: type, enter: save, esc: cancel"
                }
                if m.modal == modalEditOutlineName {
                        return "rename outline: type, enter: save, esc: cancel"
                }
                if m.modal == modalAddComment {
                        return "comment: tab: focus  ctrl+s: save  esc: cancel"
                }
                if m.modal == modalAddWorklog {
                        return "worklog: tab: focus  ctrl+s: save  esc: cancel"
                }
                return "new item: type title, enter: save, esc: cancel"
        }
        if m.splitPreviewVisible() {
                return "enter: open  o: preview  tab: toggle focus  arrows/jk/ctrl+n/p/h/l/ctrl+b/f: navigate  alt+arrows: move/indent/outdent  z/Z: collapse  n/N: add  e: edit title  space: status  Shift+←/→: cycle status  C: comment  w: worklog  r: archive  y/Y: copy"
        }
        return "enter: open  o: preview  arrows/jk/ctrl+n/p/h/l/ctrl+b/f: navigate  alt+arrows: move/indent/outdent  z/Z: collapse  n/N: add  e: edit title  space: status  Shift+←/→: cycle status  C: comment  w: worklog  r: archive  y/Y: copy"
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

func (m appModel) renderActionPanel() string {
        title := m.actionPanelTitle()
        entries := m.actionPanelEntries()
        if len(entries) == 0 {
                return renderModalBox(m.width, title, "(no actions)")
        }

        // Group global actions first, then nav, then everything else.
        globalOrder := []string{"a", "c"}
        navOrder := []string{"g", "p", "o", "l", "i"}
        seen := map[string]bool{}
        lines := []string{}

        actions := m.actionPanelActions()
        addKey := func(k string) {
                if seen[k] {
                        return
                }
                a, ok := actions[k]
                if !ok {
                        return
                }
                seen[k] = true
                lines = append(lines, fmt.Sprintf("%-12s %s", actionPanelDisplayKey(k), a.label))
        }

        for _, k := range globalOrder {
                addKey(k)
        }
        if len(lines) > 0 {
                lines = append(lines, "")
        }
        for _, k := range navOrder {
                addKey(k)
        }
        if len(lines) > 0 && lines[len(lines)-1] != "" {
                lines = append(lines, "")
        }

        rest := make([]string, 0, len(entries))
        for _, e := range entries {
                if seen[e.key] {
                        continue
                }
                rest = append(rest, e.key)
        }
        sort.Strings(rest)
        for _, k := range rest {
                addKey(k)
        }

        body := strings.Join(lines, "\n")
        body += "\n\nbackspace/esc: back    ctrl+g: close"
        return renderModalBox(m.width, title, body)
}

func (m *appModel) reportError(itemID string, err error) tea.Cmd {
        if m == nil || err == nil {
                return nil
        }
        msg := strings.TrimSpace(err.Error())
        if msg == "" {
                msg = "unknown error"
        }
        m.showMinibuffer("Error: " + msg)
        if strings.TrimSpace(itemID) == "" {
                return nil
        }
        m.flashSeq++
        seq := m.flashSeq
        m.flashItemID = strings.TrimSpace(itemID)
        m.flashKind = "error"
        if m.view == viewOutline && m.selectedOutline != nil {
                m.refreshItems(*m.selectedOutline)
        }
        return tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg { return flashDoneMsg{seq: seq} })
}

func findPrevRankLessThan(items []*model.Item, fromIdx int, upper string) string {
        upper = strings.TrimSpace(upper)
        for i := fromIdx - 1; i >= 0; i-- {
                r := strings.TrimSpace(items[i].Rank)
                if r == "" || upper == "" {
                        continue
                }
                if r < upper {
                        return r
                }
        }
        return ""
}

func findNextRankGreaterThan(items []*model.Item, fromIdx int, lower string) string {
        lower = strings.TrimSpace(lower)
        for i := fromIdx + 1; i < len(items); i++ {
                r := strings.TrimSpace(items[i].Rank)
                if r == "" || lower == "" {
                        continue
                }
                if r > lower {
                        return r
                }
        }
        return ""
}

// rankBoundsForInsert returns (lower, upper) bounds suitable for store.RankBetween.
// It intentionally skips duplicate/out-of-order neighbor ranks, and may return an empty bound
// (meaning open-ended) to avoid errors. This updates only the moved item.
func rankBoundsForInsert(sibs []*model.Item, afterID, beforeID string) (lower, upper string, ok bool) {
        afterID = strings.TrimSpace(afterID)
        beforeID = strings.TrimSpace(beforeID)
        if (afterID == "" && beforeID == "") || (afterID != "" && beforeID != "") {
                return "", "", false
        }
        if beforeID != "" {
                refIdx := indexOfItem(sibs, beforeID)
                if refIdx < 0 {
                        return "", "", false
                }
                upper = sibs[refIdx].Rank
                lower = findPrevRankLessThan(sibs, refIdx, upper)
                return lower, upper, true
        }
        refIdx := indexOfItem(sibs, afterID)
        if refIdx < 0 {
                return "", "", false
        }
        lower = sibs[refIdx].Rank
        upper = findNextRankGreaterThan(sibs, refIdx, lower)
        return lower, upper, true
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
        centeredW := w
        if centeredW > maxContentW {
                centeredW = maxContentW
        }
        m.projectsList.SetSize(centeredW, h)
        m.outlinesList.SetSize(centeredW, h)
        if m.splitPreviewVisible() {
                leftW, _ := splitPaneWidths(centeredW)
                m.itemsList.SetSize(leftW, h)
        } else {
                m.itemsList.SetSize(centeredW, h)
        }
}

func (m *appModel) splitPreviewVisible() bool {
        if m == nil {
                return false
        }
        if !m.showPreview {
                return false
        }
        return m.width >= minSplitPreviewW
}

func splitPaneWidths(contentW int) (leftW, rightW int) {
        if contentW < 10 {
                contentW = 10
        }
        avail := contentW - splitGapW
        if avail < 8 {
                avail = 8
        }
        leftW = avail / 3
        if leftW < 20 {
                leftW = 20
        }
        if leftW > avail-20 {
                leftW = avail - 20
        }
        rightW = avail - leftW
        return leftW, rightW
}

func (m *appModel) outlineLayout() (frameH, bodyH, contentW int) {
        frameH = m.height - 6
        if frameH < 8 {
                frameH = 8
        }
        bodyH = frameH - (topPadLines + breadcrumbGap + 2)
        if bodyH < 6 {
                bodyH = 6
        }
        w := m.width
        if w < 10 {
                w = 10
        }
        contentW = w
        // In split view we use full width (with outer margins) instead of centering to maxContentW.
        if m.splitPreviewVisible() {
                contentW = w - 2*splitOuterMargin
        } else if contentW > maxContentW {
                contentW = maxContentW
        }
        if contentW < 10 {
                contentW = 10
        }
        return frameH, bodyH, contentW
}

func (m *appModel) schedulePreviewCompute() tea.Cmd {
        if !m.splitPreviewVisible() {
                return nil
        }
        it, ok := m.itemsList.SelectedItem().(outlineRowItem)
        if !ok {
                return nil
        }
        _, bodyH, contentW := m.outlineLayout()
        _, rightW := splitPaneWidths(contentW)
        itemID := strings.TrimSpace(it.row.item.ID)
        if itemID == "" {
                return nil
        }
        // If the cache already matches, nothing to do.
        if m.previewCacheForID == itemID && m.previewCacheW == rightW && m.previewCacheH == bodyH && strings.TrimSpace(m.previewCache) != "" {
                return nil
        }
        m.previewSeq++
        seq := m.previewSeq
        return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg {
                return previewComputeMsg{seq: seq, itemID: itemID, w: rightW, h: bodyH}
        })
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
                if p.Archived {
                        continue
                }
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
                        if o.Archived {
                                continue
                        }
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
        title := "Outline"
        if outline.Name != nil {
                if t := strings.TrimSpace(*outline.Name); t != "" {
                        title = t
                }
        }
        m.itemsList.Title = title
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

        flat := flattenOutline(outline, its, m.collapsed)
        var items []list.Item
        for _, row := range flat {
                flash := ""
                if m.flashKind != "" && m.flashItemID != "" && row.item.ID == m.flashItemID {
                        flash = m.flashKind
                }
                items = append(items, outlineRowItem{row: row, outline: outline, flashKind: flash})
        }
        // Always-present affordance for adding an item (useful for empty outlines).
        items = append(items, addItemRow{})
        m.itemsList.SetItems(items)
        if curID != "" {
                selectListItemByID(&m.itemsList, curID)
        }
}

func (m *appModel) viewOutline() string {
        frameH, bodyHeight, contentW := m.outlineLayout()
        w := m.width
        if w < 10 {
                w = 10
        }

        crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())

        var body string
        if !m.splitPreviewVisible() {
                m.itemsList.SetSize(contentW, bodyHeight)
                body = m.itemsList.View()
        } else {
                leftW, rightW := splitPaneWidths(contentW)
                m.itemsList.SetSize(leftW, bodyHeight)
                left := m.itemsList.View()

                placeholder := lipgloss.NewStyle().Width(rightW).Height(bodyHeight).Padding(0, 1).Render("(loading…)")
                right := placeholder
                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                        id := strings.TrimSpace(it.row.item.ID)
                        if id == "" {
                                right = lipgloss.NewStyle().Width(rightW).Height(bodyHeight).Padding(0, 1).Render("(select an item)")
                        } else if m.previewCacheForID == id && m.previewCacheW == rightW && m.previewCacheH == bodyHeight && strings.TrimSpace(m.previewCache) != "" {
                                right = m.previewCache
                        } else if m.previewCacheW == rightW && m.previewCacheH == bodyHeight && strings.TrimSpace(m.previewCache) != "" {
                                // Avoid "blinking" a loading placeholder when navigating quickly:
                                // keep showing the last rendered detail pane until the new one is ready.
                                right = m.previewCache
                        }
                }

                body = lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", splitGapW), right)
                // Ensure exact width for stable centering.
                body = lipgloss.NewStyle().Width(contentW).Height(bodyHeight).Render(body)
        }

        main := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + body
        if !m.splitPreviewVisible() {
                // Single-pane view stays centered at maxContentW.
                main = lipgloss.PlaceHorizontal(w, lipgloss.Center, main)
        } else {
                // Split view uses full terminal width with a small outer margin.
                main = lipgloss.NewStyle().Width(w).Padding(0, splitOuterMargin).Render(main)
        }

        if m.modal == modalNone {
                if m.debugEnabled && m.debugOverlay {
                        ov := m.debugOverlayView()
                        if strings.TrimSpace(ov) != "" {
                                main = overlayCenter(main, ov, w, frameH)
                        }
                }
                return main
        }
        bg := dimBackground(main)
        fg := m.renderModal()
        return overlayCenter(bg, fg, w, frameH)
}

func (m *appModel) viewItem() string {
        frameH := m.height - 6
        if frameH < 8 {
                frameH = 8
        }
        bodyHeight := frameH - (topPadLines + breadcrumbGap + 2)
        if bodyHeight < 6 {
                bodyHeight = 6
        }

        w := m.width
        if w < 10 {
                w = 10
        }

        contentW := w
        if contentW > maxContentW {
                contentW = maxContentW
        }

        itemID := strings.TrimSpace(m.openItemID)
        if itemID == "" {
                crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
                msg := lipgloss.NewStyle().Width(contentW).Height(bodyHeight).Render("No item selected.")
                block := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + msg
                return lipgloss.PlaceHorizontal(w, lipgloss.Center, block)
        }

        outline, ok := m.db.FindOutline(m.selectedOutlineID)
        if !ok {
                crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
                msg := lipgloss.NewStyle().Width(contentW).Height(bodyHeight).Render("Outline not found.")
                block := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + msg
                return lipgloss.PlaceHorizontal(w, lipgloss.Center, block)
        }

        it, ok := m.db.FindItem(itemID)
        if !ok {
                crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
                msg := lipgloss.NewStyle().Width(contentW).Height(bodyHeight).Render("Item not found.")
                block := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + msg
                return lipgloss.PlaceHorizontal(w, lipgloss.Center, block)
        }

        crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
        card := renderItemDetail(m.db, *outline, *it, contentW, bodyHeight, true)
        block := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + card
        return lipgloss.PlaceHorizontal(w, lipgloss.Center, block)
}

func (m *appModel) renderModal() string {
        switch m.modal {
        case modalActionPanel:
                return m.renderActionPanel()
        case modalNewSibling, modalNewChild:
                title := "New item"
                if m.modal == modalNewChild {
                        title = "New subitem"
                }
                return renderModalBox(m.width, title, m.input.View()+"\n\nenter: save   esc: cancel")
        case modalNewProject:
                return renderModalBox(m.width, "New project", m.input.View()+"\n\nenter: save   esc: cancel")
        case modalRenameProject:
                return renderModalBox(m.width, "Rename project", m.input.View()+"\n\nenter: save   esc: cancel")
        case modalNewOutline:
                return renderModalBox(m.width, "New outline", m.input.View()+"\n\nenter: save   esc: cancel")
        case modalEditTitle:
                return renderModalBox(m.width, "Edit title", m.input.View()+"\n\nenter: save   esc: cancel")
        case modalEditOutlineName:
                return renderModalBox(m.width, "Rename outline", m.input.View()+"\n\nenter: save   esc: cancel")
        case modalPickStatus:
                return renderModalBox(m.width, "Set status", m.statusList.View()+"\n\nenter: set   esc: cancel")
        case modalAddComment:
                return m.renderTextAreaModal("Add comment")
        case modalAddWorklog:
                return m.renderTextAreaModal("Add worklog")
        case modalConfirmArchive:
                title := ""
                cascade := ""
                switch m.archiveFor {
                case archiveTargetProject:
                        title = "this project"
                        if p, ok := m.db.FindProject(m.modalForID); ok {
                                if strings.TrimSpace(p.Name) != "" {
                                        title = fmt.Sprintf("%q", p.Name)
                                }
                        }
                        outN := countUnarchivedOutlinesInProject(m.db, m.modalForID)
                        itemN := countUnarchivedItemsInProject(m.db, m.modalForID)
                        cascade = fmt.Sprintf("This will archive this project, %d outline(s), and %d item(s).", outN, itemN)

                case archiveTargetOutline:
                        title = "this outline"
                        if o, ok := m.db.FindOutline(m.modalForID); ok {
                                name := ""
                                if o.Name != nil {
                                        name = strings.TrimSpace(*o.Name)
                                }
                                if name != "" {
                                        title = fmt.Sprintf("%q", name)
                                }
                        }
                        itemN := countUnarchivedItemsInOutline(m.db, m.modalForID)
                        cascade = fmt.Sprintf("This will archive this outline and %d item(s).", itemN)

                default:
                        // archiveTargetItem
                        title = "this item"
                        if it, ok := m.db.FindItem(m.modalForID); ok {
                                if strings.TrimSpace(it.Title) != "" {
                                        title = fmt.Sprintf("%q", it.Title)
                                }
                        }
                        extra := countUnarchivedDescendants(m.db, m.modalForID)
                        cascade = "This will archive this item."
                        if extra == 1 {
                                cascade = "This will archive this item and 1 subitem."
                        } else if extra > 1 {
                                cascade = fmt.Sprintf("This will archive this item and %d subitems.", extra)
                        }
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

func (m *appModel) renderTextAreaModal(title string) string {
        // Avoid borders here: some terminals show background artifacts when nesting bordered
        // components inside a modal with a background color.
        btnBase := lipgloss.NewStyle().
                Padding(0, 1).
                Foreground(lipgloss.Color("252")).
                Background(lipgloss.Color("235"))
        btnActive := btnBase.Copy().
                Foreground(lipgloss.Color("235")).
                Background(lipgloss.Color("62")).
                Bold(true)

        save := btnBase.Render("Save")
        cancel := btnBase.Render("Cancel")
        if m.textFocus == textFocusSave {
                save = btnActive.Render("Save")
        }
        if m.textFocus == textFocusCancel {
                cancel = btnActive.Render("Cancel")
        }

        sep := lipgloss.NewStyle().Background(lipgloss.Color("235")).Render(" ")
        controls := lipgloss.JoinHorizontal(lipgloss.Top, save, sep, cancel)
        body := strings.Join([]string{
                m.textarea.View(),
                "",
                controls,
                "",
                "ctrl+s: save    tab: focus    esc: cancel",
        }, "\n")
        return renderModalBox(m.width, title, body)
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
        case viewItem:
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
                if m.modal == modalActionPanel {
                        if km, ok := msg.(tea.KeyMsg); ok {
                                switch km.String() {
                                case "ctrl+g":
                                        (&m).closeActionPanel()
                                        return m, nil
                                case "esc", "backspace":
                                        (&m).popActionPanel()
                                        return m, nil
                                }

                                actions := m.actionPanelActions()
                                if a, ok := actions[km.String()]; ok {
                                        switch a.kind {
                                        case actionPanelActionNav:
                                                // Root -> subpanel: push (so esc/backspace returns to root).
                                                // Subpanel -> subpanel: switch (avoid infinite nesting).
                                                if len(m.actionPanelStack) <= 1 {
                                                        (&m).pushActionPanel(a.next)
                                                } else {
                                                        m.actionPanelStack[len(m.actionPanelStack)-1] = a.next
                                                }
                                                return m, nil
                                        default:
                                                // Execute and close (panel takes over keys; only listed keys run).
                                                (&m).closeActionPanel()
                                                if a.handler != nil {
                                                        m2, cmd := a.handler(m)
                                                        return m2, cmd
                                                }
                                                m2Any, cmd := m.Update(km)
                                                if m2, ok := m2Any.(appModel); ok {
                                                        return m2, cmd
                                                }
                                                return m, cmd
                                        }
                                }
                        }
                        return m, nil
                }

                if m.modal == modalConfirmArchive {
                        if km, ok := msg.(tea.KeyMsg); ok {
                                switch km.String() {
                                case "esc", "n":
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        return m, nil
                                case "enter", "y":
                                        target := strings.TrimSpace(m.modalForID)
                                        m.modal = modalNone
                                        m.modalForID = ""

                                        switch m.archiveFor {
                                        case archiveTargetProject:
                                                prevIdx := m.projectsList.Index()
                                                outlinesArchived, itemsArchived, err := m.archiveProjectTree(target)
                                                m.refreshProjects()
                                                if n := len(m.projectsList.Items()); n > 0 {
                                                        idx := prevIdx
                                                        if idx < 0 {
                                                                idx = 0
                                                        }
                                                        if idx >= n {
                                                                idx = n - 1
                                                        }
                                                        m.projectsList.Select(idx)
                                                }
                                                if err != nil {
                                                        m.showMinibuffer("Archive failed: " + err.Error())
                                                } else {
                                                        msg := "Archived project"
                                                        if outlinesArchived > 0 || itemsArchived > 0 {
                                                                msg = fmt.Sprintf("Archived project (%d outline(s), %d item(s))", outlinesArchived, itemsArchived)
                                                        }
                                                        m.showMinibuffer(msg)
                                                }
                                                return m, nil

                                        case archiveTargetOutline:
                                                prevIdx := m.outlinesList.Index()
                                                itemsArchived, err := m.archiveOutlineTree(target)
                                                // If we just archived the outline we were looking at, clear selection state.
                                                if m.selectedOutlineID == target {
                                                        m.selectedOutlineID = ""
                                                        m.selectedOutline = nil
                                                }
                                                m.refreshOutlines(m.selectedProjectID)
                                                if n := len(m.outlinesList.Items()); n > 0 {
                                                        idx := prevIdx
                                                        if idx < 0 {
                                                                idx = 0
                                                        }
                                                        if idx >= n {
                                                                idx = n - 1
                                                        }
                                                        m.outlinesList.Select(idx)
                                                }
                                                if err != nil {
                                                        m.showMinibuffer("Archive failed: " + err.Error())
                                                } else {
                                                        msg := "Archived outline"
                                                        if itemsArchived > 0 {
                                                                msg = fmt.Sprintf("Archived outline (%d item(s))", itemsArchived)
                                                        }
                                                        m.showMinibuffer(msg)
                                                }
                                                return m, nil

                                        default:
                                                // archiveTargetItem
                                                prevIdx := m.itemsList.Index()
                                                nextID := m.nearestSelectableItemID(prevIdx)
                                                archived, err := m.archiveItemTree(target)
                                                if m.selectedOutline != nil {
                                                        m.refreshItems(*m.selectedOutline)
                                                        selectListItemByID(&m.itemsList, nextID)
                                                }
                                                if err != nil {
                                                        m.showMinibuffer("Archive failed: " + err.Error())
                                                } else if archived > 0 {
                                                        m.showMinibuffer(fmt.Sprintf("Archived %d item(s)", archived))
                                                }
                                                // If we archived from the full-screen item view, return to the outline.
                                                if m.view == viewItem {
                                                        m.view = viewOutline
                                                        m.openItemID = ""
                                                        m.showPreview = false
                                                        m.pane = paneOutline
                                                }
                                                return m, nil
                                        }
                                }
                        }
                        return m, nil
                }

                if m.modal == modalAddComment || m.modal == modalAddWorklog {
                        switch km := msg.(type) {
                        case tea.KeyMsg:
                                switch km.String() {
                                case "esc":
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        m.textarea.Blur()
                                        m.textFocus = textFocusBody
                                        return m, nil
                                case "tab":
                                        switch m.textFocus {
                                        case textFocusBody:
                                                m.textFocus = textFocusSave
                                        case textFocusSave:
                                                m.textFocus = textFocusCancel
                                        default:
                                                m.textFocus = textFocusBody
                                        }
                                        if m.textFocus == textFocusBody {
                                                m.textarea.Focus()
                                        } else {
                                                m.textarea.Blur()
                                        }
                                        return m, nil
                                case "shift+tab", "backtab":
                                        switch m.textFocus {
                                        case textFocusBody:
                                                m.textFocus = textFocusCancel
                                        case textFocusCancel:
                                                m.textFocus = textFocusSave
                                        default:
                                                m.textFocus = textFocusBody
                                        }
                                        if m.textFocus == textFocusBody {
                                                m.textarea.Focus()
                                        } else {
                                                m.textarea.Blur()
                                        }
                                        return m, nil
                                case "enter":
                                        if m.textFocus == textFocusSave {
                                                body := strings.TrimSpace(m.textarea.Value())
                                                if body == "" {
                                                        return m, nil
                                                }
                                                itemID := strings.TrimSpace(m.modalForID)
                                                if m.modal == modalAddComment {
                                                        _ = m.addComment(itemID, body)
                                                } else {
                                                        _ = m.addWorklog(itemID, body)
                                                }
                                                m.modal = modalNone
                                                m.modalForID = ""
                                                m.textarea.SetValue("")
                                                m.textarea.Blur()
                                                m.textFocus = textFocusBody
                                                return m, nil
                                        }
                                        if m.textFocus == textFocusCancel {
                                                m.modal = modalNone
                                                m.modalForID = ""
                                                m.textarea.Blur()
                                                m.textFocus = textFocusBody
                                                return m, nil
                                        }
                                        // else: newline in textarea
                                case "ctrl+s":
                                        body := strings.TrimSpace(m.textarea.Value())
                                        if body == "" {
                                                return m, nil
                                        }
                                        itemID := strings.TrimSpace(m.modalForID)
                                        if m.modal == modalAddComment {
                                                _ = m.addComment(itemID, body)
                                        } else {
                                                _ = m.addWorklog(itemID, body)
                                        }
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        m.textarea.SetValue("")
                                        m.textarea.Blur()
                                        m.textFocus = textFocusBody
                                        return m, nil
                                }
                        }
                        var cmd tea.Cmd
                        if m.textFocus == textFocusBody {
                                m.textarea, cmd = m.textarea.Update(msg)
                                return m, cmd
                        }
                        // Ignore all other keys when focus is on the buttons.
                        return m, nil
                }

                if m.modal == modalPickStatus {
                        switch km := msg.(type) {
                        case tea.KeyMsg:
                                switch km.String() {
                                case "esc":
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        return m, nil
                                case "enter":
                                        if it, ok := m.statusList.SelectedItem().(statusOptionItem); ok {
                                                itemID := strings.TrimSpace(m.modalForID)
                                                if err := m.setStatusForItem(itemID, it.id); err != nil {
                                                        return m, m.reportError(itemID, err)
                                                }
                                        }
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        return m, nil
                                }
                        }
                        var cmd tea.Cmd
                        m.statusList, cmd = m.statusList.Update(msg)
                        return m, cmd
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
                                switch m.modal {
                                case modalNewProject:
                                        if val == "" {
                                                return m, nil
                                        }
                                        _ = m.createProjectFromModal(val)
                                case modalRenameProject:
                                        if val == "" {
                                                return m, nil
                                        }
                                        _ = m.renameProjectFromModal(val)
                                case modalNewOutline:
                                        // Name optional.
                                        _ = m.createOutlineFromModal(val)
                                case modalEditTitle:
                                        if val == "" {
                                                return m, nil
                                        }
                                        itemID := strings.TrimSpace(m.modalForID)
                                        if err := m.setTitleFromModal(val); err != nil {
                                                return m, m.reportError(itemID, err)
                                        }
                                case modalEditOutlineName:
                                        _ = m.setOutlineNameFromModal(val)
                                default:
                                        if val == "" {
                                                return m, nil
                                        }
                                        _ = m.createItemFromModal(val)
                                }
                                m.modal = modalNone
                                m.modalForID = ""
                                m.input.Placeholder = "Title"
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
                                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                        if err := m.moveSelected("up"); err != nil {
                                                return m, m.reportError(it.row.item.ID, err)
                                        }
                                        return m, nil
                                }
                        case "down", "j", "n":
                                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                        if err := m.moveSelected("down"); err != nil {
                                                return m, m.reportError(it.row.item.ID, err)
                                        }
                                        return m, nil
                                }
                        case "right", "l", "f":
                                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                        if err := m.indentSelected(); err != nil {
                                                return m, m.reportError(it.row.item.ID, err)
                                        }
                                        return m, nil
                                }
                        case "left", "h", "b":
                                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                        if err := m.outdentSelected(); err != nil {
                                                return m, m.reportError(it.row.item.ID, err)
                                        }
                                        return m, nil
                                }
                        }
                        // Otherwise: fall through and handle the key normally.
                }

                // Focus handling (split view only).
                if msg.String() == "tab" && m.splitPreviewVisible() {
                        if m.pane == paneOutline {
                                m.pane = paneDetail
                        } else {
                                m.pane = paneOutline
                        }
                        // Focus can affect styling; refresh the cached detail (debounced).
                        m.previewCacheForID = ""
                        return m, m.schedulePreviewCompute()
                }

                if msg.String() == "D" && m.debugEnabled {
                        m.debugOverlay = !m.debugOverlay
                        if m.debugOverlay {
                                m.showMinibuffer("Debug overlay: ON")
                        } else {
                                m.showMinibuffer("Debug overlay: OFF")
                        }
                        return m, nil
                }

                // Open item / create items.
                switch msg.String() {
                case "y":
                        // Copy selected item ID.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                if err := copyToClipboard(it.row.item.ID); err != nil {
                                        m.showMinibuffer("Clipboard error: " + err.Error())
                                } else {
                                        m.showMinibuffer("Copied item ID " + it.row.item.ID)
                                }
                                return m, nil
                        }
                case "Y":
                        // Copy a helpful CLI command for the selected item.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                cmd := "clarity items show " + it.row.item.ID
                                if err := copyToClipboard(cmd); err != nil {
                                        m.showMinibuffer("Clipboard error: " + err.Error())
                                } else {
                                        m.showMinibuffer("Copied: " + cmd)
                                }
                                return m, nil
                        }
                case "C":
                        // Add comment to selected item.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                m.openTextModal(modalAddComment, it.row.item.ID, "Write a comment…")
                                return m, nil
                        }
                case "w":
                        // Add worklog entry to selected item.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                m.openTextModal(modalAddWorklog, it.row.item.ID, "Log work…")
                                return m, nil
                        }
                case " ":
                        // Set status via picker (outline pane only).
                        if m.pane == paneOutline {
                                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                        m.openStatusPicker(it.outline, it.row.item.ID, it.row.item.StatusID)
                                        m.modal = modalPickStatus
                                        m.modalForID = it.row.item.ID
                                        return m, nil
                                }
                        }
                case "shift+right":
                        if m.pane == paneOutline {
                                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                        if err := m.cycleItemStatus(it.outline, it.row.item.ID, +1); err != nil {
                                                return m, m.reportError(it.row.item.ID, err)
                                        }
                                        return m, nil
                                }
                        }
                case "shift+left":
                        if m.pane == paneOutline {
                                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                        if err := m.cycleItemStatus(it.outline, it.row.item.ID, -1); err != nil {
                                                return m, m.reportError(it.row.item.ID, err)
                                        }
                                        return m, nil
                                }
                        }
                case "enter":
                        switch m.itemsList.SelectedItem().(type) {
                        case outlineRowItem:
                                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                        m.view = viewItem
                                        m.openItemID = it.row.item.ID
                                        // Leaving preview mode when entering the full item page.
                                        m.showPreview = false
                                        m.previewCacheForID = ""
                                        return m, nil
                                }
                                return m, nil
                        case addItemRow:
                                m.modal = modalNewSibling
                                m.modalForID = ""
                                m.input.SetValue("")
                                m.input.Focus()
                                return m, nil
                        }
                case "o":
                        // Toggle split preview pane.
                        m.showPreview = !m.showPreview
                        m.pane = paneOutline
                        m.previewCacheForID = ""
                        if m.showPreview {
                                return m, m.schedulePreviewCompute()
                        }
                        return m, nil
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
                                m.archiveFor = archiveTargetItem
                                m.input.Blur()
                                return m, nil
                        }
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
                        return m, m.schedulePreviewCompute()
                }

                // Outline structural operations (left pane only).
                if m.pane == paneOutline {
                        if handled, cmd := m.mutateOutlineByKey(msg); handled {
                                return m, cmd
                        }
                }
        }

        // Allow list to handle incidental keys (help paging, etc).
        beforeID := ""
        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                beforeID = strings.TrimSpace(it.row.item.ID)
        }
        var cmd tea.Cmd
        m.itemsList, cmd = m.itemsList.Update(msg)
        if m.splitPreviewVisible() {
                afterID := ""
                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                        afterID = strings.TrimSpace(it.row.item.ID)
                }
                if beforeID != afterID {
                        return m, tea.Batch(cmd, m.schedulePreviewCompute())
                }
        }
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
        actorID := m.editActorID()
        if actorID == "" {
                return errors.New("no current actor")
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
        actorID := m.editActorID()
        if actorID == "" {
                return 0, errors.New("no current actor")
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

func (m *appModel) archiveOutlineTree(outlineID string) (int, error) {
        actorID := m.editActorID()
        if actorID == "" {
                return 0, errors.New("no current actor")
        }
        outlineID = strings.TrimSpace(outlineID)
        if outlineID == "" {
                return 0, nil
        }

        db, err := m.store.Load()
        if err != nil {
                return 0, err
        }
        m.db = db

        o, ok := m.db.FindOutline(outlineID)
        if !ok {
                return 0, nil
        }
        if o.Archived {
                return 0, nil
        }

        // Archive all items in this outline (best-effort respecting ownership).
        now := time.Now().UTC()
        itemsArchived := 0
        for i := range m.db.Items {
                it := &m.db.Items[i]
                if it.OutlineID != outlineID {
                        continue
                }
                if it.Archived {
                        continue
                }
                if !canEditItem(m.db, actorID, it) {
                        continue
                }
                it.Archived = true
                it.UpdatedAt = now
                _ = m.store.AppendEvent(actorID, "item.archive", it.ID, map[string]any{"archived": true})
                itemsArchived++
        }

        o.Archived = true
        _ = m.store.AppendEvent(actorID, "outline.archive", o.ID, map[string]any{"archived": true})

        if err := m.store.Save(m.db); err != nil {
                return itemsArchived, err
        }
        m.captureStoreModTimes()
        return itemsArchived, nil
}

func (m *appModel) archiveProjectTree(projectID string) (int, int, error) {
        actorID := m.editActorID()
        if actorID == "" {
                return 0, 0, errors.New("no current actor")
        }
        projectID = strings.TrimSpace(projectID)
        if projectID == "" {
                return 0, 0, nil
        }

        db, err := m.store.Load()
        if err != nil {
                return 0, 0, err
        }
        m.db = db

        p, ok := m.db.FindProject(projectID)
        if !ok {
                return 0, 0, nil
        }
        if p.Archived {
                return 0, 0, nil
        }

        // Archive all outlines + items in this project.
        outlinesArchived := 0
        for i := range m.db.Outlines {
                o := &m.db.Outlines[i]
                if o.ProjectID != projectID {
                        continue
                }
                if o.Archived {
                        continue
                }
                o.Archived = true
                _ = m.store.AppendEvent(actorID, "outline.archive", o.ID, map[string]any{"archived": true})
                outlinesArchived++
        }

        now := time.Now().UTC()
        itemsArchived := 0
        for i := range m.db.Items {
                it := &m.db.Items[i]
                if it.ProjectID != projectID {
                        continue
                }
                if it.Archived {
                        continue
                }
                if !canEditItem(m.db, actorID, it) {
                        continue
                }
                it.Archived = true
                it.UpdatedAt = now
                _ = m.store.AppendEvent(actorID, "item.archive", it.ID, map[string]any{"archived": true})
                itemsArchived++
        }

        p.Archived = true
        _ = m.store.AppendEvent(actorID, "project.archive", p.ID, map[string]any{"archived": true})

        // Clear current project if we just archived it.
        if m.db.CurrentProjectID == projectID {
                m.db.CurrentProjectID = ""
        }
        if m.selectedProjectID == projectID {
                m.selectedProjectID = ""
        }

        if err := m.store.Save(m.db); err != nil {
                return outlinesArchived, itemsArchived, err
        }
        m.captureStoreModTimes()
        return outlinesArchived, itemsArchived, nil
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

func countUnarchivedItemsInOutline(db *store.DB, outlineID string) int {
        if db == nil || strings.TrimSpace(outlineID) == "" {
                return 0
        }
        n := 0
        for _, it := range db.Items {
                if it.OutlineID != outlineID {
                        continue
                }
                if it.Archived {
                        continue
                }
                n++
        }
        return n
}

func countUnarchivedOutlinesInProject(db *store.DB, projectID string) int {
        if db == nil || strings.TrimSpace(projectID) == "" {
                return 0
        }
        n := 0
        for _, o := range db.Outlines {
                if o.ProjectID != projectID {
                        continue
                }
                if o.Archived {
                        continue
                }
                n++
        }
        return n
}

func countUnarchivedItemsInProject(db *store.DB, projectID string) int {
        if db == nil || strings.TrimSpace(projectID) == "" {
                return 0
        }
        n := 0
        for _, it := range db.Items {
                if it.ProjectID != projectID {
                        continue
                }
                if it.Archived {
                        continue
                }
                n++
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

func (m *appModel) mutateOutlineByKey(msg tea.KeyMsg) (bool, tea.Cmd) {
        // Move item down/up.
        if isAltDown(msg) {
                itemID := ""
                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                        itemID = it.row.item.ID
                }
                if err := m.moveSelected("down"); err != nil {
                        return true, m.reportError(itemID, err)
                }
                return true, nil
        }
        if isAltUp(msg) {
                itemID := ""
                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                        itemID = it.row.item.ID
                }
                if err := m.moveSelected("up"); err != nil {
                        return true, m.reportError(itemID, err)
                }
                return true, nil
        }
        // Indent/outdent.
        if isAltRight(msg) {
                itemID := ""
                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                        itemID = it.row.item.ID
                }
                if err := m.indentSelected(); err != nil {
                        return true, m.reportError(itemID, err)
                }
                return true, nil
        }
        if isAltLeft(msg) {
                itemID := ""
                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                        itemID = it.row.item.ID
                }
                if err := m.outdentSelected(); err != nil {
                        return true, m.reportError(itemID, err)
                }
                return true, nil
        }
        return false, nil
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

        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db
        actorID := m.editActorID()
        if actorID == "" {
                return errors.New("no current actor")
        }

        t, ok := m.db.FindItem(itemID)
        if !ok {
                return nil
        }
        if !canEditItem(m.db, actorID, t) {
                return errors.New("permission denied")
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

func (m *appModel) setOutlineNameFromModal(name string) error {
        outlineID := strings.TrimSpace(m.modalForID)
        if outlineID == "" {
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

        o, ok := m.db.FindOutline(outlineID)
        if !ok {
                return nil
        }

        trim := strings.TrimSpace(name)
        if trim == "" {
                o.Name = nil
        } else {
                tmp := trim
                o.Name = &tmp
        }

        if err := m.store.Save(m.db); err != nil {
                return err
        }
        _ = m.store.AppendEvent(actorID, "outline.rename", o.ID, map[string]any{"name": o.Name})
        m.captureStoreModTimes()

        m.refreshOutlines(m.selectedProjectID)
        selectListItemByID(&m.outlinesList, o.ID)
        m.showMinibuffer("Renamed outline " + o.ID)
        return nil
}

func (m *appModel) createProjectFromModal(name string) error {
        name = strings.TrimSpace(name)
        if name == "" {
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

        p := model.Project{
                ID:        m.store.NextID(m.db, "proj"),
                Name:      name,
                CreatedBy: actorID,
                CreatedAt: time.Now().UTC(),
        }
        m.db.Projects = append(m.db.Projects, p)
        m.db.CurrentProjectID = p.ID
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        _ = m.store.AppendEvent(actorID, "project.create", p.ID, p)
        m.captureStoreModTimes()

        m.selectedProjectID = p.ID
        m.refreshProjects()
        selectListItemByID(&m.projectsList, p.ID)

        // Take the user into outlines for the new project (same as selecting it).
        m.view = viewOutlines
        m.refreshOutlines(p.ID)
        m.showMinibuffer("Created project " + p.ID)
        return nil
}

func (m *appModel) renameProjectFromModal(name string) error {
        projectID := strings.TrimSpace(m.modalForID)
        name = strings.TrimSpace(name)
        if projectID == "" || name == "" {
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

        p, ok := m.db.FindProject(projectID)
        if !ok {
                return nil
        }
        p.Name = name
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        _ = m.store.AppendEvent(actorID, "project.rename", p.ID, map[string]any{"name": p.Name})
        m.captureStoreModTimes()

        m.refreshProjects()
        selectListItemByID(&m.projectsList, p.ID)
        m.showMinibuffer("Renamed project " + p.ID)
        return nil
}

func (m *appModel) createOutlineFromModal(name string) error {
        actorID := strings.TrimSpace(m.db.CurrentActorID)
        if actorID == "" {
                return nil
        }
        projectID := strings.TrimSpace(m.selectedProjectID)
        if projectID == "" {
                projectID = strings.TrimSpace(m.db.CurrentProjectID)
        }
        if projectID == "" {
                return nil
        }

        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db

        if _, ok := m.db.FindProject(projectID); !ok {
                return nil
        }

        var namePtr *string
        trim := strings.TrimSpace(name)
        if trim != "" {
                tmp := trim
                namePtr = &tmp
        }
        o := model.Outline{
                ID:         m.store.NextID(m.db, "out"),
                ProjectID:  projectID,
                Name:       namePtr,
                StatusDefs: store.DefaultOutlineStatusDefs(),
                CreatedBy:  actorID,
                CreatedAt:  time.Now().UTC(),
        }
        m.db.Outlines = append(m.db.Outlines, o)
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        _ = m.store.AppendEvent(actorID, "outline.create", o.ID, o)
        m.captureStoreModTimes()

        m.refreshOutlines(projectID)
        selectListItemByID(&m.outlinesList, o.ID)
        m.showMinibuffer("Created outline " + o.ID)
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

func (m *appModel) openStatusPicker(outline model.Outline, itemID, currentStatusID string) {
        opts := []list.Item{statusOptionItem{id: "", label: "(no status)"}}
        for _, def := range outline.StatusDefs {
                opts = append(opts, statusOptionItem{id: def.ID, label: def.Label})
        }

        m.statusList.Title = ""
        m.statusList.SetItems(opts)

        // Size the picker to something reasonable.
        modalW := m.width - 12
        if modalW > m.width-4 {
                modalW = m.width - 4
        }
        if modalW < 20 {
                modalW = 20
        }
        if modalW > 96 {
                modalW = 96
        }
        h := len(opts) + 2
        if h > 14 {
                h = 14
        }
        if h < 6 {
                h = 6
        }
        m.statusList.SetSize(modalW-6, h)

        // Preselect current.
        selected := 0
        for i := 0; i < len(opts); i++ {
                if s, ok := opts[i].(statusOptionItem); ok && s.id == currentStatusID {
                        selected = i
                        break
                }
        }
        m.statusList.Select(selected)
}

func (m *appModel) cycleItemStatus(outline model.Outline, itemID string, delta int) error {
        itemID = strings.TrimSpace(itemID)
        if itemID == "" || delta == 0 {
                return nil
        }
        cur, ok := m.db.FindItem(itemID)
        if !ok {
                return nil
        }
        opts := []string{""}
        for _, def := range outline.StatusDefs {
                opts = append(opts, def.ID)
        }
        if len(opts) == 0 {
                return nil
        }

        curIdx := 0
        for i, sid := range opts {
                if sid == cur.StatusID {
                        curIdx = i
                        break
                }
        }
        next := (curIdx + delta) % len(opts)
        if next < 0 {
                next += len(opts)
        }
        return m.setStatusForItem(itemID, opts[next])
}

func (m *appModel) setStatusForItem(itemID, statusID string) error {
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return nil
        }

        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db
        actorID := m.editActorID()
        if actorID == "" {
                return errors.New("no current actor")
        }

        t, ok := m.db.FindItem(itemID)
        if !ok {
                return nil
        }
        if !canEditItem(m.db, actorID, t) {
                return errors.New("permission denied")
        }

        // Validate against outline status defs (empty is always allowed).
        if strings.TrimSpace(statusID) != "" {
                o, ok := m.db.FindOutline(t.OutlineID)
                if ok {
                        valid := false
                        for _, def := range o.StatusDefs {
                                if def.ID == statusID {
                                        valid = true
                                        break
                                }
                        }
                        if !valid {
                                return errors.New("invalid status")
                        }
                }
        }

        t.StatusID = strings.TrimSpace(statusID)
        t.UpdatedAt = time.Now().UTC()
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        _ = m.store.AppendEvent(actorID, "item.set_status", t.ID, map[string]any{"status": t.StatusID})
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

func (m *appModel) moveSelected(dir string) error {
        it, ok := m.itemsList.SelectedItem().(outlineRowItem)
        if !ok {
                return nil
        }

        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db
        actorID := m.editActorID()
        if actorID == "" {
                return errors.New("no current actor")
        }
        t, ok := m.db.FindItem(it.row.item.ID)
        if !ok {
                return nil
        }
        if !canEditItem(m.db, actorID, t) {
                return errors.New("permission denied")
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

        lower, upper, ok := rankBoundsForInsert(sibs, afterID, beforeID)
        if !ok {
                return nil
        }

        existing := map[string]bool{}
        for _, s := range sibs {
                rn := strings.ToLower(strings.TrimSpace(s.Rank))
                if rn != "" {
                        existing[rn] = true
                }
        }
        r, err := store.RankBetweenUnique(existing, lower, upper)
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
        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db
        actorID := m.editActorID()
        if actorID == "" {
                return errors.New("no current actor")
        }
        t, ok := m.db.FindItem(it.row.item.ID)
        if !ok {
                return nil
        }
        if !canEditItem(m.db, actorID, t) {
                return errors.New("permission denied")
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
        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db
        actorID := m.editActorID()
        if actorID == "" {
                return errors.New("no current actor")
        }
        t, ok := m.db.FindItem(it.row.item.ID)
        if !ok {
                return nil
        }
        if !canEditItem(m.db, actorID, t) {
                return errors.New("permission denied")
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
                upper := findNextRankGreaterThan(sibs, refIdx, lower)
                existing := map[string]bool{}
                for _, s := range sibs {
                        rn := strings.ToLower(strings.TrimSpace(s.Rank))
                        if rn != "" {
                                existing[rn] = true
                        }
                }
                r, err := store.RankBetweenUnique(existing, lower, upper)
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
        return perm.CanEditItem(db, actorID, t)
}

// editActorID returns the human actor id to attribute mutations to.
//
// The interactive TUI is primarily for humans. If the current actor is an agent (often due to
// a previous `clarity agent start ...`), we still want user-driven edits (status, move, etc.)
// to work against items owned by the human and be attributed to that human actor.
func (m *appModel) editActorID() string {
        if m == nil || m.db == nil {
                return ""
        }
        cur := strings.TrimSpace(m.db.CurrentActorID)
        if cur == "" {
                return ""
        }
        if human, ok := m.db.HumanUserIDForActor(cur); ok && strings.TrimSpace(human) != "" {
                return strings.TrimSpace(human)
        }
        return cur
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

func (m *appModel) openTextModal(kind modalKind, itemID, placeholder string) {
        if strings.TrimSpace(itemID) == "" {
                return
        }
        m.modal = kind
        m.modalForID = itemID
        m.textFocus = textFocusBody

        w := m.width - 12
        if w > m.width-4 {
                w = m.width - 4
        }
        if w < 20 {
                w = 20
        }
        if w > 96 {
                w = 96
        }
        bodyW := w - 6 // renderModalBox has padding 1,2
        if bodyW < 10 {
                bodyW = 10
        }

        h := m.height - 12
        if h < 6 {
                h = 6
        }
        if h > 16 {
                h = 16
        }

        m.textarea.Placeholder = placeholder
        m.textarea.SetWidth(bodyW)
        m.textarea.SetHeight(h)
        m.textarea.SetValue("")
        m.textarea.Focus()
}

func (m *appModel) addComment(itemID, body string) error {
        itemID = strings.TrimSpace(itemID)
        body = strings.TrimSpace(body)
        if itemID == "" || body == "" {
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

        if _, ok := m.db.FindItem(itemID); !ok {
                return nil
        }

        c := model.Comment{
                ID:        m.store.NextID(m.db, "cmt"),
                ItemID:    itemID,
                AuthorID:  actorID,
                Body:      body,
                CreatedAt: time.Now().UTC(),
        }
        m.db.Comments = append(m.db.Comments, c)
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        _ = m.store.AppendEvent(actorID, "comment.add", c.ID, c)
        m.captureStoreModTimes()

        if m.selectedOutline != nil {
                if o, ok := m.db.FindOutline(m.selectedOutline.ID); ok {
                        m.selectedOutline = o
                }
                m.refreshItems(*m.selectedOutline)
                selectListItemByID(&m.itemsList, itemID)
        }
        m.showMinibuffer("Comment added")
        return nil
}

func (m *appModel) addWorklog(itemID, body string) error {
        itemID = strings.TrimSpace(itemID)
        body = strings.TrimSpace(body)
        if itemID == "" || body == "" {
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

        if _, ok := m.db.FindItem(itemID); !ok {
                return nil
        }

        w := model.WorklogEntry{
                ID:        m.store.NextID(m.db, "wlg"),
                ItemID:    itemID,
                AuthorID:  actorID,
                Body:      body,
                CreatedAt: time.Now().UTC(),
        }
        m.db.Worklog = append(m.db.Worklog, w)
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        _ = m.store.AppendEvent(actorID, "worklog.add", w.ID, w)
        m.captureStoreModTimes()

        if m.selectedOutline != nil {
                if o, ok := m.db.FindOutline(m.selectedOutline.ID); ok {
                        m.selectedOutline = o
                }
                m.refreshItems(*m.selectedOutline)
                selectListItemByID(&m.itemsList, itemID)
        }
        m.showMinibuffer("Worklog added")
        return nil
}
