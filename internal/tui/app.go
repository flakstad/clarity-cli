package tui

import (
        "errors"
        "fmt"
        "os"
        "path/filepath"
        "sort"
        "strconv"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/mutate"
        "clarity-cli/internal/perm"
        "clarity-cli/internal/statusutil"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/bubbles/list"
        "github.com/charmbracelet/bubbles/textinput"
        tea "github.com/charmbracelet/bubbletea"
        "github.com/charmbracelet/lipgloss"
        xansi "github.com/charmbracelet/x/ansi"
)

type itemNavEntry struct {
        // fromID is the item we navigated *from* (the "back" target).
        fromID string
        // toID is the item we navigated *to* (used to restore focus/selection when returning).
        toID string
}

func (m *appModel) currentWriteActorID() string {
        if m == nil || m.db == nil {
                return ""
        }
        cur := strings.TrimSpace(m.db.CurrentActorID)
        if cur == "" {
                return ""
        }
        // In the TUI, we treat manual actions as coming from the owning human actor even if
        // CurrentActorID is currently an agent actor.
        if humanID, ok := m.db.HumanUserIDForActor(cur); ok {
                if strings.TrimSpace(humanID) != "" {
                        return strings.TrimSpace(humanID)
                }
        }
        return cur
}

const maxRecentItems = 5

func (m *appModel) recordRecentItemVisit(itemID string) {
        if m == nil {
                return
        }
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return
        }
        // Best-effort validation: skip missing/archived items.
        if m.db != nil {
                if it, ok := m.db.FindItem(itemID); !ok || it == nil || it.Archived {
                        return
                }
        }

        // De-dupe (preserve existing relative order) and cap.
        next := make([]string, 0, maxRecentItems)
        next = append(next, itemID)
        for _, id := range m.recentItemIDs {
                id = strings.TrimSpace(id)
                if id == "" || id == itemID {
                        continue
                }
                next = append(next, id)
                if len(next) >= maxRecentItems {
                        break
                }
        }
        m.recentItemIDs = next
}

type outlineViewMode int

const (
        outlineViewModeList outlineViewMode = iota
        outlineViewModeListPreview
        outlineViewModeColumns
)

func outlineViewModeLabel(v outlineViewMode) string {
        switch v {
        case outlineViewModeListPreview:
                return "list+preview"
        case outlineViewModeColumns:
                return "columns"
        default:
                return "list"
        }
}

func (m *appModel) curOutlineViewMode() outlineViewMode {
        if m == nil {
                return outlineViewModeList
        }
        id := strings.TrimSpace(m.selectedOutlineID)
        return m.outlineViewModeForID(id)
}

func (m *appModel) outlineViewModeForID(id string) outlineViewMode {
        if m == nil {
                return outlineViewModeList
        }
        id = strings.TrimSpace(id)
        if id == "" || m.outlineViewMode == nil {
                return outlineViewModeList
        }
        if v, ok := m.outlineViewMode[id]; ok {
                return v
        }
        return outlineViewModeList
}

func (m *appModel) setOutlineViewMode(id string, mode outlineViewMode) {
        if m == nil {
                return
        }
        id = strings.TrimSpace(id)
        if id == "" {
                return
        }
        if m.outlineViewMode == nil {
                m.outlineViewMode = map[string]outlineViewMode{}
        }

        m.outlineViewMode[id] = mode

        // Apply side effects.
        switch mode {
        case outlineViewModeListPreview:
                m.showPreview = true
                m.pane = paneOutline
        case outlineViewModeColumns:
                // Kanban uses the whole canvas: disable preview.
                m.showPreview = false
                m.pane = paneOutline
                // Clear any active outline filter (columns view doesn't render the list/filter UI).
                if m.itemsList.SettingFilter() || m.itemsList.IsFiltered() {
                        m.itemsList.ResetFilter()
                }
        default:
                // list
                m.showPreview = false
                m.pane = paneOutline
        }
        m.previewCacheForID = ""
}

func (m *appModel) cycleOutlineViewMode() {
        if m == nil {
                return
        }
        id := strings.TrimSpace(m.selectedOutlineID)
        if id == "" {
                return
        }
        cur := m.outlineViewModeForID(id)
        next := outlineViewModeList
        switch cur {
        case outlineViewModeList:
                next = outlineViewModeListPreview
        case outlineViewModeListPreview:
                next = outlineViewModeColumns
        default:
                next = outlineViewModeList
        }
        m.setOutlineViewMode(id, next)
        m.showMinibuffer("View: " + outlineViewModeLabel(next))
}

func (m *appModel) openActionPanel(kind actionPanelKind) {
        if m == nil {
                return
        }
        m.modal = modalActionPanel
        m.actionPanelStack = []actionPanelKind{kind}
        m.actionPanelSelectedKey = ""
        if kind == actionPanelCapture {
                m.captureKeySeq = nil
        }
        m.ensureActionPanelSelection()
        m.pendingEsc = false
}

func (m *appModel) closeActionPanel() {
        if m == nil {
                return
        }
        if m.modal == modalActionPanel {
                m.modal = modalNone
                m.actionPanelStack = nil
                m.actionPanelSelectedKey = ""
                m.pendingEsc = false
        }
}

func (m *appModel) openCaptureModal() (appModel, tea.Cmd) {
        if m == nil {
                return appModel{}, nil
        }

        cfg, err := store.LoadConfig()
        if err != nil {
                m.showMinibuffer("Capture: " + err.Error())
                return *m, nil
        }
        if err := store.ValidateCaptureTemplates(cfg); err != nil {
                m.showMinibuffer("Capture: " + err.Error())
                return *m, nil
        }

        actorOverride := strings.TrimSpace(m.currentWriteActorID())
        cm, err := newEmbeddedCaptureModel(cfg, actorOverride)
        if err != nil {
                m.showMinibuffer("Capture: " + err.Error())
                return *m, nil
        }
        // Ensure the embedded capture model has an initial size so it doesn't show "Loading…".
        if m.width > 0 && m.height > 0 {
                mmAny, _ := cm.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
                if mm, ok := mmAny.(captureModel); ok {
                        cm = mm
                }
        }

        m.modal = modalCapture
        m.capture = &cm
        m.pendingEsc = false
        return *m, nil
}

func (m *appModel) pushActionPanel(kind actionPanelKind) {
        if m == nil {
                return
        }
        if m.modal != modalActionPanel {
                return
        }
        m.actionPanelStack = append(m.actionPanelStack, kind)
        m.actionPanelSelectedKey = ""
        if kind == actionPanelCapture {
                m.captureKeySeq = nil
        }
        m.ensureActionPanelSelection()
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
        m.actionPanelSelectedKey = ""
        if m.curActionPanelKind() != actionPanelCapture {
                m.captureKeySeq = nil
        }
        m.ensureActionPanelSelection()
}

func (m *appModel) ensureActionPanelSelection() {
        if m == nil || m.modal != modalActionPanel {
                return
        }
        entries := m.actionPanelEntries()
        if len(entries) == 0 {
                m.actionPanelSelectedKey = ""
                return
        }
        cur := strings.TrimSpace(m.actionPanelSelectedKey)
        if cur != "" {
                for _, e := range entries {
                        if e.key == cur {
                                return
                        }
                }
        }
        m.actionPanelSelectedKey = entries[0].key
}

func (m *appModel) moveActionPanelSelection(delta int) {
        if m == nil || m.modal != modalActionPanel {
                return
        }
        entries := m.actionPanelEntries()
        if len(entries) == 0 {
                m.actionPanelSelectedKey = ""
                return
        }
        cur := strings.TrimSpace(m.actionPanelSelectedKey)
        idx := 0
        for i, e := range entries {
                if e.key == cur {
                        idx = i
                        break
                }
        }
        next := idx + delta
        for next < 0 {
                next += len(entries)
        }
        for next >= len(entries) {
                next -= len(entries)
        }
        m.actionPanelSelectedKey = entries[next].key
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
                return "Go to"
        case actionPanelAgenda:
                return "Agenda Commands"
        case actionPanelCapture:
                return "Capture"
        case actionPanelOutline:
                return "Outline…"
        default:
                return "Actions"
        }
}

func (m appModel) actionPanelActions() map[string]actionPanelAction {
        cur := m.curActionPanelKind()
        actions := map[string]actionPanelAction{}

        // Only the root action panel (opened with x) shows global entrypoints.
        if cur == actionPanelContext {
                actions["a"] = actionPanelAction{label: "Agenda Commands…", kind: actionPanelActionNav, next: actionPanelAgenda}
                actions["c"] = actionPanelAction{
                        label: "Capture…",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                return (&mm).openCaptureModal()
                        },
                }
                actions["ctrl+t"] = actionPanelAction{
                        label: "Capture templates…",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                (&mm).openCaptureTemplatesModal()
                                return mm, nil
                        },
                }
                actions["s"] = actionPanelAction{label: "Sync…", kind: actionPanelActionNav, next: actionPanelSync}
        }

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
                                mm.itemArchivedReadOnly = false
                                mm.pane = paneOutline
                                mm.refreshProjects()
                                return mm, nil
                        },
                }
                actions["A"] = actionPanelAction{
                        label: "Archived",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                mm.hasArchivedReturnView = true
                                mm.archivedReturnView = mm.view
                                mm.view = viewArchived
                                mm.showPreview = false
                                mm.openItemID = ""
                                mm.hasReturnView = false
                                mm.itemArchivedReadOnly = false
                                mm.pane = paneOutline
                                mm.refreshArchived()
                                return mm, nil
                        },
                }
                actions["W"] = actionPanelAction{
                        label: "Workspaces…",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                (&mm).openWorkspacePicker()
                                return mm, nil
                        },
                }
                actions["s"] = actionPanelAction{label: "Sync…", kind: actionPanelActionNav, next: actionPanelSync}
                actions["j"] = actionPanelAction{
                        label: "Jump to item by id…",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                mm.modalForKey = ""
                                (&mm).openInputModal(modalJumpToItem, "", "item-…", "")
                                return mm, nil
                        },
                }

                // Recent items (full item view visits), with digit shortcuts.
                // 1 = most recent, 5 = least recent (within the shown set).
                if m.db != nil {
                        for i := 0; i < len(m.recentItemIDs) && i < maxRecentItems; i++ {
                                id := strings.TrimSpace(m.recentItemIDs[i])
                                if id == "" {
                                        continue
                                }
                                it, ok := m.db.FindItem(id)
                                if !ok || it == nil || it.Archived {
                                        continue
                                }

                                key := strconv.Itoa(i + 1)
                                itemID := id

                                // Label is best-effort and mainly used for "entries" bookkeeping; rendering
                                // is handled by the special full-width Recent items section.
                                title := strings.TrimSpace(it.Title)
                                if title == "" {
                                        title = "(untitled)"
                                }
                                label := title

                                actions[key] = actionPanelAction{
                                        label: label,
                                        kind:  actionPanelActionExec,
                                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                                if err := (&mm).jumpToItemByID(itemID); err != nil {
                                                        mm.showMinibuffer("Jump: " + err.Error())
                                                        return mm, nil
                                                }
                                                return mm, nil
                                        },
                                }
                        }
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
                                        mm.itemArchivedReadOnly = false
                                        mm.itemFocus = itemFocusTitle
                                        mm.itemCommentIdx = 0
                                        mm.itemWorklogIdx = 0
                                        mm.itemHistoryIdx = 0
                                        mm.itemSideScroll = 0
                                        mm.showPreview = false
                                        mm.pane = paneOutline
                                        return mm, nil
                                },
                        }
                }

        case actionPanelAgenda:
                actions["t"] = actionPanelAction{label: "List all TODO entries", kind: actionPanelActionExec, handler: func(mm appModel) (appModel, tea.Cmd) {
                        if mm.view != viewAgenda {
                                mm.hasAgendaReturnView = true
                                mm.agendaReturnView = mm.view
                        }
                        mm.view = viewAgenda
                        mm.refreshAgenda()
                        return mm, nil
                }}

        case actionPanelCapture:
                actions["ctrl+t"] = actionPanelAction{
                        label: "Capture templates…",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                (&mm).openCaptureTemplatesModal()
                                return mm, nil
                        },
                }

        case actionPanelOutline:
                // Outline sub-menu (from outline screen or outlines list).
                actions["e"] = actionPanelAction{
                        label: "Rename outline",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                oid := strings.TrimSpace(mm.selectedOutlineID)
                                if mm.view == viewOutlines {
                                        if it, ok := mm.outlinesList.SelectedItem().(outlineItem); ok {
                                                oid = strings.TrimSpace(it.outline.ID)
                                        }
                                }
                                if oid == "" {
                                        mm.showMinibuffer("No outline selected")
                                        return mm, nil
                                }
                                name := ""
                                if mm.db != nil {
                                        if o, ok := mm.db.FindOutline(oid); ok && o != nil && o.Name != nil {
                                                name = strings.TrimSpace(*o.Name)
                                        }
                                }
                                mm.openInputModal(modalEditOutlineName, oid, "Outline name (optional)", name)
                                return mm, nil
                        },
                }
                actions["D"] = actionPanelAction{
                        label: "Edit outline description",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                oid := strings.TrimSpace(mm.selectedOutlineID)
                                desc := ""
                                if mm.view == viewOutlines {
                                        if it, ok := mm.outlinesList.SelectedItem().(outlineItem); ok {
                                                oid = strings.TrimSpace(it.outline.ID)
                                                desc = it.outline.Description
                                        }
                                } else if mm.db != nil {
                                        if o, ok := mm.db.FindOutline(oid); ok && o != nil {
                                                desc = o.Description
                                        }
                                }
                                if oid == "" {
                                        mm.showMinibuffer("No outline selected")
                                        return mm, nil
                                }
                                mm.openTextModal(modalEditOutlineDescription, oid, "Markdown outline description…", desc)
                                return mm, nil
                        },
                }

        case actionPanelSync:
                actions["s"] = actionPanelAction{
                        label: "Refresh status",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                return mm, (&mm).syncRefreshStatusNow()
                        },
                }
                if !m.gitStatus.IsRepo || strings.TrimSpace(m.gitStatus.Upstream) == "" {
                        actions["g"] = actionPanelAction{
                                label: "Setup Git…",
                                kind:  actionPanelActionExec,
                                handler: func(mm appModel) (appModel, tea.Cmd) {
                                        mm.openInputModal(modalGitSetupRemote, "", "Remote URL (optional)", "")
                                        return mm, nil
                                },
                        }
                }
                actions["p"] = actionPanelAction{
                        label: "Pull --rebase",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                return mm, (&mm).syncPullCmd()
                        },
                }
                actions["P"] = actionPanelAction{
                        label: "Commit + push",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                return mm, (&mm).syncPushCmd()
                        },
                }
                actions["r"] = actionPanelAction{
                        label: "Resolve conflicts (help)",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                (&mm).syncResolveNote()
                                return mm, nil
                        },
                }

        default:
                // Contextual (depends on current view/pane).
                actions["g"] = actionPanelAction{label: "Go to…", kind: actionPanelActionNav, next: actionPanelNav}
                actions["W"] = actionPanelAction{
                        label: "Workspaces…",
                        kind:  actionPanelActionExec,
                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                (&mm).openWorkspacePicker()
                                return mm, nil
                        },
                }
                actions["s"] = actionPanelAction{label: "Sync…", kind: actionPanelActionNav, next: actionPanelSync}

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
                        actions["D"] = actionPanelAction{label: "Edit outline description", kind: actionPanelActionExec}
                        actions["O"] = actionPanelAction{label: "Outline…", kind: actionPanelActionNav, next: actionPanelOutline}
                        actions["S"] = actionPanelAction{label: "Edit outline statuses…", kind: actionPanelActionExec}
                        actions["r"] = actionPanelAction{label: "Archive outline", kind: actionPanelActionExec}
                        actions["q"] = actionPanelAction{label: "Quit", kind: actionPanelActionExec}
                case viewItem:
                        readOnly := false
                        if m.db != nil {
                                id := strings.TrimSpace(m.openItemID)
                                if id != "" {
                                        if it, ok := m.db.FindItem(id); ok && it != nil && (it.Archived || m.itemArchivedReadOnly) {
                                                readOnly = true
                                        }
                                }
                        }
                        if readOnly {
                                actions["y"] = actionPanelAction{label: "Copy item ref (includes --workspace)", kind: actionPanelActionExec}
                                actions["Y"] = actionPanelAction{label: "Copy CLI show command (includes --workspace)", kind: actionPanelActionExec}
                                actions["q"] = actionPanelAction{label: "Quit", kind: actionPanelActionExec}
                        } else {
                                actions["e"] = actionPanelAction{label: "Edit title", kind: actionPanelActionExec}
                                actions["D"] = actionPanelAction{label: "Edit description", kind: actionPanelActionExec}
                                actions["p"] = actionPanelAction{label: "Toggle priority", kind: actionPanelActionExec}
                                actions["o"] = actionPanelAction{label: "Toggle on hold", kind: actionPanelActionExec}
                                actions["u"] = actionPanelAction{
                                        label: "Assign…",
                                        kind:  actionPanelActionExec,
                                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                                id := strings.TrimSpace(mm.openItemID)
                                                if id != "" {
                                                        (&mm).openAssigneePicker(id)
                                                }
                                                return mm, nil
                                        },
                                }
                                actions["t"] = actionPanelAction{
                                        label: "Tags…",
                                        kind:  actionPanelActionExec,
                                        handler: func(mm appModel) (appModel, tea.Cmd) {
                                                id := strings.TrimSpace(mm.openItemID)
                                                if id != "" {
                                                        (&mm).openTagsEditor(id)
                                                }
                                                return mm, nil
                                        },
                                }
                                actions["d"] = actionPanelAction{label: "Set due", kind: actionPanelActionExec}
                                actions["s"] = actionPanelAction{label: "Set schedule", kind: actionPanelActionExec}
                                actions[" "] = actionPanelAction{label: "Change status", kind: actionPanelActionExec}
                                actions["C"] = actionPanelAction{label: "Add comment", kind: actionPanelActionExec}
                                actions["w"] = actionPanelAction{label: "Add worklog", kind: actionPanelActionExec}
                                actions["y"] = actionPanelAction{label: "Copy item ref (includes --workspace)", kind: actionPanelActionExec}
                                actions["Y"] = actionPanelAction{label: "Copy CLI show command (includes --workspace)", kind: actionPanelActionExec}
                                actions["m"] = actionPanelAction{label: "Move to outline…", kind: actionPanelActionExec}
                                actions["r"] = actionPanelAction{label: "Archive item", kind: actionPanelActionExec}
                                actions["q"] = actionPanelAction{label: "Quit", kind: actionPanelActionExec}
                        }
                case viewOutline:
                        actions["enter"] = actionPanelAction{label: "Open item", kind: actionPanelActionExec}
                        actions["v"] = actionPanelAction{label: "Cycle view mode", kind: actionPanelActionExec}
                        actions["O"] = actionPanelAction{label: "Outline…", kind: actionPanelActionNav, next: actionPanelOutline}
                        actions["S"] = actionPanelAction{label: "Edit outline statuses…", kind: actionPanelActionExec}
                        if m.splitPreviewVisible() {
                                actions["tab"] = actionPanelAction{label: "Toggle focus (outline/detail)", kind: actionPanelActionExec}
                        }
                        actions["z"] = actionPanelAction{label: "Toggle collapse", kind: actionPanelActionExec}
                        actions["Z"] = actionPanelAction{label: "Collapse/expand all", kind: actionPanelActionExec}
                        actions["y"] = actionPanelAction{label: "Copy item ref (includes --workspace)", kind: actionPanelActionExec}
                        actions["Y"] = actionPanelAction{label: "Copy CLI show command (includes --workspace)", kind: actionPanelActionExec}
                        actions["C"] = actionPanelAction{label: "Add comment", kind: actionPanelActionExec}
                        actions["w"] = actionPanelAction{label: "Add worklog", kind: actionPanelActionExec}
                        actions["p"] = actionPanelAction{label: "Toggle priority", kind: actionPanelActionExec}
                        actions["o"] = actionPanelAction{label: "Toggle on hold", kind: actionPanelActionExec}
                        // Use "u" (like "user") in the action panel to avoid shadowing the global "a: agenda"
                        // entrypoint in context menus.
                        actions["u"] = actionPanelAction{
                                label: "Assign…",
                                kind:  actionPanelActionExec,
                                handler: func(mm appModel) (appModel, tea.Cmd) {
                                        if it, ok := mm.itemsList.SelectedItem().(outlineRowItem); ok {
                                                (&mm).openAssigneePicker(it.row.item.ID)
                                        }
                                        return mm, nil
                                },
                        }
                        actions["t"] = actionPanelAction{label: "Tags…", kind: actionPanelActionExec}
                        actions["d"] = actionPanelAction{label: "Set due", kind: actionPanelActionExec}
                        actions["s"] = actionPanelAction{label: "Set schedule", kind: actionPanelActionExec}
                        actions["D"] = actionPanelAction{label: "Edit description", kind: actionPanelActionExec}
                        actions["r"] = actionPanelAction{label: "Archive item", kind: actionPanelActionExec}
                        actions["q"] = actionPanelAction{label: "Quit", kind: actionPanelActionExec}

                        // Item mutations should be discoverable from both panes when preview is visible.
                        if m.pane == paneOutline || (m.pane == paneDetail && m.splitPreviewVisible()) {
                                actions["e"] = actionPanelAction{label: "Edit title", kind: actionPanelActionExec}
                                actions["n"] = actionPanelAction{label: "New sibling", kind: actionPanelActionExec}
                                actions["N"] = actionPanelAction{label: "New child", kind: actionPanelActionExec}
                                actions[" "] = actionPanelAction{label: "Change status", kind: actionPanelActionExec}
                                actions["shift+left"] = actionPanelAction{label: "Cycle status (prev)", kind: actionPanelActionExec}
                                actions["shift+right"] = actionPanelAction{label: "Cycle status (next)", kind: actionPanelActionExec}
                                actions["m"] = actionPanelAction{label: "Move to outline…", kind: actionPanelActionExec}
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

func (m *appModel) workspaceLabel() string {
        if m == nil {
                return "workspace"
        }
        if w := strings.TrimSpace(m.workspace); w != "" {
                if badge := m.gitStatusBadgeText(); badge != "" {
                        return w + " " + badge
                }
                return w
        }
        return "workspace"
}

const eventsTailLimit = 500

func (m *appModel) refreshEventsTail() {
        if m == nil {
                return
        }
        evs, err := store.ReadEventsTail(m.dir, eventsTailLimit)
        if err != nil {
                // Best-effort: history is optional UI sugar.
                m.eventsTail = nil
                m.debugLogf("read events tail: %v", err)
                return
        }
        m.eventsTail = evs
}

func viewToString(v view) string {
        switch v {
        case viewProjects:
                return "projects"
        case viewOutlines:
                return "outlines"
        case viewOutline:
                return "outline"
        case viewItem:
                return "item"
        case viewAgenda:
                return "agenda"
        case viewArchived:
                return "archived"
        default:
                return "projects"
        }
}

func viewFromString(s string) (view, bool) {
        switch strings.TrimSpace(strings.ToLower(s)) {
        case "projects":
                return viewProjects, true
        case "outlines":
                return viewOutlines, true
        case "outline":
                return viewOutline, true
        case "item":
                return viewItem, true
        case "agenda":
                return viewAgenda, true
        case "archived":
                return viewArchived, true
        default:
                return viewProjects, false
        }
}

func paneToString(p pane) string {
        switch p {
        case paneOutline:
                return "outline"
        case paneDetail:
                return "detail"
        default:
                return "outline"
        }
}

func paneFromString(s string) (pane, bool) {
        switch strings.TrimSpace(strings.ToLower(s)) {
        case "outline":
                return paneOutline, true
        case "detail":
                return paneDetail, true
        default:
                return paneOutline, false
        }
}

func outlineViewModeToString(v outlineViewMode) string {
        switch v {
        case outlineViewModeListPreview:
                return "list+preview"
        case outlineViewModeColumns:
                return "columns"
        default:
                return "list"
        }
}

func outlineViewModeFromString(s string) (outlineViewMode, bool) {
        switch strings.TrimSpace(strings.ToLower(s)) {
        case "columns":
                return outlineViewModeColumns, true
        case "document":
                // Back-compat: "document" was an experimental mode; treat it as list.
                return outlineViewModeList, true
        case "list+descriptions", "list-descriptions", "descriptions", "list+desc", "list-desc":
                // Back-compat: descriptions mode is now the default list.
                return outlineViewModeList, true
        case "list+preview", "list-preview", "preview", "split":
                return outlineViewModeListPreview, true
        case "list":
                return outlineViewModeList, true
        default:
                return outlineViewModeList, false
        }
}

func (m appModel) snapshotTUIState() *store.TUIState {
        st := &store.TUIState{
                View:              viewToString(m.view),
                SelectedProjectID: strings.TrimSpace(m.selectedProjectID),
                SelectedOutlineID: strings.TrimSpace(m.selectedOutlineID),
                OpenItemID:        strings.TrimSpace(m.openItemID),
                ReturnView:        "",
                AgendaReturnView:  "",
                Pane:              paneToString(m.pane),
                ShowPreview:       m.showPreview,
        }

        if len(m.recentItemIDs) > 0 {
                st.RecentItemIDs = append([]string(nil), m.recentItemIDs...)
        }

        if m.hasReturnView {
                st.ReturnView = viewToString(m.returnView)
        }
        if m.hasAgendaReturnView {
                st.AgendaReturnView = viewToString(m.agendaReturnView)
        }

        if len(m.outlineViewMode) > 0 {
                st.OutlineViewMode = map[string]string{}
                for id, v := range m.outlineViewMode {
                        if strings.TrimSpace(id) == "" {
                                continue
                        }
                        st.OutlineViewMode[id] = outlineViewModeToString(v)
                }
        }

        return st
}

func (m *appModel) applySavedTUIState(st *store.TUIState) {
        if m == nil || st == nil || m.db == nil {
                return
        }

        // Restore per-outline view mode.
        if len(st.OutlineViewMode) > 0 {
                m.outlineViewMode = map[string]outlineViewMode{}
                for id, mode := range st.OutlineViewMode {
                        id = strings.TrimSpace(id)
                        if id == "" {
                                continue
                        }
                        if v, ok := outlineViewModeFromString(mode); ok {
                                m.outlineViewMode[id] = v
                        }
                }
        }

        // Restore split-preview state (it may be forced off later due to width).
        if p, ok := paneFromString(st.Pane); ok {
                m.pane = p
        }
        m.showPreview = st.ShowPreview

        // Restore recent item visits (best-effort; drop missing/archived ids).
        if len(st.RecentItemIDs) > 0 {
                m.recentItemIDs = nil
                for _, id := range st.RecentItemIDs {
                        id = strings.TrimSpace(id)
                        if id == "" {
                                continue
                        }
                        it, ok := m.db.FindItem(id)
                        if !ok || it == nil || it.Archived {
                                continue
                        }
                        // Preserve stored order while de-duping/capping.
                        already := false
                        for _, cur := range m.recentItemIDs {
                                if cur == id {
                                        already = true
                                        break
                                }
                        }
                        if already {
                                continue
                        }
                        m.recentItemIDs = append(m.recentItemIDs, id)
                        if len(m.recentItemIDs) >= maxRecentItems {
                                break
                        }
                }
        }

        wantView, _ := viewFromString(st.View)

        // Archived is a standalone view (doesn't require project/outline context).
        if wantView == viewArchived {
                m.view = viewArchived
                m.refreshArchived()
                return
        }

        // If we were on an item view, prefer the item's project/outline to keep breadcrumbs consistent.
        openItemID := strings.TrimSpace(st.OpenItemID)
        if wantView == viewItem && openItemID != "" {
                if it, ok := m.db.FindItem(openItemID); ok && it != nil && !it.Archived {
                        m.selectedProjectID = it.ProjectID
                        m.selectedOutlineID = it.OutlineID
                } else {
                        wantView = viewProjects
                        openItemID = ""
                }
        }

        // Resolve/select project.
        projectID := strings.TrimSpace(m.selectedProjectID)
        if projectID == "" {
                projectID = strings.TrimSpace(st.SelectedProjectID)
        }
        if projectID == "" {
                projectID = strings.TrimSpace(m.db.CurrentProjectID)
        }
        if projectID != "" {
                if p, ok := m.db.FindProject(projectID); !ok || p == nil || p.Archived {
                        projectID = ""
                }
        }
        if projectID == "" {
                for _, p := range m.db.Projects {
                        if !p.Archived {
                                projectID = p.ID
                                break
                        }
                }
        }

        m.refreshProjects()
        if projectID != "" {
                m.selectedProjectID = projectID
                selectListItemByID(&m.projectsList, projectID)
        }

        // Resolve/select outline if needed.
        outlineID := strings.TrimSpace(m.selectedOutlineID)
        if outlineID == "" {
                outlineID = strings.TrimSpace(st.SelectedOutlineID)
        }
        if outlineID != "" {
                if o, ok := m.db.FindOutline(outlineID); !ok || o == nil || o.Archived || (projectID != "" && o.ProjectID != projectID) {
                        outlineID = ""
                }
        }
        if outlineID == "" && projectID != "" {
                for _, o := range m.db.Outlines {
                        if o.ProjectID == projectID && !o.Archived {
                                outlineID = o.ID
                                break
                        }
                }
        }

        if wantView == viewProjects {
                m.view = viewProjects
                return
        }

        // Outlines view requires a project selection.
        if projectID == "" {
                m.view = viewProjects
                return
        }

        m.refreshOutlines(projectID)
        m.view = viewOutlines
        if outlineID != "" {
                m.selectedOutlineID = outlineID
                selectListItemByID(&m.outlinesList, outlineID)
        }

        if wantView == viewOutlines {
                return
        }

        // Outline/item views require a selected outline.
        if outlineID == "" {
                m.view = viewOutlines
                return
        }
        ol, ok := m.db.FindOutline(outlineID)
        if !ok || ol == nil {
                m.view = viewOutlines
                return
        }

        m.selectedOutline = ol

        // Backward compatibility: older state stored preview as a separate boolean. If that was set and
        // the outline mode is still "list" (or missing), upgrade to list+preview.
        mode := m.outlineViewModeForID(outlineID)
        if st.ShowPreview && mode == outlineViewModeList {
                mode = outlineViewModeListPreview
        }
        m.setOutlineViewMode(outlineID, mode)

        m.collapsed = map[string]bool{}
        m.refreshItems(*ol)
        m.openItemID = ""
        m.hasReturnView = false
        m.hasAgendaReturnView = false

        if wantView == viewOutline {
                m.view = viewOutline
                // View mode (including preview) is restored via per-outline TUI state.
                return
        }

        if wantView == viewAgenda {
                m.view = viewAgenda
                // Agenda doesn't currently support preview (preview is part of per-outline view modes).
                m.showPreview = false
                m.pane = paneOutline
                if rv, ok := viewFromString(st.AgendaReturnView); ok && rv != viewAgenda {
                        m.hasAgendaReturnView = true
                        m.agendaReturnView = rv
                }
                m.refreshAgenda()
                return
        }

        // Item view.
        if wantView == viewItem && openItemID != "" {
                if it, ok := m.db.FindItem(openItemID); ok && it != nil && !it.Archived {
                        m.openItemID = it.ID
                        m.view = viewItem
                        m.recordRecentItemVisit(m.openItemID)
                        m.itemFocus = itemFocusTitle
                        m.itemCommentIdx = 0
                        m.itemWorklogIdx = 0
                        m.itemHistoryIdx = 0
                        m.itemSideScroll = 0
                        m.itemDetailScroll = 0
                        m.showPreview = false
                        m.pane = paneOutline

                        if rv, ok := viewFromString(st.ReturnView); ok && rv != viewItem {
                                m.hasReturnView = true
                                m.returnView = rv
                        } else {
                                m.hasReturnView = true
                                m.returnView = viewOutline
                        }
                        return
                }
        }

        // Fallback if anything doesn't resolve.
        m.view = viewOutlines
}

func (m appModel) quitWithStateCmd() tea.Cmd {
        snap := m
        return func() tea.Msg {
                _ = snap.store.SaveTUIState(snap.snapshotTUIState())
                return tea.Quit()
        }
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
        in := m.inputDbg
        body := strings.Join([]string{
                "DEBUG (toggle with D)",
                func() string {
                        if in.lastAt.IsZero() {
                                return "last input: (none)"
                        }
                        s := strings.TrimSpace(in.lastStr)
                        if s == "" {
                                s = "(empty)"
                        }
                        // Avoid huge dumps.
                        if len(s) > 140 {
                                s = s[:140] + "…"
                        }
                        return fmt.Sprintf("last input: %s  %s  %q", in.lastAt.Format("15:04:05.000"), in.lastType, s)
                }(),
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
                BorderForeground(colorMuted).
                Background(colorSurfaceBg).
                Foreground(colorSurfaceFg).
                Padding(1, 2)
        return box.Render(body)
}

func (m appModel) Init() tea.Cmd { return tickReload() }

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

        if m.modal == modalCapture && m.capture != nil {
                return m.capture.View()
        }

        var body string
        switch m.view {
        case viewProjects:
                body = m.viewProjects()
        case viewOutlines:
                body = m.viewOutlines()
        case viewAgenda:
                body = m.viewAgenda()
        case viewArchived:
                body = m.viewArchived()
        case viewOutline:
                body = m.viewOutline()
        case viewItem:
                body = m.viewItem()
        default:
                body = ""
        }

        footer := m.footerBlock()
        // Keep the footer (minibuffer + shortcuts) anchored to the bottom by inserting
        // the "extra" vertical space between body and footer.
        w := m.width
        if w < 0 {
                w = 0
        }
        h := m.height
        if h <= 0 {
                // Fallback: no reliable terminal height; render compactly.
                return strings.Join([]string{body, footer}, "\n")
        }
        blankLine := strings.Repeat(" ", w)

        bodyLines := strings.Split(body, "\n")
        footerLines := strings.Split(footer, "\n")

        gap := 1
        if len(bodyLines)+len(footerLines) < h {
                gap = h - (len(bodyLines) + len(footerLines))
        }

        lines := make([]string, 0, len(bodyLines)+gap+len(footerLines))
        lines = append(lines, bodyLines...)
        for i := 0; i < gap; i++ {
                lines = append(lines, blankLine)
        }
        lines = append(lines, footerLines...)
        return strings.Join(lines, "\n")
}

func (m *appModel) breadcrumbText() string {
        parts := []string{m.workspaceLabel()}
        if m.view == viewAgenda {
                return strings.Join(append(parts, "agenda"), " > ")
        }
        if m.view == viewArchived {
                return strings.Join(append(parts, "archived"), " > ")
        }
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
                base := strings.Join(parts, " > ")
                if m.itemsList.IsFiltered() {
                        f := strings.TrimSpace(m.itemsList.FilterValue())
                        if f != "" {
                                return base + "  /" + f
                        }
                }
                return base
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
        frameH := m.frameHeight()
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

        contentW := w - 2*splitOuterMargin
        if contentW < 10 {
                contentW = w
        }

        crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
        body := m.listBodyWithOverflowHint(&m.projectsList, contentW, bodyHeight)
        main := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + body
        main = lipgloss.NewStyle().Width(w).Padding(0, splitOuterMargin).Render(main)
        if m.modal == modalNone {
                return main
        }
        bg := dimBackground(main)
        fg := m.renderModal()
        return overlayCenter(bg, fg, w, frameH)
}

func (m *appModel) viewOutlines() string {
        frameH := m.frameHeight()
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

        contentW := w - 2*splitOuterMargin
        if contentW < 10 {
                contentW = w
        }

        crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
        body := m.listBodyWithOverflowHint(&m.outlinesList, contentW, bodyHeight)
        main := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + body
        main = lipgloss.NewStyle().Width(w).Padding(0, splitOuterMargin).Render(main)
        if m.modal == modalNone {
                return main
        }
        bg := dimBackground(main)
        fg := m.renderModal()
        return overlayCenter(bg, fg, w, frameH)
}

func (m *appModel) viewAgenda() string {
        frameH := m.frameHeight()
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

        contentW := w - 2*splitOuterMargin
        if contentW < 10 {
                contentW = w
        }

        crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
        body := m.listBodyWithOverflowHint(&m.agendaList, contentW, bodyHeight)
        main := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + body
        main = lipgloss.NewStyle().Width(w).Padding(0, splitOuterMargin).Render(main)
        if m.modal == modalNone {
                return main
        }
        bg := dimBackground(main)
        fg := m.renderModal()
        return overlayCenter(bg, fg, w, frameH)
}

func (m *appModel) viewArchived() string {
        frameH := m.frameHeight()
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

        contentW := w - 2*splitOuterMargin
        if contentW < 10 {
                contentW = w
        }

        crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
        body := m.listBodyWithOverflowHint(&m.archivedList, contentW, bodyHeight)
        main := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + body
        main = lipgloss.NewStyle().Width(w).Padding(0, splitOuterMargin).Render(main)
        if m.modal == modalNone {
                return main
        }
        bg := dimBackground(main)
        fg := m.renderModal()
        return overlayCenter(bg, fg, w, frameH)
}

func (m appModel) footerText() string {
        // The footer should list only global shortcuts. Contextual shortcuts belong in the action panel.
        base := "x/?: actions  g: nav  a: agenda  c: capture  q: quit"
        if m.modal == modalActionPanel {
                return "action: type a key  backspace/esc: back  ctrl+g: close"
        }
        // Modal-specific hints should still be shown because the action panel isn't reachable while a modal is open.
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
        if m.modal == modalEditOutlineStatuses {
                return "outline statuses: a:add  r:rename  e:toggle end  n:toggle note  d:delete  ctrl+k/j:move  esc:close"
        }
        if m.modal == modalAddOutlineStatus {
                return "add status: type label, enter: add, esc: cancel"
        }
        if m.modal == modalRenameOutlineStatus {
                return "rename status: type label, enter: save, esc: cancel"
        }
        if m.modal != modalNone {
                if m.modal == modalConfirmArchive {
                        return "archive: y/enter: confirm  n/esc: cancel"
                }
                if m.modal == modalPickStatus {
                        return "status: enter: set  esc/ctrl+g: cancel"
                }
                if m.modal == modalPickOutline {
                        return "move outline: enter: move  esc/ctrl+g: cancel"
                }
                if m.modal == modalPickAssignee {
                        return "assign: enter: set  esc/ctrl+g: cancel"
                }
                if m.modal == modalEditTags {
                        return "tags: tab: focus  enter: add/toggle  esc/ctrl+g: close"
                }
                if m.modal == modalEditTitle {
                        return "edit title: type, enter/ctrl+s: save, esc/ctrl+g: cancel"
                }
                if m.modal == modalEditOutlineName {
                        return "rename outline: type, enter/ctrl+s: save, esc/ctrl+g: cancel"
                }
                if m.modal == modalAddComment {
                        return "comment: tab: focus  ctrl+o: editor  ctrl+s: save  esc/ctrl+g: cancel"
                }
                if m.modal == modalReplyComment {
                        return "reply: tab: focus  ctrl+o: editor  ctrl+s: save  esc/ctrl+g: cancel"
                }
                if m.modal == modalAddWorklog {
                        return "worklog: tab: focus  ctrl+o: editor  ctrl+s: save  esc/ctrl+g: cancel"
                }
                if m.modal == modalEditDescription {
                        return "description: tab: focus  ctrl+o: editor  ctrl+s: save  esc/ctrl+g: cancel"
                }
                if m.modal == modalStatusNote {
                        return "status note: tab: focus  ctrl+o: editor  ctrl+s: save  esc/ctrl+g: cancel"
                }
                if m.modal == modalEditOutlineDescription {
                        return "outline description: tab: focus  ctrl+o: editor  ctrl+s: save  esc/ctrl+g: cancel"
                }
                if m.modal == modalSetDue {
                        return "due: tab: focus  enter/ctrl+s: save  ctrl+c: clear  esc/ctrl+g: cancel"
                }
                if m.modal == modalSetSchedule {
                        return "schedule: tab: focus  enter/ctrl+s: save  ctrl+c: clear  esc/ctrl+g: cancel"
                }
                return "new item: type title, enter/ctrl+s: save, esc/ctrl+g: cancel"
        }
        return base
}

func (m appModel) footerBlock() string {
        keyHelp := lipgloss.NewStyle().Faint(true).Render(m.footerText())
        return m.minibufferView() + "\n" + keyHelp
}

// frameHeight returns the vertical space available for the main view content (everything above the footer),
// leaving room for the footer itself plus the single blank separator line that View() inserts.
func (m appModel) frameHeight() int {
        h := m.height
        if h < 0 {
                h = 0
        }
        footerLines := len(strings.Split(m.footerBlock(), "\n"))
        frame := h - footerLines - 1
        if frame < 0 {
                frame = 0
        }
        return frame
}

// listBodyWithOverflowHint renders a bubbles list and adds a muted hint line when the list can scroll.
// It does NOT increase the total rendered height: when the hint is shown, we shrink the list height by 1
// to reserve space for the hint line.
func (m *appModel) listBodyWithOverflowHint(l *list.Model, width, height int) string {
        if m == nil || l == nil {
                return ""
        }
        if height <= 0 {
                l.SetSize(width, 0)
                return l.View()
        }

        // First pass at the full height to learn whether it overflows.
        l.SetSize(width, height)
        if l.Paginator.TotalPages <= 1 {
                return l.View()
        }

        // Reserve one row for the hint line.
        l.SetSize(width, height-1)
        body := l.View()

        hasAbove := l.Paginator.Page > 0
        hasBelow := l.Paginator.Page < l.Paginator.TotalPages-1
        switch {
        case hasAbove && hasBelow:
                return body + "\n" + styleMuted().Width(width).Render("↑ more / ↓ more")
        case hasAbove:
                return body + "\n" + styleMuted().Width(width).Render("↑ more")
        case hasBelow:
                return body + "\n" + styleMuted().Width(width).Render("↓ more")
        default:
                return body
        }
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
                Background(colorControlBg).
                Foreground(colorSurfaceFg).
                Render(txt)
}

func (m *appModel) showMinibuffer(text string) {
        m.minibufferText = text
}

func (m appModel) renderActionPanel() string {
        if m.curActionPanelKind() == actionPanelCapture {
                return m.renderCaptureActionPanel()
        }

        title := m.actionPanelTitle()
        entries := m.actionPanelEntries()
        if len(entries) == 0 {
                return renderModalBox(m.width, title, "(no actions)")
        }

        // Approximate the available content width inside the modal box.
        // This mirrors renderModalBox's sizing and accounts for horizontal padding.
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
        contentW := modalW - 4 // Padding(1,2) => 2 columns of padding on each side.
        if contentW < 20 {
                contentW = 20
        }

        actions := m.actionPanelActions()
        seen := map[string]bool{}

        isFocusedItemContext := m.curActionPanelKind() == actionPanelContext &&
                ((m.view == viewOutline && (m.pane == paneOutline || (m.pane == paneDetail && m.splitPreviewVisible()))) ||
                        m.view == viewItem)

        selectedKey := strings.TrimSpace(m.actionPanelSelectedKey)

        renderCell := func(k string, a actionPanelAction) string {
                line := fmt.Sprintf("%-12s %s", actionPanelDisplayKey(k), a.label)
                if k == selectedKey {
                        return lipgloss.NewStyle().
                                Foreground(colorSelectedFg).
                                Background(colorSelectedBg).
                                Bold(true).
                                Render(line)
                }
                return line
        }

        cutToWidth := func(s string, w int) string {
                if w <= 0 {
                        return s
                }
                if xansi.StringWidth(s) <= w {
                        return s
                }
                if w <= 1 {
                        return xansi.Cut(s, 0, w)
                }
                return xansi.Cut(s, 0, w-1) + "…"
        }

        addKey := func(k string, cells *[]string) bool {
                if seen[k] {
                        return false
                }
                a, ok := actions[k]
                if !ok {
                        return false
                }
                seen[k] = true
                *cells = append(*cells, renderCell(k, a))
                return true
        }

        type sectionBlock struct {
                header string
                lines  []string // already cut/padded appropriately for its column width later
        }
        blocks := []sectionBlock{}

        addSection := func(header string, keys []string) {
                cells := []string{}
                for _, k := range keys {
                        addKey(k, &cells)
                }
                if len(cells) == 0 {
                        return
                }
                lns := []string{strings.ToUpper(strings.TrimSpace(header))}
                lns = append(lns, cells...) // keep per-group as a single vertical list
                blocks = append(blocks, sectionBlock{header: header, lines: lns})
        }

        // Only the root action panel (opened with x) shows global entrypoints.
        if m.curActionPanelKind() == actionPanelContext {
                // Note: "Global" group below will pick up nav actions (g/a/c) automatically.
        }

        // Navigation group:
        // - In the context panel, only include actions that actually navigate to a subpanel.
        //   (We don't want to "steal" exec actions like "v" Cycle view mode from the View section.)
        // - In the Go to panel, show destinations explicitly.
        if m.curActionPanelKind() == actionPanelNav {
                addSection("Destinations", []string{"p", "A", "o", "l", "i"})
                // Note: "Recent items" are rendered as a special full-width block at the bottom.
                // Mark them as seen so they don't fall into "Other".
                for _, k := range []string{"1", "2", "3", "4", "5"} {
                        seen[k] = true
                }
        } else if !isFocusedItemContext {
                navKeys := []string{}
                // Stable "nice" order first.
                for _, k := range []string{"g", "a", "c"} {
                        if a, ok := actions[k]; ok && a.kind == actionPanelActionNav {
                                navKeys = append(navKeys, k)
                        }
                }
                // Any other nav actions (sorted).
                other := []string{}
                for k, a := range actions {
                        if a.kind != actionPanelActionNav {
                                continue
                        }
                        if k == "g" || k == "a" || k == "c" {
                                continue
                        }
                        other = append(other, k)
                }
                sort.Strings(other)
                navKeys = append(navKeys, other...)
                addSection("Go to", navKeys)
        }

        // When focused on an item, present clearer grouped actions.
        if isFocusedItemContext {
                switch m.view {
                case viewItem:
                        // Full-screen item page: show item work + global entrypoints.
                        addSection("Item", []string{"e", "D", "p", "o", "u", "t", "d", "s", " ", "C", "w", "m", "y", "Y", "r"})

                        globalKeys := []string{}
                        for _, k := range []string{"g", "a", "c"} {
                                if a, ok := actions[k]; ok && a.kind == actionPanelActionNav {
                                        globalKeys = append(globalKeys, k)
                                }
                        }
                        if _, ok := actions["q"]; ok {
                                globalKeys = append(globalKeys, "q")
                        }
                        addSection("Global", globalKeys)

                default:
                        // Regroup by "what you're operating on":
                        // - Outline View: view/navigation of the outline split view
                        // - Item: mutations + collaboration actions on the selected item
                        // - Global: app-level entrypoints (navigate / agenda / capture / quit)

                        // Outline-level view controls (preview is now part of the v cycle; outline actions are under O).
                        addSection("Outline View", []string{"enter", "v", "O", "S", "tab", "z", "Z"})

                        // Item work: all changes + notes/comments, and related helpers.
                        addSection("Item", []string{
                                "e", "n", "N", // title/new items
                                " ", "shift+left", "shift+right", // status
                                "p", "o", "u", "t", "d", "s", "D", // priority/on-hold/assign/tags/due/schedule/description
                                "m",      // move
                                "C", "w", // comment/worklog
                                "y", "Y", // copy helpers (still item-scoped)
                                "r", // archive
                        })

                        // Global entrypoints.
                        globalKeys := []string{}
                        for _, k := range []string{"g", "a", "c"} {
                                if a, ok := actions[k]; ok && a.kind == actionPanelActionNav {
                                        globalKeys = append(globalKeys, k)
                                }
                        }
                        // Quit is always global.
                        if _, ok := actions["q"]; ok {
                                globalKeys = append(globalKeys, "q")
                        }
                        addSection("Global", globalKeys)
                }
        } else {
                // Default: show remaining actions in sorted order.
                rest := make([]string, 0, len(entries))
                for _, e := range entries {
                        if seen[e.key] {
                                continue
                        }
                        rest = append(rest, e.key)
                }
                sort.Strings(rest)
                cells := []string{}
                for _, k := range rest {
                        addKey(k, &cells)
                }
                if len(cells) > 0 {
                        lns := []string{strings.ToUpper("Other")}
                        lns = append(lns, cells...)
                        blocks = append(blocks, sectionBlock{header: "Other", lines: lns})
                }
        }

        // Render blocks: prefer two columns of whole sections when there's room.
        const colGap = 4
        const minColW = 34
        useTwoCols := len(blocks) > 1 && contentW >= (minColW*2+colGap)

        lines := []string{}
        if !useTwoCols {
                for bi := range blocks {
                        for _, ln := range blocks[bi].lines {
                                lines = append(lines, cutToWidth(ln, contentW))
                        }
                        lines = append(lines, "")
                }
        } else {
                colW := (contentW - colGap) / 2
                if colW < minColW {
                        // Safety fallback to single column.
                        for bi := range blocks {
                                for _, ln := range blocks[bi].lines {
                                        lines = append(lines, cutToWidth(ln, contentW))
                                }
                                lines = append(lines, "")
                        }
                } else {
                        left := []string{}
                        right := []string{}

                        colHeight := func(col []string) int {
                                // Trim trailing blanks for height.
                                n := len(col)
                                for n > 0 && strings.TrimSpace(col[n-1]) == "" {
                                        n--
                                }
                                return n
                        }

                        appendBlock := func(col *[]string, b sectionBlock) {
                                for _, ln := range b.lines {
                                        *col = append(*col, cutToWidth(ln, colW))
                                }
                                *col = append(*col, "")
                        }

                        // Greedy balance by line count, but keep each section as an atomic block.
                        for _, b := range blocks {
                                if colHeight(left) <= colHeight(right) {
                                        appendBlock(&left, b)
                                } else {
                                        appendBlock(&right, b)
                                }
                        }

                        trimTrailingBlanks := func(col []string) []string {
                                for len(col) > 0 && strings.TrimSpace(col[len(col)-1]) == "" {
                                        col = col[:len(col)-1]
                                }
                                return col
                        }
                        left = trimTrailingBlanks(left)
                        right = trimTrailingBlanks(right)

                        maxN := len(left)
                        if len(right) > maxN {
                                maxN = len(right)
                        }
                        for i := 0; i < maxN; i++ {
                                l := ""
                                r := ""
                                if i < len(left) {
                                        l = left[i]
                                }
                                if i < len(right) {
                                        r = right[i]
                                }
                                l = cutToWidth(l, colW)
                                if n := xansi.StringWidth(l); n < colW {
                                        l += strings.Repeat(" ", colW-n)
                                }
                                if strings.TrimSpace(r) == "" {
                                        lines = append(lines, strings.TrimRight(l, " "))
                                        continue
                                }
                                r = cutToWidth(r, colW)
                                line := l + strings.Repeat(" ", colGap) + r
                                lines = append(lines, strings.TrimRight(line, " "))
                        }
                }
        }

        // Trim trailing blank lines.
        for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
                lines = lines[:len(lines)-1]
        }

        // Special full-width section at the bottom: recent items (Go to panel only).
        if m.curActionPanelKind() == actionPanelNav {
                // Build recent item rows in an outline-like layout. Important: the modal uses a
                // background style wrapper (renderModalBox), but any nested lipgloss rendering
                // emits ANSI resets. To prevent "holes" where the terminal background shows
                // through, we explicitly render most segments with the modal's surface bg.
                rec := make([]string, 0, maxRecentItems+1)
                if m.db != nil {
                        const keyColW = 3
                        rowW := contentW - keyColW
                        if rowW < 8 {
                                rowW = 8
                        }

                        renderRecentRow := func(outline model.Outline, it model.Item, doneChildren, totalChildren int, width int) string {
                                // Base style: force modal surface background.
                                base := lipgloss.NewStyle().Foreground(colorSurfaceFg).Background(colorSurfaceBg)

                                twisty := " "
                                if totalChildren > 0 {
                                        twisty = "▸"
                                }
                                leadSeg := base.Render(twisty + " ")

                                // Status (styled like outline, but ensure surface bg).
                                statusID := strings.TrimSpace(it.StatusID)
                                statusTxt := strings.ToUpper(strings.TrimSpace(statusLabel(outline, statusID)))
                                statusSeg := ""
                                statusRaw := ""
                                if statusTxt != "" {
                                        style := statusNonEndStyle
                                        if isEndState(outline, statusID) {
                                                style = statusEndStyle
                                        }
                                        style = style.Copy().Background(colorSurfaceBg)
                                        statusSeg = style.Render(statusTxt) + base.Render(" ")
                                        statusRaw = statusTxt + " "
                                }

                                // Progress cookie: keep the colored "pill", but ensure the leading space
                                // uses the modal surface background (renderProgressCookie starts with a raw space).
                                progressCookie := ""
                                if totalChildren > 0 {
                                        raw := renderProgressCookie(doneChildren, totalChildren)
                                        if strings.HasPrefix(raw, " ") {
                                                progressCookie = base.Render(" ") + strings.TrimPrefix(raw, " ")
                                        } else {
                                                progressCookie = base.Render(" ") + raw
                                        }
                                }
                                progressW := xansi.StringWidth(progressCookie)

                                // Inline metadata (priority / on hold), matching outline semantics.
                                metaParts := make([]string, 0, 2)
                                if it.Priority {
                                        st := metaPriorityStyle.Copy().Background(colorSurfaceBg)
                                        metaParts = append(metaParts, st.Render("priority"))
                                }
                                if it.OnHold {
                                        st := metaOnHoldStyle.Copy().Background(colorSurfaceBg)
                                        metaParts = append(metaParts, st.Render("on hold"))
                                }
                                inlineMetaSeg := strings.Join(metaParts, base.Render(" "))
                                inlineMetaW := xansi.StringWidth(inlineMetaSeg)

                                title := strings.TrimSpace(it.Title)
                                if title == "" {
                                        title = "(untitled)"
                                }

                                leadW := xansi.StringWidth(twisty + " ")
                                statusW := xansi.StringWidth(statusRaw)
                                availTitle := width - leadW - statusW
                                if progressCookie != "" {
                                        availTitle -= progressW
                                }
                                if inlineMetaSeg != "" {
                                        availTitle -= (1 + inlineMetaW) // space + inline metadata
                                }
                                if availTitle < 0 {
                                        availTitle = 0
                                }

                                titleTrunc := truncateText(title, availTitle)
                                titleStyle := base
                                if isEndState(outline, statusID) {
                                        titleStyle = faintIfDark(base.Copy()).
                                                Foreground(colorMuted).
                                                Strikethrough(true)
                                }
                                titleSeg := titleStyle.Render(titleTrunc)

                                metaSpacer := ""
                                if inlineMetaSeg != "" {
                                        metaSpacer = base.Render(" ")
                                }

                                out := leadSeg + statusSeg + titleSeg + progressCookie + metaSpacer + inlineMetaSeg
                                // Ensure full-width fill uses surface bg.
                                curW := xansi.StringWidth(out)
                                if curW < width {
                                        out += base.Render(strings.Repeat(" ", width-curW))
                                } else if curW > width {
                                        out = xansi.Cut(out, 0, width) + "\x1b[0m"
                                }
                                return out
                        }

                        for i := 0; i < len(m.recentItemIDs) && i < maxRecentItems; i++ {
                                id := strings.TrimSpace(m.recentItemIDs[i])
                                if id == "" {
                                        continue
                                }
                                it, ok := m.db.FindItem(id)
                                if !ok || it == nil || it.Archived {
                                        continue
                                }
                                ol, ok := m.db.FindOutline(strings.TrimSpace(it.OutlineID))
                                if !ok || ol == nil || ol.Archived {
                                        continue
                                }

                                children := m.db.ChildrenOf(it.ID)
                                doneChildren, totalChildren := countProgressChildren(*ol, children)

                                rendered := renderRecentRow(*ol, *it, doneChildren, totalChildren, rowW)
                                key := strconv.Itoa(i + 1)
                                line := fmt.Sprintf("%-3s%s", actionPanelDisplayKey(key), rendered)
                                rec = append(rec, cutToWidth(line, contentW))
                        }
                }
                if len(rec) > 0 {
                        if len(lines) > 0 {
                                lines = append(lines, "")
                        }
                        lines = append(lines, "RECENT ITEMS")
                        lines = append(lines, rec...)
                }
        }

        body := strings.Join(lines, "\n")
        body += "\n\nbackspace/esc: back    ctrl+g: close"
        return renderModalBox(m.width, title, body)
}

func (m appModel) renderCaptureActionPanel() string {
        title := "Capture"
        desc := "Type a capture template key sequence (org-capture style). Backspace deletes a key. Enter selects when complete. ctrl+t manages templates."

        cfg, err := store.LoadConfig()
        if err != nil {
                body := strings.TrimSpace(desc) + "\n\n" + "Error: " + err.Error()
                return renderModalBox(m.width, title, body)
        }
        if err := store.ValidateCaptureTemplates(cfg); err != nil {
                body := strings.TrimSpace(desc) + "\n\n" + "Error: " + err.Error()
                return renderModalBox(m.width, title, body)
        }
        if cfg == nil || len(cfg.CaptureTemplates) == 0 {
                body := strings.TrimSpace(desc) + "\n\n" + "No capture templates configured. Press ctrl+t to add one."
                return renderModalBox(m.width, title, body)
        }

        prefix := append([]string{}, m.captureKeySeq...)
        exact, next := captureTemplatesAtPrefix(cfg.CaptureTemplates, prefix)

        lines := []string{strings.TrimSpace(desc), ""}
        if len(prefix) == 0 {
                lines = append(lines, "Prefix: (none)")
        } else {
                lines = append(lines, "Prefix: "+strings.Join(prefix, " "))
        }
        lines = append(lines, "")

        // Prefer stable ordering.
        nextKeys := make([]string, 0, len(next))
        for k := range next {
                nextKeys = append(nextKeys, k)
        }
        sort.Strings(nextKeys)

        // Ensure selection points at a visible key when possible.
        selected := strings.TrimSpace(m.actionPanelSelectedKey)
        if selected != "" {
                found := false
                for _, k := range nextKeys {
                        if k == selected {
                                found = true
                                break
                        }
                }
                if !found {
                        selected = ""
                }
        }
        if selected == "" && len(nextKeys) > 0 {
                selected = nextKeys[0]
        }

        if exact != nil && len(nextKeys) == 0 {
                // Completed sequence.
                name := strings.TrimSpace(exact.Name)
                if name == "" {
                        name = "(unnamed)"
                }
                lines = append(lines, "Selected: "+name)
                lines = append(lines, "")
                lines = append(lines, "Press Enter to start capture.")
                return renderModalBox(m.width, title, strings.Join(lines, "\n"))
        }

        lines = append(lines, "Options:")
        if len(nextKeys) == 0 {
                lines = append(lines, "  (no matches)")
        } else {
                for _, k := range nextKeys {
                        lbl := next[k]
                        line := fmt.Sprintf("%-3s %s", k, lbl)
                        if k == selected {
                                line = lipgloss.NewStyle().
                                        Foreground(colorSelectedFg).
                                        Background(colorSelectedBg).
                                        Bold(true).
                                        Render(line)
                        }
                        lines = append(lines, line)
                }
        }

        if exact != nil {
                name := strings.TrimSpace(exact.Name)
                if name == "" {
                        name = "(unnamed)"
                }
                lines = append(lines, "")
                lines = append(lines, "Enter also works now: "+name)
        }

        return renderModalBox(m.width, title, strings.Join(lines, "\n"))
}

func captureTemplatesAtPrefix(templates []store.CaptureTemplate, prefix []string) (*store.CaptureTemplate, map[string]string) {
        var exact *store.CaptureTemplate
        next := map[string]string{} // next key -> label

        for i := range templates {
                t := templates[i]
                keys, err := store.NormalizeCaptureTemplateKeys(t.Keys)
                if err != nil {
                        continue
                }
                if len(prefix) > len(keys) {
                        continue
                }
                match := true
                for j := 0; j < len(prefix); j++ {
                        if keys[j] != prefix[j] {
                                match = false
                                break
                        }
                }
                if !match {
                        continue
                }

                if len(keys) == len(prefix) {
                        // Exact match at this prefix.
                        exact = &templates[i]
                        continue
                }

                k := keys[len(prefix)]
                // Only compute label once; prefer a leaf template label when unambiguous.
                if _, ok := next[k]; ok {
                        continue
                }
                if len(keys) == len(prefix)+1 {
                        name := strings.TrimSpace(t.Name)
                        if name == "" {
                                name = "(unnamed)"
                        }
                        next[k] = name + "  →  " + captureTemplateTargetLabel(t.Target.Workspace, t.Target.OutlineID)
                } else {
                        next[k] = "(prefix)"
                }
        }

        return exact, next
}

func (m *appModel) reportError(itemID string, err error) tea.Cmd {
        if m == nil || err == nil {
                return nil
        }
        msg := strings.TrimSpace(err.Error())
        if store.IsGitWriteBlocked(err) {
                msg = "write blocked by Git merge/rebase; resolve first (try: clarity sync resolve)"
        }
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
        h := m.frameHeight()
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
        m.agendaList.SetSize(centeredW, h)
        if m.splitPreviewVisible() {
                // In split-preview mode we overlay the right pane on top of the list; keep the list
                // at full width so its content doesn't get squashed.
                contentW := w - 2*splitOuterMargin
                if contentW < 10 {
                        contentW = w
                }
                m.itemsList.SetSize(contentW, h)
        } else {
                // Keep the default capped width for non-outline list views.
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

func renderSplitWithLeftHeader(contentW, frameH, leftW, rightW int, leftHeader string, leftBody string, rightBody string) string {
        // Split view: render the header only over the LEFT pane, so the right pane
        // can start at the top without wasted header padding.
        leftCol := leftHeader + strings.Repeat("\n", breadcrumbGap+1) + leftBody

        // Below top padding, we render a full-height block (stable split rendering).
        contentH := frameH - topPadLines
        if contentH < 0 {
                contentH = 0
        }

        // Important: render the left content at FULL width so it doesn't get squashed;
        // then overlay the right pane on top. This keeps the underlying left layout stable
        // (wrapping/truncation based on full width) while still presenting a split view.
        //
        // Visually, the right pane starts at x = leftW + splitGapW (i.e. 2/3 on the right),
        // and we blank out the split gap so left content doesn't "bleed" under it.
        bg := normalizePane(leftCol, contentW, contentH)
        bgLines := strings.Split(bg, "\n")

        if splitGapW > 0 {
                gapLine := strings.Repeat(" ", splitGapW)
                gap := make([]string, 0, contentH)
                for i := 0; i < contentH; i++ {
                        gap = append(gap, gapLine)
                }
                overlayAt(bgLines, gap, contentW, leftW, 0, splitGapW)
        }

        rightCol := normalizePane(rightBody, rightW, contentH)
        rightLines := strings.Split(rightCol, "\n")
        overlayAt(bgLines, rightLines, contentW, leftW+splitGapW, 0, rightW)

        body := strings.Join(bgLines, "\n")
        body = lipgloss.NewStyle().Width(contentW).Height(contentH).Render(body)
        return strings.Repeat("\n", topPadLines) + body
}

func renderSplitWithLeftHeaderGap(contentW, frameH, leftW, rightW int, leftHeader string, headerGapLines int, leftBody string, rightBody string) string {
        // Like renderSplitWithLeftHeader, but allows the caller to control spacing between the left header
        // and the left body (useful for more compact headers).
        if headerGapLines < 0 {
                headerGapLines = 0
        }
        leftCol := leftHeader + strings.Repeat("\n", headerGapLines) + leftBody

        contentH := frameH - topPadLines
        if contentH < 0 {
                contentH = 0
        }

        // Same overlay strategy as renderSplitWithLeftHeader.
        bg := normalizePane(leftCol, contentW, contentH)
        bgLines := strings.Split(bg, "\n")

        if splitGapW > 0 {
                gapLine := strings.Repeat(" ", splitGapW)
                gap := make([]string, 0, contentH)
                for i := 0; i < contentH; i++ {
                        gap = append(gap, gapLine)
                }
                overlayAt(bgLines, gap, contentW, leftW, 0, splitGapW)
        }

        rightCol := normalizePane(rightBody, rightW, contentH)
        rightLines := strings.Split(rightCol, "\n")
        overlayAt(bgLines, rightLines, contentW, leftW+splitGapW, 0, rightW)

        body := strings.Join(bgLines, "\n")
        body = lipgloss.NewStyle().Width(contentW).Height(contentH).Render(body)
        return strings.Repeat("\n", topPadLines) + body
}

func (m *appModel) outlineLayout() (frameH, bodyH, contentW int) {
        frameH = m.frameHeight()
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
        // The outline view uses an outer margin (like split view) and should be left-aligned.
        // Compute the usable inner width and clamp it for stable rendering.
        innerW := w - 2*splitOuterMargin
        if innerW < 10 {
                innerW = w
        }
        // Use the full available inner width (no maxContentW cap). This keeps outline rows truly full-width.
        contentW = innerW
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
        // Always-present affordance for creating a project (same as pressing "n").
        items = append(items, addProjectRow{})
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
        // Always-present affordance for creating an outline (same as pressing "n").
        items = append(items, addOutlineRow{})
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
        if m.collapsed == nil {
                m.collapsed = map[string]bool{}
        }
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
        // Default outline behavior: start parents collapsed so the view is lean/scannable.
        // Only initialize collapse state for items we haven't seen before (so user toggles
        // persist while the app is running).
        childrenCount := map[string]int{}
        for _, it := range its {
                if it.ParentID == nil || *it.ParentID == "" {
                        continue
                }
                childrenCount[*it.ParentID]++
        }
        for id, n := range childrenCount {
                if n <= 0 {
                        continue
                }
                if _, ok := m.collapsed[id]; !ok {
                        m.collapsed[id] = true
                }
        }
        // In "list+descriptions" mode, items with descriptions are also collapsible and
        // should start collapsed by default.
        for _, it := range its {
                if strings.TrimSpace(it.Description) == "" {
                        continue
                }
                if _, ok := m.collapsed[it.ID]; !ok {
                        m.collapsed[it.ID] = true
                }
        }

        flat := flattenOutline(outline, its, m.collapsed)
        var items []list.Item
        showInlineDescriptions := m.outlineViewModeForID(outline.ID) != outlineViewModeColumns

        for _, row := range flat {
                if showInlineDescriptions && strings.TrimSpace(row.item.Description) != "" {
                        row.hasDescription = true
                }
                // Cache display labels for metadata that needs DB context.
                if row.item.AssignedActorID != nil && strings.TrimSpace(*row.item.AssignedActorID) != "" {
                        row.assignedLabel = actorDisplayLabel(m.db, *row.item.AssignedActorID)
                }
                // Normalize tags for stable display.
                if len(row.item.Tags) > 0 {
                        cleaned := make([]string, 0, len(row.item.Tags))
                        for _, t := range row.item.Tags {
                                t = normalizeTag(t)
                                if t == "" {
                                        continue
                                }
                                cleaned = append(cleaned, t)
                        }
                        row.item.Tags = uniqueSortedStrings(cleaned)
                }
                flash := ""
                if m.flashKind != "" && m.flashItemID != "" && row.item.ID == m.flashItemID {
                        flash = m.flashKind
                }
                items = append(items, outlineRowItem{row: row, outline: outline, flashKind: flash})
                if showInlineDescriptions && row.hasDescription && !row.collapsed {
                        contentW := m.itemsList.Width()
                        if contentW <= 0 {
                                contentW = m.width
                        }
                        if contentW <= 0 {
                                contentW = 80
                        }
                        descDepth := row.depth + 1
                        leadW := (2 * descDepth) + 2 // indent + "  "
                        avail := contentW - leadW
                        for _, line := range outlineDescriptionLinesMarkdown(row.item.Description, avail) {
                                items = append(items, outlineDescRowItem{
                                        parentID: row.item.ID,
                                        depth:    descDepth,
                                        line:     line,
                                })
                        }
                }
        }
        // Always-present affordance for adding an item (useful for empty outlines).
        items = append(items, addItemRow{})
        // If a filter is active, SetItems returns a Cmd that recomputes filtered matches.
        // refreshItems isn't part of the main update-return-cmd path, so we apply that Cmd
        // immediately to keep filtering responsive during refreshes.
        if cmd := m.itemsList.SetItems(items); cmd != nil {
                if msg := cmd(); msg != nil {
                        m.itemsList, _ = m.itemsList.Update(msg)
                }
        }
        // list.Model doesn't always clamp index when items shrink; clamp defensively to avoid
        // panics in navigation helpers that iterate relative to Index().
        if len(items) > 0 {
                idx := m.itemsList.Index()
                if idx < 0 {
                        m.itemsList.Select(0)
                } else if idx >= len(items) {
                        m.itemsList.Select(len(items) - 1)
                }
        }
        if strings.TrimSpace(curID) != "" {
                selectListItemByID(&m.itemsList, strings.TrimSpace(curID))
                return
        }
        // Default selection: first real item row, otherwise "+ Add item".
        for i := 0; i < len(items); i++ {
                if _, ok := items[i].(outlineRowItem); ok {
                        m.itemsList.Select(i)
                        return
                }
        }
        selectListItemByID(&m.itemsList, "__add__")
}

func (m *appModel) refreshAgenda() {
        if m == nil || m.db == nil {
                return
        }

        curID := ""
        if it, ok := m.agendaList.SelectedItem().(agendaRowItem); ok {
                curID = it.row.item.ID
        }

        // Sort projects by name for a stable agenda ordering.
        projects := make([]model.Project, 0, len(m.db.Projects))
        for _, p := range m.db.Projects {
                if p.Archived {
                        continue
                }
                projects = append(projects, p)
        }
        sort.Slice(projects, func(i, j int) bool {
                pi := strings.ToLower(strings.TrimSpace(projects[i].Name))
                pj := strings.ToLower(strings.TrimSpace(projects[j].Name))
                if pi == pj {
                        return projects[i].ID < projects[j].ID
                }
                if pi == "" {
                        return false
                }
                if pj == "" {
                        return true
                }
                return pi < pj
        })

        // Pre-group outlines by project.
        outlinesByProject := map[string][]model.Outline{}
        for _, o := range m.db.Outlines {
                if o.Archived {
                        continue
                }
                outlinesByProject[o.ProjectID] = append(outlinesByProject[o.ProjectID], o)
        }
        for pid := range outlinesByProject {
                outs := outlinesByProject[pid]
                sort.Slice(outs, func(i, j int) bool {
                        ni := ""
                        nj := ""
                        if outs[i].Name != nil {
                                ni = strings.ToLower(strings.TrimSpace(*outs[i].Name))
                        }
                        if outs[j].Name != nil {
                                nj = strings.ToLower(strings.TrimSpace(*outs[j].Name))
                        }
                        if ni == nj {
                                return outs[i].ID < outs[j].ID
                        }
                        if ni == "" {
                                return false
                        }
                        if nj == "" {
                                return true
                        }
                        return ni < nj
                })
                outlinesByProject[pid] = outs
        }

        var items []list.Item

        // Build agenda rows per outline using the existing outline flattener.
        for _, p := range projects {
                outs := outlinesByProject[p.ID]
                for _, o := range outs {
                        projectName := strings.TrimSpace(p.Name)
                        if projectName == "" {
                                projectName = p.ID
                        }
                        outName := ""
                        if o.Name != nil {
                                outName = strings.TrimSpace(*o.Name)
                        }

                        var its []model.Item
                        for _, it := range m.db.Items {
                                if it.Archived {
                                        continue
                                }
                                if it.OnHold {
                                        continue
                                }
                                if it.ProjectID != p.ID {
                                        continue
                                }
                                if it.OutlineID != o.ID {
                                        continue
                                }
                                if isEndState(o, it.StatusID) {
                                        continue
                                }
                                its = append(its, it)
                        }
                        if len(its) == 0 {
                                continue
                        }
                        items = append(items, agendaHeadingItem{projectName: projectName, outlineName: outName})
                        // Default agenda behavior: start parents collapsed so the agenda is lean/scannable.
                        // Only initialize collapse state for items we haven't seen before (so user toggles
                        // persist while the app is running).
                        childCount := map[string]int{}
                        for i := range its {
                                if its[i].ParentID != nil && strings.TrimSpace(*its[i].ParentID) != "" {
                                        childCount[strings.TrimSpace(*its[i].ParentID)]++
                                }
                        }
                        for parentID, n := range childCount {
                                if n <= 0 {
                                        continue
                                }
                                if _, ok := m.agendaCollapsed[parentID]; !ok {
                                        m.agendaCollapsed[parentID] = true
                                }
                        }
                        flat := flattenOutline(o, its, m.agendaCollapsed)
                        for _, row := range flat {
                                if row.item.AssignedActorID != nil && strings.TrimSpace(*row.item.AssignedActorID) != "" {
                                        row.assignedLabel = actorDisplayLabel(m.db, *row.item.AssignedActorID)
                                }
                                if len(row.item.Tags) > 0 {
                                        cleaned := make([]string, 0, len(row.item.Tags))
                                        for _, t := range row.item.Tags {
                                                t = normalizeTag(t)
                                                if t == "" {
                                                        continue
                                                }
                                                cleaned = append(cleaned, t)
                                        }
                                        row.item.Tags = uniqueSortedStrings(cleaned)
                                }
                                items = append(items, agendaRowItem{
                                        row:     row,
                                        outline: o,
                                })
                        }
                }
        }

        m.agendaList.SetItems(items)
        if curID != "" {
                selectListItemByID(&m.agendaList, curID)
        } else {
                // Prefer selecting the first actual item (skip headings).
                for i := 0; i < len(items); i++ {
                        if _, ok := items[i].(agendaRowItem); ok {
                                m.agendaList.Select(i)
                                break
                        }
                }
        }
}

func (m *appModel) refreshArchived() {
        if m == nil || m.db == nil {
                return
        }

        // Preserve selection (best-effort).
        curItemID := ""
        if it, ok := m.archivedList.SelectedItem().(archivedItemRowItem); ok {
                curItemID = strings.TrimSpace(it.itemID)
        }

        projectNameByID := map[string]string{}
        for _, p := range m.db.Projects {
                projectNameByID[p.ID] = strings.TrimSpace(p.Name)
        }
        outlineNameByID := map[string]string{}
        for _, o := range m.db.Outlines {
                outlineNameByID[o.ID] = outlineDisplayName(o)
        }

        // Archived projects.
        projects := make([]model.Project, 0, len(m.db.Projects))
        for _, p := range m.db.Projects {
                if p.Archived {
                        projects = append(projects, p)
                }
        }
        sort.Slice(projects, func(i, j int) bool {
                ni := strings.ToLower(strings.TrimSpace(projects[i].Name))
                nj := strings.ToLower(strings.TrimSpace(projects[j].Name))
                if ni == nj {
                        return projects[i].ID < projects[j].ID
                }
                if ni == "" {
                        return false
                }
                if nj == "" {
                        return true
                }
                return ni < nj
        })

        // Archived outlines.
        outlines := make([]model.Outline, 0, len(m.db.Outlines))
        for _, o := range m.db.Outlines {
                if o.Archived {
                        outlines = append(outlines, o)
                }
        }
        sort.Slice(outlines, func(i, j int) bool {
                pi := strings.ToLower(strings.TrimSpace(projectNameByID[outlines[i].ProjectID]))
                pj := strings.ToLower(strings.TrimSpace(projectNameByID[outlines[j].ProjectID]))
                if pi != pj {
                        if pi == "" {
                                return false
                        }
                        if pj == "" {
                                return true
                        }
                        return pi < pj
                }
                oi := strings.ToLower(strings.TrimSpace(outlineDisplayName(outlines[i])))
                oj := strings.ToLower(strings.TrimSpace(outlineDisplayName(outlines[j])))
                if oi == oj {
                        return outlines[i].ID < outlines[j].ID
                }
                if oi == "" {
                        return false
                }
                if oj == "" {
                        return true
                }
                return oi < oj
        })

        // Archived items.
        itemsOnly := make([]model.Item, 0, len(m.db.Items))
        for _, it := range m.db.Items {
                if it.Archived {
                        itemsOnly = append(itemsOnly, it)
                }
        }
        sort.Slice(itemsOnly, func(i, j int) bool {
                pi := strings.ToLower(strings.TrimSpace(projectNameByID[itemsOnly[i].ProjectID]))
                pj := strings.ToLower(strings.TrimSpace(projectNameByID[itemsOnly[j].ProjectID]))
                if pi != pj {
                        if pi == "" {
                                return false
                        }
                        if pj == "" {
                                return true
                        }
                        return pi < pj
                }
                oi := strings.ToLower(strings.TrimSpace(outlineNameByID[itemsOnly[i].OutlineID]))
                oj := strings.ToLower(strings.TrimSpace(outlineNameByID[itemsOnly[j].OutlineID]))
                if oi != oj {
                        if oi == "" {
                                return false
                        }
                        if oj == "" {
                                return true
                        }
                        return oi < oj
                }
                ti := strings.ToLower(strings.TrimSpace(itemsOnly[i].Title))
                tj := strings.ToLower(strings.TrimSpace(itemsOnly[j].Title))
                if ti == tj {
                        return itemsOnly[i].ID < itemsOnly[j].ID
                }
                if ti == "" {
                        return false
                }
                if tj == "" {
                        return true
                }
                return ti < tj
        })

        // Render list rows.
        rows := make([]list.Item, 0, 8+len(projects)+len(outlines)+len(itemsOnly))
        if len(projects) == 0 && len(outlines) == 0 && len(itemsOnly) == 0 {
                rows = append(rows, archivedHeadingItem{label: "No archived content"})
                m.archivedList.SetItems(rows)
                m.archivedList.Select(0)
                return
        }

        rows = append(rows, archivedHeadingItem{label: "Archived projects"})
        for _, p := range projects {
                rows = append(rows, archivedProjectItem{
                        projectName: p.Name,
                        projectID:   p.ID,
                })
        }

        rows = append(rows, archivedHeadingItem{label: "Archived outlines"})
        for _, o := range outlines {
                rows = append(rows, archivedOutlineItem{
                        projectName: projectNameByID[o.ProjectID],
                        outlineName: outlineDisplayName(o),
                        outlineID:   o.ID,
                })
        }

        rows = append(rows, archivedHeadingItem{label: "Archived items"})
        for _, it := range itemsOnly {
                rows = append(rows, archivedItemRowItem{
                        projectName: projectNameByID[it.ProjectID],
                        outlineName: outlineNameByID[it.OutlineID],
                        title:       it.Title,
                        itemID:      it.ID,
                })
        }

        m.archivedList.SetItems(rows)

        // Restore selection: prefer the previously-selected item; otherwise select the first real item row.
        if curItemID != "" {
                for i := 0; i < len(rows); i++ {
                        if r, ok := rows[i].(archivedItemRowItem); ok && strings.TrimSpace(r.itemID) == curItemID {
                                m.archivedList.Select(i)
                                return
                        }
                }
        }
        for i := 0; i < len(rows); i++ {
                if _, ok := rows[i].(archivedItemRowItem); ok {
                        m.archivedList.Select(i)
                        break
                }
        }
}

func (m *appModel) viewOutline() string {
        frameH, bodyHeight, contentW := m.outlineLayout()
        w := m.width
        if w < 10 {
                w = 10
        }

        // We want a stable header under the breadcrumb:
        // breadcrumb
        //
        // <outline title> (heading)
        // <outline description> (markdown; single-line in columns mode)
        outlineTitle := func(o model.Outline) string {
                if o.Name != nil {
                        if t := strings.TrimSpace(*o.Name); t != "" {
                                return t
                        }
                }
                return "Outline"
        }
        titleStyle := func(width int) lipgloss.Style {
                // Slightly more prominent than other text; keep it simple and readable.
                return lipgloss.NewStyle().Width(width).Bold(true)
        }
        lineCount := func(s string) int {
                s = strings.TrimRight(s, "\n")
                if s == "" {
                        return 0
                }
                return strings.Count(s, "\n") + 1
        }

        // Experimental: column/kanban view (status as columns).
        if m.curOutlineViewMode() == outlineViewModeColumns {
                crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
                outline, ok := m.db.FindOutline(m.selectedOutlineID)
                if !ok {
                        msg := lipgloss.NewStyle().Width(contentW).Height(bodyHeight).Render("Outline not found.")
                        main := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + msg
                        main = lipgloss.NewStyle().Width(w).Padding(0, splitOuterMargin).Render(main)
                        return main
                }

                // Header: breadcrumb + title + (optional) single-line description.
                header := crumb + "\n\n" + titleStyle(contentW).Render(truncateText(outlineTitle(*outline), contentW))
                desc := strings.TrimSpace(outline.Description)
                extraLines := 2 // blank line + title
                if desc != "" {
                        one := ""
                        if parts := strings.Split(desc, "\n"); len(parts) > 0 {
                                one = strings.TrimSpace(parts[0])
                        }
                        one = strings.Join(strings.Fields(one), " ")
                        if one != "" {
                                one = truncateText(one, contentW)
                                line := lipgloss.NewStyle().Width(contentW).Foreground(colorMuted).Render(one)
                                header = header + "\n" + line
                                extraLines++
                        }
                }

                its := make([]model.Item, 0, 64)
                for _, it := range m.db.Items {
                        if it.Archived {
                                continue
                        }
                        if it.OutlineID != outline.ID {
                                continue
                        }
                        its = append(its, it)
                }
                boardH := bodyHeight - extraLines
                if boardH < 3 {
                        boardH = 3
                }
                boardData := buildOutlineColumnsBoard(m.db, *outline, its)
                sel := m.columnsSel[strings.TrimSpace(outline.ID)]
                sel = boardData.clamp(sel)
                m.columnsSel[strings.TrimSpace(outline.ID)] = sel
                // Keep list selection aligned so switching back to list mode preserves focus.
                if it, ok := boardData.selectedItem(sel); ok {
                        selectListItemByID(&m.itemsList, strings.TrimSpace(it.Item.ID))
                }
                board := renderOutlineColumns(*outline, boardData, sel, contentW, boardH)
                main := strings.Repeat("\n", topPadLines) + header + "\n" + board
                main = lipgloss.NewStyle().Width(w).Padding(0, splitOuterMargin).Render(main)
                if m.modal == modalNone {
                        return main
                }
                bg := dimBackground(main)
                fg := m.renderModal()
                return overlayCenter(bg, fg, w, frameH)
        }

        var main string
        if !m.splitPreviewVisible() {
                crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
                outline, ok := m.db.FindOutline(m.selectedOutlineID)
                if !ok {
                        msg := lipgloss.NewStyle().Width(contentW).Height(bodyHeight).Render("Outline not found.")
                        main = strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + msg
                } else {
                        titleLine := titleStyle(contentW).Render(truncateText(outlineTitle(*outline), contentW))
                        header := crumb + "\n\n" + titleLine
                        descMD := strings.TrimSpace(outline.Description)
                        descRendered := ""
                        if descMD != "" {
                                descRendered = renderMarkdownNoMargin(descMD, contentW)
                                descRendered = strings.TrimLeft(descRendered, "\n")
                                header = header + "\n" + descRendered
                        }
                        headerLines := lineCount(header)
                        availH := (frameH - topPadLines) - headerLines - 1 // 1 line gap between header and body
                        bodyH := availH
                        if bodyH < 3 {
                                bodyH = 3
                        }
                        body := m.listBodyWithOverflowHint(&m.itemsList, contentW, bodyH)
                        main = strings.Repeat("\n", topPadLines) + header + "\n" + body
                }
        } else {
                // Split view: render the breadcrumb only over the LEFT pane, so the detail pane
                // can start at the top without wasted header padding.
                leftW, rightW := splitPaneWidths(contentW)
                outline, ok := m.db.FindOutline(m.selectedOutlineID)
                titleLine := titleStyle(leftW).Render(truncateText("Outline", leftW))
                descRendered := ""
                if ok {
                        titleLine = titleStyle(leftW).Render(truncateText(outlineTitle(*outline), leftW))
                        descMD := strings.TrimSpace(outline.Description)
                        if descMD != "" {
                                descRendered = renderMarkdownNoMargin(descMD, leftW)
                                descRendered = strings.TrimLeft(descRendered, "\n")
                        }
                }
                // Render the breadcrumb at full content width so it doesn't wrap early on narrow left panes.
                // The right pane overlays on top, so any breadcrumb overflow into the right side is naturally hidden.
                fullCrumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
                leftHeaderTmp := fullCrumb + "\n\n" + titleLine
                if strings.TrimSpace(descRendered) != "" {
                        leftHeaderTmp = leftHeaderTmp + "\n" + descRendered
                }
                headerLines := lineCount(leftHeaderTmp)
                leftBodyH := (frameH - topPadLines) - headerLines - 1 // 1 line gap
                if leftBodyH < 3 {
                        leftBodyH = 3
                }
                // Render the left list at full width (the right pane will overlay it).
                leftBody := m.listBodyWithOverflowHint(&m.itemsList, contentW, leftBodyH)

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

                leftHeader := fullCrumb + "\n\n" + titleLine
                if strings.TrimSpace(descRendered) != "" {
                        leftHeader = leftHeader + "\n" + descRendered
                }
                main = renderSplitWithLeftHeaderGap(contentW, frameH, leftW, rightW, leftHeader, 1, leftBody, right)
        }
        // Outline content should be left-aligned with a small outer margin (same feel as split view).
        main = lipgloss.NewStyle().Width(w).Padding(0, splitOuterMargin).Render(main)

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
        frameH := m.frameHeight()
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

        contentW := w - 2*splitOuterMargin
        if contentW < 10 {
                contentW = w
        }

        wrap := func(content string) string {
                main := lipgloss.NewStyle().Width(w).Padding(0, splitOuterMargin).Render(content)
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

        itemID := strings.TrimSpace(m.openItemID)
        if itemID == "" {
                crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
                msg := lipgloss.NewStyle().Width(contentW).Height(bodyHeight).Render("No item selected.")
                block := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + msg
                return wrap(block)
        }

        outline, ok := m.db.FindOutline(m.selectedOutlineID)
        if !ok {
                crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
                msg := lipgloss.NewStyle().Width(contentW).Height(bodyHeight).Render("Outline not found.")
                block := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + msg
                return wrap(block)
        }

        it, ok := m.db.FindItem(itemID)
        if !ok {
                crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
                msg := lipgloss.NewStyle().Width(contentW).Height(bodyHeight).Render("Item not found.")
                block := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + msg
                return wrap(block)
        }

        crumb := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())

        // Split view when focusing a section that has a side panel.
        if sidePanelKindForFocus(m.itemFocus) != itemSideNone {
                leftW, rightW := splitPaneWidths(contentW)
                // Render the left pane at full width (the right pane will overlay it).
                left := renderItemDetailInteractive(m.db, *outline, *it, contentW, bodyHeight, m.itemFocus, m.eventsTail, m.itemChildIdx, m.itemChildOff, m.itemDetailScroll)
                right := renderItemSidePanelWithEvents(m.db, *it, rightW, bodyHeight, sidePanelKindForFocus(m.itemFocus), m.itemCommentIdx, m.itemWorklogIdx, m.itemHistoryIdx, m.itemSideScroll, m.eventsTail)
                // Same breadcrumb strategy as outline split preview: render at full width so wrapping is "behind" the overlay.
                leftHeader := lipgloss.NewStyle().Width(contentW).Foreground(lipgloss.Color("243")).Render(m.breadcrumbText())
                main := renderSplitWithLeftHeader(contentW, frameH, leftW, rightW, leftHeader, left, right)
                // Item view uses the standard (full-width) breadcrumb in non-split mode; in split mode
                // the right pane overlays on top, so the breadcrumb effectively sits "under" it.
                return wrap(main)
        }

        card := renderItemDetailInteractive(m.db, *outline, *it, contentW, bodyHeight, m.itemFocus, m.eventsTail, m.itemChildIdx, m.itemChildOff, m.itemDetailScroll)
        block := strings.Repeat("\n", topPadLines) + crumb + strings.Repeat("\n", breadcrumbGap+1) + card
        return wrap(block)
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
                return m.renderInputModal(title)
        case modalNewProject:
                return m.renderInputModal("New project")
        case modalRenameProject:
                return m.renderInputModal("Rename project")
        case modalNewOutline:
                return m.renderInputModal("New outline")
        case modalEditTitle:
                return m.renderInputModal("Edit title")
        case modalEditDescription:
                return m.renderTextAreaModal("Edit description")
        case modalStatusNote:
                return m.renderStatusNoteModal()
        case modalEditOutlineName:
                return m.renderInputModal("Rename outline")
        case modalEditOutlineDescription:
                return m.renderTextAreaModal("Edit outline description")
        case modalSetDue:
                return m.renderDateTimeModal("Due date")
        case modalSetSchedule:
                return m.renderDateTimeModal("Schedule")
        case modalPickStatus:
                return renderModalBox(m.width, "Set status", m.statusList.View()+"\n\nenter: set   esc/ctrl+g: cancel")
        case modalPickOutline:
                return renderModalBox(m.width, "Move to outline", m.outlinePickList.View()+"\n\nenter: move   esc/ctrl+g: cancel")
        case modalPickAssignee:
                return renderModalBox(m.width, "Assign", m.assigneeList.View()+"\n\nenter: set   esc/ctrl+g: cancel")
        case modalEditTags:
                return m.renderTagsModal()
        case modalPickWorkspace:
                return renderModalBox(m.width, "Workspaces", m.workspaceList.View()+"\n\nenter: switch   n:new   r:rename   esc/ctrl+g: close")
        case modalCaptureTemplates:
                return m.renderCaptureTemplatesModal()
        case modalCaptureTemplateName:
                return m.renderInputModalWithDescription("Capture template: name", "Name shown in pickers (e.g. \"Work inbox\").")
        case modalCaptureTemplateKeys:
                return m.renderInputModalWithDescription("Capture template: keys", "Enter a multi-key sequence (e.g. \"w i\" or \"wi\"). Each key is one character.")
        case modalCaptureTemplatePickWorkspace:
                return renderModalBox(m.width, "Capture template: workspace", "Pick the target workspace for this template.\n\n"+m.captureTemplateWorkspaceList.View()+"\n\nenter: select   esc/ctrl+g: cancel")
        case modalCaptureTemplatePickOutline:
                return renderModalBox(m.width, "Capture template: outline", "Pick the target outline (archived outlines are hidden).\n\n"+m.captureTemplateOutlineList.View()+"\n\nenter: select   esc/ctrl+g: cancel")
        case modalConfirmDeleteCaptureTemplate:
                return m.renderConfirmDeleteCaptureTemplateModal()
        case modalGitSetupRemote:
                return m.renderInputModalWithDescription("Git setup", "Enter a Git remote URL (e.g. GitHub). Leave blank to only initialize a local repo.")
        case modalNewWorkspace:
                return m.renderInputModal("New workspace")
        case modalRenameWorkspace:
                return m.renderInputModal("Rename workspace")
        case modalEditOutlineStatuses:
                return renderModalBox(m.width, "Outline statuses", m.outlineStatusDefsList.View()+"\n\na:add  r:rename  e:toggle end  n:toggle note  d:delete  ctrl+k/j:move  esc/ctrl+g: close")
        case modalAddOutlineStatus:
                return m.renderInputModal("Add status")
        case modalRenameOutlineStatus:
                return m.renderInputModal("Rename status")
        case modalJumpToItem:
                return m.renderInputModal("Jump to item")
        case modalAddComment:
                return m.renderTextAreaModal("Add comment")
        case modalReplyComment:
                return m.renderReplyCommentModal()
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
                return renderModalBox(m.width, "Confirm", body+"\n\nenter/y: archive   esc/n: cancel   ctrl+g: close")
        default:
                return ""
        }
}

func (m *appModel) renderDateTimeModal(title string) string {
        bodyW := modalBodyWidth(m.width)

        focusBtn := func(active bool) lipgloss.Style {
                if active {
                        return lipgloss.NewStyle().Padding(0, 1).Foreground(colorSelectedFg).Background(colorSelectedBg).Bold(true)
                }
                return lipgloss.NewStyle().Padding(0, 1).Foreground(colorSurfaceFg).Background(colorControlBg)
        }

        renderPill := func(active bool, content string) string {
                // Avoid Width()+Padding() wrapping which can render as two lines in some terminals.
                st := lipgloss.NewStyle().Background(colorInputBg).Foreground(colorSurfaceFg)
                if active {
                        st = lipgloss.NewStyle().Foreground(colorSelectedFg).Background(colorSelectedBg).Bold(true)
                }
                return st.Render(" " + content + " ")
        }

        // Use the raw input values for rendering (avoid nested background styling in textinput.View()).
        y := renderPill(m.dateFocus == dateFocusYear, padLeft(strings.TrimSpace(m.yearInput.Value()), 4))
        mo := renderPill(m.dateFocus == dateFocusMonth, padLeft(strings.TrimSpace(m.monthInput.Value()), 2))
        da := renderPill(m.dateFocus == dateFocusDay, padLeft(strings.TrimSpace(m.dayInput.Value()), 2))

        timeToggle := "[ ] time"
        if m.timeEnabled {
                timeToggle = "[x] time"
        }
        toggleLine := renderPill(m.dateFocus == dateFocusTimeToggle, timeToggle)

        hh := renderPill(m.dateFocus == dateFocusHour, padLeft(strings.TrimSpace(m.hourInput.Value()), 2))
        min := renderPill(m.dateFocus == dateFocusMinute, padLeft(strings.TrimSpace(m.minuteInput.Value()), 2))

        save := focusBtn(m.dateFocus == dateFocusSave).Render("Save")
        clear := focusBtn(m.dateFocus == dateFocusClear).Render("Clear")
        cancel := focusBtn(m.dateFocus == dateFocusCancel).Render("Cancel")

        help := styleMuted().Width(bodyW).Render("tab: focus  h/l: prev/next  j/k or ↓/↑: -/+  enter/ctrl+s: save  ctrl+c: clear  esc/ctrl+g: cancel")

        timeLine := toggleLine
        if m.timeEnabled {
                timeLine = toggleLine + "  " + lipgloss.JoinHorizontal(lipgloss.Left, hh, ":", min)
        }

        body := strings.Join([]string{
                styleMuted().Width(bodyW).Render("Date"),
                lipgloss.JoinHorizontal(lipgloss.Left, y, "-", mo, "-", da),
                "",
                styleMuted().Width(bodyW).Render("Time (optional)"),
                timeLine,
                "",
                lipgloss.JoinHorizontal(lipgloss.Left, save, clear, cancel),
                "",
                help,
        }, "\n")

        return renderModalBox(m.width, title, body)
}

func padLeft(s string, w int) string {
        for xansi.StringWidth(s) < w {
                s = "0" + s
        }
        if xansi.StringWidth(s) > w {
                return xansi.Cut(s, xansi.StringWidth(s)-w, xansi.StringWidth(s))
        }
        return s
}

func (m *appModel) renderTextAreaModal(title string) string {
        // Avoid borders here: some terminals show background artifacts when nesting bordered
        // components inside a modal with a background color.
        btnBase := lipgloss.NewStyle().
                Padding(0, 1).
                Foreground(colorSurfaceFg).
                Background(colorControlBg)
        btnActive := btnBase.
                Foreground(colorSelectedFg).
                Background(colorSelectedBg).
                Bold(true)

        save := btnBase.Render("Save")
        cancel := btnBase.Render("Cancel")
        if m.textFocus == textFocusSave {
                save = btnActive.Render("Save")
        }
        if m.textFocus == textFocusCancel {
                cancel = btnActive.Render("Cancel")
        }

        sep := lipgloss.NewStyle().Background(colorControlBg).Render(" ")
        controls := lipgloss.JoinHorizontal(lipgloss.Top, save, sep, cancel)
        body := strings.Join([]string{
                m.textarea.View(),
                "",
                controls,
                "",
                "ctrl+o: editor    ctrl+s: save    tab: focus    esc/ctrl+g: cancel",
        }, "\n")
        return renderModalBox(m.width, title, body)
}

func (m *appModel) renderStatusNoteModal() string {
        itemID := strings.TrimSpace(m.modalForID)
        statusID := strings.TrimSpace(m.modalForKey)
        statusLbl := statusID
        if m.db != nil && itemID != "" {
                if it, ok := m.db.FindItem(itemID); ok && it != nil {
                        if o, ok := m.db.FindOutline(strings.TrimSpace(it.OutlineID)); ok && o != nil {
                                lbl := strings.TrimSpace(statusLabel(*o, statusID))
                                if lbl != "" {
                                        statusLbl = lbl
                                }
                        }
                }
        }
        statusLbl = strings.TrimSpace(statusLbl)
        if statusLbl == "" {
                statusLbl = "(no status)"
        }

        header := styleMuted().Render("Add note setting item to " + strings.ToUpper(statusLbl))

        // Avoid borders here: some terminals show background artifacts when nesting bordered
        // components inside a modal with a background color.
        btnBase := lipgloss.NewStyle().
                Padding(0, 1).
                Foreground(colorSurfaceFg).
                Background(colorControlBg)
        btnActive := btnBase.
                Foreground(colorSelectedFg).
                Background(colorSelectedBg).
                Bold(true)

        save := btnBase.Render("Save")
        cancel := btnBase.Render("Cancel")
        if m.textFocus == textFocusSave {
                save = btnActive.Render("Save")
        }
        if m.textFocus == textFocusCancel {
                cancel = btnActive.Render("Cancel")
        }

        sep := lipgloss.NewStyle().Background(colorControlBg).Render(" ")
        controls := lipgloss.JoinHorizontal(lipgloss.Top, save, sep, cancel)
        body := strings.Join([]string{
                header,
                "",
                m.textarea.View(),
                "",
                controls,
                "",
                "ctrl+o: editor    ctrl+s: save    tab: focus    esc/ctrl+g: cancel",
        }, "\n")
        return renderModalBox(m.width, "Status note", body)
}

func (m *appModel) renderReplyCommentModal() string {
        // Avoid borders here: some terminals show background artifacts when nesting bordered
        // components inside a modal with a background color.
        btnBase := lipgloss.NewStyle().
                Padding(0, 1).
                Foreground(colorSurfaceFg).
                Background(colorControlBg)
        btnActive := btnBase.
                Foreground(colorSelectedFg).
                Background(colorSelectedBg).
                Bold(true)

        save := btnBase.Render("Save")
        cancel := btnBase.Render("Cancel")
        if m.textFocus == textFocusSave {
                save = btnActive.Render("Save")
        }
        if m.textFocus == textFocusCancel {
                cancel = btnActive.Render("Cancel")
        }

        sep := lipgloss.NewStyle().Background(colorControlBg).Render(" ")
        controls := lipgloss.JoinHorizontal(lipgloss.Top, save, sep, cancel)

        quoteMD := strings.TrimSpace(m.replyQuoteMD)
        if quoteMD == "" {
                quoteMD = "> (original comment missing)"
        }
        // Render markdown for consistent wrapping, but keep it compact.
        // Use the textarea width as the effective modal body width.
        bodyW := m.textarea.Width()
        if bodyW < 10 {
                bodyW = 10
        }
        quoteRendered := truncateLines(strings.TrimSpace(renderMarkdown(quoteMD, bodyW)), 8)
        quoteRendered = faintIfDark(lipgloss.NewStyle()).Render(quoteRendered)

        body := strings.Join([]string{
                quoteRendered,
                "",
                m.textarea.View(),
                "",
                controls,
                "",
                "ctrl+o: editor   tab: focus   ctrl+s: save   esc/ctrl+g: cancel",
        }, "\n")
        return renderModalBox(m.width, "Reply", body)
}

func (m *appModel) renderInputModal(title string) string {
        btnBase := lipgloss.NewStyle().
                Padding(0, 1).
                Foreground(colorSurfaceFg).
                Background(colorControlBg)
        btnActive := btnBase.
                Foreground(colorSelectedFg).
                Background(colorSelectedBg).
                Bold(true)

        save := btnBase.Render("Save")
        cancel := btnBase.Render("Cancel")
        if m.textFocus == textFocusSave {
                save = btnActive.Render("Save")
        }
        if m.textFocus == textFocusCancel {
                cancel = btnActive.Render("Cancel")
        }

        sep := lipgloss.NewStyle().Background(colorControlBg).Render(" ")
        controls := lipgloss.JoinHorizontal(lipgloss.Top, save, sep, cancel)

        // Keep the input visually distinct from the modal surface, and match the full modal width.
        //
        // Important: terminal background colors can "bleed" across newlines if not reset.
        // Using PlaceHorizontal with a whitespace background keeps this to a single line
        // and avoids the field looking taller than intended.
        bodyW := modalBodyWidth(m.width)
        inputW := bodyW - 2 // one space padding on each side
        if inputW < 10 {
                inputW = 10
        }
        m.input.Width = inputW
        inputLine := lipgloss.PlaceHorizontal(
                bodyW,
                lipgloss.Left,
                " "+m.input.View()+" ",
                lipgloss.WithWhitespaceChars(" "),
                lipgloss.WithWhitespaceBackground(colorInputBg),
        )
        body := strings.Join([]string{
                inputLine,
                "",
                controls,
                "",
                "ctrl+s: save    tab: focus    esc: cancel    ctrl+g: close",
        }, "\n")
        return renderModalBox(m.width, title, body)
}

func (m *appModel) renderInputModalWithDescription(title, desc string) string {
        btnBase := lipgloss.NewStyle().
                Padding(0, 1).
                Foreground(colorSurfaceFg).
                Background(colorControlBg)
        btnActive := btnBase.
                Foreground(colorSelectedFg).
                Background(colorSelectedBg).
                Bold(true)

        save := btnBase.Render("Save")
        cancel := btnBase.Render("Cancel")
        if m.textFocus == textFocusSave {
                save = btnActive.Render("Save")
        }
        if m.textFocus == textFocusCancel {
                cancel = btnActive.Render("Cancel")
        }

        sep := lipgloss.NewStyle().Background(colorControlBg).Render(" ")
        controls := lipgloss.JoinHorizontal(lipgloss.Top, save, sep, cancel)

        bodyW := modalBodyWidth(m.width)
        inputW := bodyW - 2 // one space padding on each side
        if inputW < 10 {
                inputW = 10
        }
        m.input.Width = inputW
        inputLine := lipgloss.PlaceHorizontal(
                bodyW,
                lipgloss.Left,
                " "+m.input.View()+" ",
                lipgloss.WithWhitespaceChars(" "),
                lipgloss.WithWhitespaceBackground(colorInputBg),
        )
        descLine := styleMuted().Width(bodyW).Render(strings.TrimSpace(desc))
        body := strings.Join([]string{
                descLine,
                "",
                inputLine,
                "",
                controls,
                "",
                "ctrl+s: save    tab: focus    esc: cancel    ctrl+g: close",
        }, "\n")
        return renderModalBox(m.width, title, body)
}

func (m *appModel) renderTagsModal() string {
        bodyW := modalBodyWidth(m.width)
        inputW := bodyW - 2 // one space padding on each side
        if inputW < 10 {
                inputW = 10
        }
        m.input.Width = inputW

        inputLine := lipgloss.PlaceHorizontal(
                bodyW,
                lipgloss.Left,
                " "+m.input.View()+" ",
                lipgloss.WithWhitespaceChars(" "),
                lipgloss.WithWhitespaceBackground(colorInputBg),
        )
        help := styleMuted().Width(bodyW).Render("tab: focus  enter (input): add  enter/space (list): toggle  esc/ctrl+g: close")
        body := strings.Join([]string{
                inputLine,
                "",
                m.tagsList.View(),
                "",
                help,
        }, "\n")
        return renderModalBox(m.width, "Tags", body)
}

func tickReload() tea.Cmd {
        return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg { return reloadTickMsg{} })
}

func (m *appModel) captureStoreModTimes() {
        dbMt, evMt := m.storeModTimes()
        m.lastDBModTime = dbMt
        m.lastEventsModTime = evMt
}

func (m *appModel) storeChanged() bool {
        dbMt, evMt := m.storeModTimes()
        return dbMt.After(m.lastDBModTime) || evMt.After(m.lastEventsModTime)
}

func (m *appModel) storeModTimes() (dbMt time.Time, eventsMt time.Time) {
        // Derived index (preferred).
        dbMt = fileModTime(filepath.Join(m.dir, ".clarity", "index.sqlite"))
        // Legacy sqlite.
        if dbMt.IsZero() {
                dbMt = fileModTime(filepath.Join(m.dir, ".clarity", "clarity.sqlite"))
        }
        if dbMt.IsZero() {
                dbMt = fileModTime(filepath.Join(m.dir, "clarity.sqlite"))
        }

        // Canonical events dir (Git-backed) or legacy monolith.
        eventsMt = fileModTime(filepath.Join(m.dir, "events"))
        if eventsMt.IsZero() {
                eventsMt = fileModTime(filepath.Join(m.dir, "events.jsonl"))
        }
        return dbMt, eventsMt
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
        m.refreshEventsTail()

        // Refresh current view (and keep selection if possible).
        switch m.view {
        case viewProjects:
                m.refreshProjects()
        case viewOutlines:
                m.refreshOutlines(m.selectedProjectID)
        case viewAgenda:
                m.refreshAgenda()
        case viewArchived:
                m.refreshArchived()
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
                case workspaceItem:
                        if it.name == id {
                                l.Select(i)
                                return
                        }
                case outlineRowItem:
                        if it.row.item.ID == id {
                                l.Select(i)
                                return
                        }
                case agendaRowItem:
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
        if m.modal == modalCapture {
                if m.capture == nil {
                        (&m).closeAllModals()
                        return m, nil
                }
                mmAny, cmd := m.capture.Update(msg)
                if mm, ok := mmAny.(captureModel); ok {
                        *m.capture = mm
                }
                return m, cmd
        }

        // Modal input takes over all keys.
        if m.modal != modalNone {
                // Ctrl+G should always close any modal (Esc may mean "back" in some flows).
                if km, ok := msg.(tea.KeyMsg); ok && km.String() == "ctrl+g" {
                        (&m).closeAllModals()
                        return m, nil
                }

                if m.modal == modalActionPanel {
                        if km, ok := msg.(tea.KeyMsg); ok {
                                switch km.String() {
                                case "ctrl+g":
                                        (&m).closeActionPanel()
                                        return m, nil
                                case "esc", "backspace":
                                        // Special-case capture: backspace edits the key sequence, not panel navigation.
                                        if m.curActionPanelKind() == actionPanelCapture && km.String() == "backspace" {
                                                if len(m.captureKeySeq) > 0 {
                                                        m.captureKeySeq = m.captureKeySeq[:len(m.captureKeySeq)-1]
                                                        m.actionPanelSelectedKey = ""
                                                        m.ensureActionPanelSelection()
                                                        return m, nil
                                                }
                                        }
                                        (&m).popActionPanel()
                                        return m, nil
                                case "tab":
                                        (&m).moveActionPanelSelection(+1)
                                        return m, nil
                                case "shift+tab", "backtab":
                                        (&m).moveActionPanelSelection(-1)
                                        return m, nil
                                case "up", "k", "ctrl+p":
                                        (&m).moveActionPanelSelection(-1)
                                        return m, nil
                                case "down", "j", "ctrl+n":
                                        (&m).moveActionPanelSelection(+1)
                                        return m, nil
                                }

                                actions := m.actionPanelActions()
                                // Capture panel: org-capture style typed selection.
                                if m.curActionPanelKind() == actionPanelCapture {
                                        if km.String() == "ctrl+t" {
                                                (&m).closeActionPanel()
                                                (&m).openCaptureTemplatesModal()
                                                return m, nil
                                        }

                                        cfg, err := store.LoadConfig()
                                        if err != nil {
                                                m.showMinibuffer("Capture templates: " + err.Error())
                                                return m, nil
                                        }
                                        if err := store.ValidateCaptureTemplates(cfg); err != nil {
                                                m.showMinibuffer("Capture templates: " + err.Error())
                                                return m, nil
                                        }
                                        exact, next := captureTemplatesAtPrefix(cfg.CaptureTemplates, m.captureKeySeq)

                                        chooseKey := func(k string) (appModel, bool) {
                                                k = strings.TrimSpace(k)
                                                if k == "" {
                                                        return m, false
                                                }
                                                if _, ok := next[k]; !ok {
                                                        return m, false
                                                }
                                                m.captureKeySeq = append(m.captureKeySeq, k)
                                                m.actionPanelSelectedKey = ""
                                                m.ensureActionPanelSelection()

                                                ex2, next2 := captureTemplatesAtPrefix(cfg.CaptureTemplates, m.captureKeySeq)
                                                if ex2 != nil && len(next2) == 0 {
                                                        (&m).closeActionPanel()
                                                        m2, cmd := startCaptureItemFromTemplate(m, *ex2)
                                                        _ = cmd
                                                        return m2, true
                                                }
                                                return m, true
                                        }

                                        switch km.String() {
                                        case "enter":
                                                if exact != nil {
                                                        (&m).closeActionPanel()
                                                        m2, cmd := startCaptureItemFromTemplate(m, *exact)
                                                        _ = cmd
                                                        return m2, nil
                                                }
                                                if k := strings.TrimSpace(m.actionPanelSelectedKey); k != "" {
                                                        if m2, ok := chooseKey(k); ok {
                                                                return m2, nil
                                                        }
                                                }
                                                return m, nil
                                        }
                                        if km.Type == tea.KeyRunes && len(km.Runes) == 1 {
                                                if m2, ok := chooseKey(string(km.Runes[0])); ok {
                                                        return m2, nil
                                                }
                                        }
                                        // Fall through to normal key handling (e.g. ctrl+g/backspace handled above).
                                }
                                // If the current panel has no explicit "enter" binding, allow Enter to execute
                                // the currently selected action key.
                                if km.String() == "enter" {
                                        if _, ok := actions["enter"]; !ok {
                                                k := strings.TrimSpace(m.actionPanelSelectedKey)
                                                if k != "" {
                                                        if a, ok := actions[k]; ok {
                                                                (&m).closeActionPanel()
                                                                if a.handler != nil {
                                                                        m2, cmd := a.handler(m)
                                                                        return m2, cmd
                                                                }
                                                                return m, nil
                                                        }
                                                }
                                                return m, nil
                                        }
                                }
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
                                                if a.next == actionPanelCapture {
                                                        m.captureKeySeq = nil
                                                }
                                                m.actionPanelSelectedKey = ""
                                                m.ensureActionPanelSelection()
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

                if m.modal == modalEditOutlineStatuses {
                        if km, ok := msg.(tea.KeyMsg); ok {
                                switch km.String() {
                                case "esc":
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        m.modalForKey = ""
                                        return m, nil
                                case "a":
                                        m.modalForKey = ""
                                        m.openInputModal(modalAddOutlineStatus, strings.TrimSpace(m.modalForID), "Status label", "")
                                        return m, nil
                                case "r":
                                        if it, ok := m.outlineStatusDefsList.SelectedItem().(outlineStatusDefItem); ok {
                                                m.modalForKey = strings.TrimSpace(it.def.ID)
                                                m.openInputModal(modalRenameOutlineStatus, strings.TrimSpace(m.modalForID), "Status label", strings.TrimSpace(it.def.Label))
                                                return m, nil
                                        }
                                case "e":
                                        if it, ok := m.outlineStatusDefsList.SelectedItem().(outlineStatusDefItem); ok {
                                                oid := strings.TrimSpace(m.modalForID)
                                                if err := m.toggleOutlineStatusEndState(oid, strings.TrimSpace(it.def.ID)); err != nil {
                                                        m.showMinibuffer("Update failed: " + err.Error())
                                                        return m, nil
                                                }
                                                m.refreshOutlineStatusDefsEditor(oid, strings.TrimSpace(it.def.ID))
                                                return m, nil
                                        }
                                case "n":
                                        if it, ok := m.outlineStatusDefsList.SelectedItem().(outlineStatusDefItem); ok {
                                                oid := strings.TrimSpace(m.modalForID)
                                                if err := m.toggleOutlineStatusRequiresNote(oid, strings.TrimSpace(it.def.ID)); err != nil {
                                                        m.showMinibuffer("Update failed: " + err.Error())
                                                        return m, nil
                                                }
                                                m.refreshOutlineStatusDefsEditor(oid, strings.TrimSpace(it.def.ID))
                                                return m, nil
                                        }
                                case "d":
                                        if it, ok := m.outlineStatusDefsList.SelectedItem().(outlineStatusDefItem); ok {
                                                oid := strings.TrimSpace(m.modalForID)
                                                if err := m.removeOutlineStatusDef(oid, strings.TrimSpace(it.def.ID)); err != nil {
                                                        m.showMinibuffer("Remove failed: " + err.Error())
                                                        return m, nil
                                                }
                                                m.refreshOutlineStatusDefsEditor(oid, "")
                                                return m, nil
                                        }
                                case "ctrl+k":
                                        if it, ok := m.outlineStatusDefsList.SelectedItem().(outlineStatusDefItem); ok {
                                                oid := strings.TrimSpace(m.modalForID)
                                                if err := m.moveOutlineStatusDef(oid, strings.TrimSpace(it.def.ID), -1); err != nil {
                                                        m.showMinibuffer("Reorder failed: " + err.Error())
                                                        return m, nil
                                                }
                                                m.refreshOutlineStatusDefsEditor(oid, strings.TrimSpace(it.def.ID))
                                                return m, nil
                                        }
                                case "ctrl+j":
                                        if it, ok := m.outlineStatusDefsList.SelectedItem().(outlineStatusDefItem); ok {
                                                oid := strings.TrimSpace(m.modalForID)
                                                if err := m.moveOutlineStatusDef(oid, strings.TrimSpace(it.def.ID), +1); err != nil {
                                                        m.showMinibuffer("Reorder failed: " + err.Error())
                                                        return m, nil
                                                }
                                                m.refreshOutlineStatusDefsEditor(oid, strings.TrimSpace(it.def.ID))
                                                return m, nil
                                        }
                                }
                        }
                        var cmd tea.Cmd
                        m.outlineStatusDefsList, cmd = m.outlineStatusDefsList.Update(msg)
                        return m, cmd
                }

                if m.modal == modalSetDue || m.modal == modalSetSchedule {
                        switch km := msg.(type) {
                        case tea.KeyMsg:
                                switch km.String() {
                                case "esc", "ctrl+g":
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        m.yearInput.Blur()
                                        m.monthInput.Blur()
                                        m.dayInput.Blur()
                                        m.hourInput.Blur()
                                        m.minuteInput.Blur()
                                        m.dateFocus = dateFocusDay
                                        return m, nil
                                case "tab":
                                        switch m.dateFocus {
                                        case dateFocusYear:
                                                m.dateFocus = dateFocusMonth
                                        case dateFocusMonth:
                                                m.dateFocus = dateFocusDay
                                        case dateFocusDay:
                                                m.dateFocus = dateFocusTimeToggle
                                        case dateFocusTimeToggle:
                                                if m.timeEnabled {
                                                        m.dateFocus = dateFocusHour
                                                } else {
                                                        m.dateFocus = dateFocusSave
                                                }
                                        case dateFocusHour:
                                                m.dateFocus = dateFocusMinute
                                        case dateFocusMinute:
                                                m.dateFocus = dateFocusSave
                                        case dateFocusSave:
                                                m.dateFocus = dateFocusClear
                                        case dateFocusClear:
                                                m.dateFocus = dateFocusCancel
                                        default:
                                                m.dateFocus = dateFocusYear
                                        }
                                case "shift+tab", "backtab":
                                        switch m.dateFocus {
                                        case dateFocusYear:
                                                m.dateFocus = dateFocusCancel
                                        case dateFocusCancel:
                                                m.dateFocus = dateFocusClear
                                        case dateFocusClear:
                                                m.dateFocus = dateFocusSave
                                        case dateFocusSave:
                                                if m.timeEnabled {
                                                        m.dateFocus = dateFocusMinute
                                                } else {
                                                        m.dateFocus = dateFocusTimeToggle
                                                }
                                        case dateFocusMinute:
                                                m.dateFocus = dateFocusHour
                                        case dateFocusHour:
                                                m.dateFocus = dateFocusTimeToggle
                                        case dateFocusTimeToggle:
                                                m.dateFocus = dateFocusDay
                                        case dateFocusDay:
                                                m.dateFocus = dateFocusMonth
                                        case dateFocusMonth:
                                                m.dateFocus = dateFocusYear
                                        default:
                                                m.dateFocus = dateFocusYear
                                        }
                                case "ctrl+c":
                                        // Clear (safe: avoid single-letter clears).
                                        itemID := strings.TrimSpace(m.modalForID)
                                        if m.modal == modalSetDue {
                                                _ = m.setDue(itemID, nil)
                                        } else {
                                                _ = m.setSchedule(itemID, nil)
                                        }
                                        (&m).closeAllModals()
                                        return m, nil
                                case "left", "h":
                                        switch m.dateFocus {
                                        case dateFocusYear:
                                                // wrap to last field (depends on time enabled)
                                                if m.timeEnabled {
                                                        m.dateFocus = dateFocusMinute
                                                } else {
                                                        m.dateFocus = dateFocusTimeToggle
                                                }
                                                return m, nil
                                        case dateFocusMonth:
                                                m.dateFocus = dateFocusYear
                                                return m, nil
                                        case dateFocusDay:
                                                m.dateFocus = dateFocusMonth
                                                return m, nil
                                        case dateFocusTimeToggle:
                                                m.dateFocus = dateFocusDay
                                                return m, nil
                                        case dateFocusHour:
                                                m.dateFocus = dateFocusDay
                                                return m, nil
                                        case dateFocusMinute:
                                                m.dateFocus = dateFocusHour
                                                return m, nil
                                        }
                                case "right", "l":
                                        switch m.dateFocus {
                                        case dateFocusYear:
                                                m.dateFocus = dateFocusMonth
                                                return m, nil
                                        case dateFocusMonth:
                                                m.dateFocus = dateFocusDay
                                                return m, nil
                                        case dateFocusDay:
                                                m.dateFocus = dateFocusTimeToggle
                                                return m, nil
                                        case dateFocusTimeToggle:
                                                if m.timeEnabled {
                                                        m.dateFocus = dateFocusHour
                                                } else {
                                                        m.dateFocus = dateFocusSave
                                                }
                                                return m, nil
                                        case dateFocusHour:
                                                m.dateFocus = dateFocusMinute
                                                return m, nil
                                        case dateFocusMinute:
                                                m.dateFocus = dateFocusYear
                                                return m, nil
                                        }
                                case "up", "k", "ctrl+p":
                                        if m.dateFocus == dateFocusTimeToggle {
                                                // Toggle time on/off.
                                                m.timeEnabled = !m.timeEnabled
                                                if !m.timeEnabled {
                                                        m.hourInput.SetValue("")
                                                        m.minuteInput.SetValue("")
                                                } else {
                                                        // Seed a sensible time when enabling.
                                                        if strings.TrimSpace(m.hourInput.Value()) == "" {
                                                                m.hourInput.SetValue("09")
                                                        }
                                                        if strings.TrimSpace(m.minuteInput.Value()) == "" {
                                                                m.minuteInput.SetValue("00")
                                                        }
                                                }
                                                return m, nil
                                        }
                                        if m.bumpDateTimeField(+1) {
                                                return m, nil
                                        }
                                case "down", "j", "ctrl+n":
                                        if m.dateFocus == dateFocusTimeToggle {
                                                m.timeEnabled = !m.timeEnabled
                                                if !m.timeEnabled {
                                                        m.hourInput.SetValue("")
                                                        m.minuteInput.SetValue("")
                                                } else {
                                                        if strings.TrimSpace(m.hourInput.Value()) == "" {
                                                                m.hourInput.SetValue("09")
                                                        }
                                                        if strings.TrimSpace(m.minuteInput.Value()) == "" {
                                                                m.minuteInput.SetValue("00")
                                                        }
                                                }
                                                return m, nil
                                        }
                                        if m.bumpDateTimeField(-1) {
                                                return m, nil
                                        }
                                case " ", "t":
                                        if m.dateFocus == dateFocusTimeToggle {
                                                m.timeEnabled = !m.timeEnabled
                                                if !m.timeEnabled {
                                                        m.hourInput.SetValue("")
                                                        m.minuteInput.SetValue("")
                                                } else {
                                                        if strings.TrimSpace(m.hourInput.Value()) == "" {
                                                                m.hourInput.SetValue("09")
                                                        }
                                                        if strings.TrimSpace(m.minuteInput.Value()) == "" {
                                                                m.minuteInput.SetValue("00")
                                                        }
                                                }
                                                return m, nil
                                        }
                                case "enter":
                                        if m.dateFocus == dateFocusTimeToggle {
                                                m.timeEnabled = !m.timeEnabled
                                                if !m.timeEnabled {
                                                        m.hourInput.SetValue("")
                                                        m.minuteInput.SetValue("")
                                                } else {
                                                        if strings.TrimSpace(m.hourInput.Value()) == "" {
                                                                m.hourInput.SetValue("09")
                                                        }
                                                        if strings.TrimSpace(m.minuteInput.Value()) == "" {
                                                                m.minuteInput.SetValue("00")
                                                        }
                                                }
                                                return m, nil
                                        }
                                        fallthrough
                                case "ctrl+s":
                                        itemID := strings.TrimSpace(m.modalForID)
                                        // If focused on cancel, treat enter as cancel.
                                        if m.dateFocus == dateFocusCancel {
                                                (&m).closeAllModals()
                                                return m, nil
                                        }
                                        // If focused on clear, clear.
                                        if m.dateFocus == dateFocusClear {
                                                if m.modal == modalSetDue {
                                                        _ = m.setDue(itemID, nil)
                                                } else {
                                                        _ = m.setSchedule(itemID, nil)
                                                }
                                                (&m).closeAllModals()
                                                return m, nil
                                        }
                                        // If time is disabled, ignore time fields.
                                        hv := m.hourInput.Value()
                                        mv := m.minuteInput.Value()
                                        if !m.timeEnabled {
                                                hv = ""
                                                mv = ""
                                        }
                                        dt, err := parseDateTimeInputsFields(m.yearInput.Value(), m.monthInput.Value(), m.dayInput.Value(), hv, mv)
                                        if err != nil {
                                                m.showMinibuffer(err.Error())
                                                return m, nil
                                        }
                                        if m.modal == modalSetDue {
                                                if err := m.setDue(itemID, dt); err != nil {
                                                        return m, m.reportError(itemID, err)
                                                }
                                        } else {
                                                if err := m.setSchedule(itemID, dt); err != nil {
                                                        return m, m.reportError(itemID, err)
                                                }
                                        }
                                        (&m).closeAllModals()
                                        return m, nil
                                }

                                // Apply focus to inputs.
                                m.applyDateFieldFocus()

                                // Update inputs if focused.
                                var cmd tea.Cmd
                                switch m.dateFocus {
                                case dateFocusYear:
                                        m.yearInput, cmd = m.yearInput.Update(msg)
                                        return m, cmd
                                case dateFocusMonth:
                                        m.monthInput, cmd = m.monthInput.Update(msg)
                                        return m, cmd
                                case dateFocusDay:
                                        m.dayInput, cmd = m.dayInput.Update(msg)
                                        return m, cmd
                                case dateFocusHour:
                                        m.hourInput, cmd = m.hourInput.Update(msg)
                                        return m, cmd
                                case dateFocusMinute:
                                        m.minuteInput, cmd = m.minuteInput.Update(msg)
                                        return m, cmd
                                }
                                return m, nil
                        }
                        return m, nil
                }

                if m.modal == modalAddOutlineStatus || m.modal == modalRenameOutlineStatus {
                        switch km := msg.(type) {
                        case tea.KeyMsg:
                                switch km.String() {
                                case "esc":
                                        m.modal = modalEditOutlineStatuses
                                        m.modalForKey = ""
                                        m.input.Placeholder = "Title"
                                        m.input.SetValue("")
                                        m.input.Blur()
                                        return m, nil
                                case "enter", "ctrl+s":
                                        val := strings.TrimSpace(m.input.Value())
                                        if val == "" {
                                                return m, nil
                                        }
                                        oid := strings.TrimSpace(m.modalForID)
                                        switch m.modal {
                                        case modalAddOutlineStatus:
                                                id, err := m.addOutlineStatusDef(oid, val, false)
                                                if err != nil {
                                                        m.showMinibuffer("Add failed: " + err.Error())
                                                        return m, nil
                                                }
                                                m.refreshOutlineStatusDefsEditor(oid, id)
                                        case modalRenameOutlineStatus:
                                                sid := strings.TrimSpace(m.modalForKey)
                                                if sid == "" {
                                                        return m, nil
                                                }
                                                if err := m.renameOutlineStatusDef(oid, sid, val); err != nil {
                                                        m.showMinibuffer("Rename failed: " + err.Error())
                                                        return m, nil
                                                }
                                                m.refreshOutlineStatusDefsEditor(oid, sid)
                                        }
                                        m.modal = modalEditOutlineStatuses
                                        m.modalForKey = ""
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

                if m.modal == modalAddComment || m.modal == modalReplyComment || m.modal == modalAddWorklog || m.modal == modalEditDescription || m.modal == modalEditOutlineDescription || m.modal == modalStatusNote {
                        switch km := msg.(type) {
                        case tea.KeyMsg:
                                switch km.String() {
                                case "esc", "ctrl+g":
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        m.modalForKey = ""
                                        m.replyQuoteMD = ""
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
                                                itemID := strings.TrimSpace(m.modalForID)
                                                if m.modal == modalEditDescription {
                                                        if err := m.setDescriptionFromModal(body); err != nil {
                                                                return m, m.reportError(itemID, err)
                                                        }
                                                } else if m.modal == modalStatusNote {
                                                        statusID := strings.TrimSpace(m.modalForKey)
                                                        note := body // allow empty
                                                        if err := (&m).setStatusForItemWithNote(itemID, statusID, &note); err != nil {
                                                                return m, m.reportError(itemID, err)
                                                        }
                                                } else {
                                                        if body == "" {
                                                                return m, nil
                                                        }
                                                        if m.modal == modalAddComment {
                                                                _, _ = m.addComment(itemID, body, nil)
                                                        } else if m.modal == modalReplyComment {
                                                                replyTo := strings.TrimSpace(m.modalForKey)
                                                                _, _ = m.addComment(itemID, body, &replyTo)
                                                        } else {
                                                                _ = m.addWorklog(itemID, body)
                                                        }
                                                }
                                                m.modal = modalNone
                                                m.modalForID = ""
                                                m.modalForKey = ""
                                                m.replyQuoteMD = ""
                                                m.textarea.SetValue("")
                                                m.textarea.Blur()
                                                m.textFocus = textFocusBody
                                                return m, nil
                                        }
                                        if m.textFocus == textFocusCancel {
                                                m.modal = modalNone
                                                m.modalForID = ""
                                                m.modalForKey = ""
                                                m.replyQuoteMD = ""
                                                m.textarea.Blur()
                                                m.textFocus = textFocusBody
                                                return m, nil
                                        }
                                        // else: newline in textarea
                                case "ctrl+s":
                                        body := strings.TrimSpace(m.textarea.Value())
                                        itemID := strings.TrimSpace(m.modalForID)
                                        if m.modal == modalEditDescription {
                                                if err := m.setDescriptionFromModal(body); err != nil {
                                                        return m, m.reportError(itemID, err)
                                                }
                                        } else if m.modal == modalStatusNote {
                                                statusID := strings.TrimSpace(m.modalForKey)
                                                note := body // allow empty
                                                if err := (&m).setStatusForItemWithNote(itemID, statusID, &note); err != nil {
                                                        return m, m.reportError(itemID, err)
                                                }
                                        } else if m.modal == modalEditOutlineDescription {
                                                _ = m.setOutlineDescriptionFromModal(body)
                                        } else {
                                                if body == "" {
                                                        return m, nil
                                                }
                                                if m.modal == modalAddComment {
                                                        _, _ = m.addComment(itemID, body, nil)
                                                } else if m.modal == modalReplyComment {
                                                        replyTo := strings.TrimSpace(m.modalForKey)
                                                        _, _ = m.addComment(itemID, body, &replyTo)
                                                } else {
                                                        _ = m.addWorklog(itemID, body)
                                                }
                                        }
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        m.modalForKey = ""
                                        m.replyQuoteMD = ""
                                        m.textarea.SetValue("")
                                        m.textarea.Blur()
                                        m.textFocus = textFocusBody
                                        return m, nil
                                case "ctrl+o":
                                        // Open the current textarea content in $VISUAL/$EDITOR.
                                        // Keep the modal open; ctrl+s/Save still commits changes to the store.
                                        if m.textFocus != textFocusBody {
                                                return m, nil
                                        }
                                        m.textarea.Blur()
                                        cmd, err := m.openExternalEditorForTextarea()
                                        if err != nil {
                                                m.textarea.Focus()
                                                m.showMinibuffer("Editor open failed: " + err.Error())
                                                return m, nil
                                        }
                                        return m, cmd
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

                if m.modal == modalPickOutline {
                        switch km := msg.(type) {
                        case tea.KeyMsg:
                                switch km.String() {
                                case "esc":
                                        m.pendingMoveOutlineTo = ""
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        return m, nil
                                case "enter":
                                        itemID := strings.TrimSpace(m.modalForID)
                                        to := ""
                                        if it, ok := m.outlinePickList.SelectedItem().(outlineItem); ok {
                                                to = strings.TrimSpace(it.outline.ID)
                                        }

                                        // Close the outline picker; we may reopen a status picker.
                                        m.modal = modalNone
                                        m.modalForID = ""

                                        if itemID == "" || to == "" || m.db == nil {
                                                return m, nil
                                        }
                                        curItem, ok := m.db.FindItem(itemID)
                                        if !ok {
                                                return m, nil
                                        }
                                        if strings.TrimSpace(curItem.OutlineID) == strings.TrimSpace(to) {
                                                m.showMinibuffer("Already in that outline")
                                                return m, nil
                                        }
                                        o, ok := m.db.FindOutline(to)
                                        if !ok {
                                                m.showMinibuffer("Error: outline not found")
                                                return m, nil
                                        }

                                        // If any status in the subtree isn't valid in the target outline, prompt for one.
                                        if subtreeHasInvalidStatusInOutline(m.db, curItem.ID, o.ID) {
                                                m.pendingMoveOutlineTo = o.ID
                                                // No "(no status)" option: moved items must have a valid status in the target outline.
                                                m.openStatusPickerForOutline(*o, curItem.StatusID, false)
                                                m.modal = modalPickStatus
                                                m.modalForID = itemID
                                                return m, nil
                                        }

                                        if err := m.moveItemToOutline(itemID, o.ID, "", false); err != nil {
                                                return m, m.reportError(itemID, err)
                                        }
                                        return m, nil
                                }
                        }
                        var cmd tea.Cmd
                        m.outlinePickList, cmd = m.outlinePickList.Update(msg)
                        return m, cmd
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
                                                if strings.TrimSpace(m.pendingMoveOutlineTo) != "" {
                                                        to := strings.TrimSpace(m.pendingMoveOutlineTo)
                                                        m.pendingMoveOutlineTo = ""
                                                        if err := m.moveItemToOutline(itemID, to, it.id, true); err != nil {
                                                                return m, m.reportError(itemID, err)
                                                        }
                                                        m.modal = modalNone
                                                        m.modalForID = ""
                                                        return m, nil
                                                }

                                                if m.db != nil {
                                                        if cur, ok := m.db.FindItem(itemID); ok && cur != nil {
                                                                var outline model.Outline
                                                                if o, ok := m.db.FindOutline(cur.OutlineID); ok && o != nil {
                                                                        outline = *o
                                                                }
                                                                if statusutil.IsEndState(outline, it.id) {
                                                                        if reason := explainCompletionBlockers(m.db, cur.ID); strings.TrimSpace(reason) != "" {
                                                                                return m, m.reportError(itemID, completionBlockedError{taskID: cur.ID, reason: reason})
                                                                        }
                                                                }
                                                                if statusutil.RequiresNote(outline, it.id) {
                                                                        m.openTextModal(modalStatusNote, itemID, "Status note…", "")
                                                                        m.modalForKey = strings.TrimSpace(it.id)
                                                                        return m, nil
                                                                }
                                                        }
                                                }

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

                if m.modal == modalPickAssignee {
                        switch km := msg.(type) {
                        case tea.KeyMsg:
                                switch km.String() {
                                case "esc":
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        return m, nil
                                case "enter":
                                        itemID := strings.TrimSpace(m.modalForID)
                                        pick, _ := m.assigneeList.SelectedItem().(assigneeOptionItem)
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        if itemID == "" {
                                                return m, nil
                                        }
                                        if strings.TrimSpace(pick.id) == "" {
                                                if err := m.setAssignedActor(itemID, nil); err != nil {
                                                        return m, m.reportError(itemID, err)
                                                }
                                                return m, nil
                                        }
                                        tmp := strings.TrimSpace(pick.id)
                                        if err := m.setAssignedActor(itemID, &tmp); err != nil {
                                                return m, m.reportError(itemID, err)
                                        }
                                        return m, nil
                                }
                        }
                        var cmd tea.Cmd
                        m.assigneeList, cmd = m.assigneeList.Update(msg)
                        return m, cmd
                }

                if m.modal == modalEditTags {
                        itemID := strings.TrimSpace(m.modalForID)
                        switch km := msg.(type) {
                        case tea.KeyMsg:
                                switch km.String() {
                                case "esc":
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        m.tagsFocus = tagsFocusInput
                                        if m.tagsListActive != nil {
                                                *m.tagsListActive = false
                                        }
                                        m.input.Placeholder = "Title"
                                        m.input.SetValue("")
                                        m.input.Blur()
                                        return m, nil
                                case "tab":
                                        if m.tagsFocus == tagsFocusInput {
                                                m.tagsFocus = tagsFocusList
                                                m.input.Blur()
                                                if m.tagsListActive != nil {
                                                        *m.tagsListActive = true
                                                }
                                        } else {
                                                m.tagsFocus = tagsFocusInput
                                                m.input.Focus()
                                                if m.tagsListActive != nil {
                                                        *m.tagsListActive = false
                                                }
                                        }
                                        return m, nil
                                case "shift+tab", "backtab":
                                        if m.tagsFocus == tagsFocusList {
                                                m.tagsFocus = tagsFocusInput
                                                m.input.Focus()
                                                if m.tagsListActive != nil {
                                                        *m.tagsListActive = false
                                                }
                                        } else {
                                                m.tagsFocus = tagsFocusList
                                                m.input.Blur()
                                                if m.tagsListActive != nil {
                                                        *m.tagsListActive = true
                                                }
                                        }
                                        return m, nil
                                }
                                if m.tagsFocus == tagsFocusInput {
                                        switch km.String() {
                                        case "enter":
                                                tag := normalizeTag(m.input.Value())
                                                if tag == "" || itemID == "" {
                                                        return m, nil
                                                }
                                                if err := m.setTagChecked(itemID, tag, true); err != nil {
                                                        return m, m.reportError(itemID, err)
                                                }
                                                m.input.SetValue("")
                                                m.refreshTagsEditor(itemID, tag)
                                                return m, nil
                                        case "down", "j", "ctrl+n":
                                                if len(m.tagsList.Items()) > 0 {
                                                        m.tagsFocus = tagsFocusList
                                                        m.input.Blur()
                                                        if m.tagsListActive != nil {
                                                                *m.tagsListActive = true
                                                        }
                                                        return m, nil
                                                }
                                        }
                                } else {
                                        switch km.String() {
                                        case "up", "k", "ctrl+p":
                                                if m.tagsList.Index() <= 0 {
                                                        m.tagsFocus = tagsFocusInput
                                                        m.input.Focus()
                                                        if m.tagsListActive != nil {
                                                                *m.tagsListActive = false
                                                        }
                                                        return m, nil
                                                }
                                        }
                                        switch km.String() {
                                        case "enter", " ":
                                                pick, ok := m.tagsList.SelectedItem().(tagOptionItem)
                                                if !ok {
                                                        return m, nil
                                                }
                                                tag := strings.TrimSpace(pick.tag)
                                                if tag == "" || itemID == "" {
                                                        return m, nil
                                                }
                                                if err := m.setTagChecked(itemID, tag, !pick.checked); err != nil {
                                                        return m, m.reportError(itemID, err)
                                                }
                                                m.refreshTagsEditor(itemID, tag)
                                                return m, nil
                                        }
                                }
                        }
                        if m.tagsFocus == tagsFocusInput {
                                var cmd tea.Cmd
                                m.input, cmd = m.input.Update(msg)
                                return m, cmd
                        }
                        var cmd tea.Cmd
                        m.tagsList, cmd = m.tagsList.Update(msg)
                        return m, cmd
                }

                if m.modal == modalPickWorkspace {
                        switch km := msg.(type) {
                        case tea.KeyMsg:
                                switch km.String() {
                                case "esc":
                                        m.modal = modalNone
                                        m.modalForKey = ""
                                        return m, nil
                                case "enter":
                                        name := ""
                                        if it, ok := m.workspaceList.SelectedItem().(workspaceItem); ok {
                                                name = strings.TrimSpace(it.name)
                                        }
                                        if name == "" {
                                                return m, nil
                                        }
                                        nm, err := m.switchWorkspaceTo(name)
                                        if err != nil {
                                                m.showMinibuffer("Workspace error: " + err.Error())
                                                return m, nil
                                        }
                                        (&nm).showMinibuffer("Workspace: " + name)
                                        return nm, nil
                                case "n":
                                        m.modalForKey = ""
                                        m.openInputModal(modalNewWorkspace, "", "Workspace name", "")
                                        return m, nil
                                case "r":
                                        old := strings.TrimSpace(m.workspace)
                                        if old == "" {
                                                if cfg, err := store.LoadConfig(); err == nil {
                                                        old = strings.TrimSpace(cfg.CurrentWorkspace)
                                                }
                                        }
                                        if old == "" {
                                                m.showMinibuffer("Workspace: no current workspace")
                                                return m, nil
                                        }
                                        m.modalForKey = old
                                        m.openInputModal(modalRenameWorkspace, "", "New workspace name", old)
                                        return m, nil
                                }
                        }
                        var cmd tea.Cmd
                        m.workspaceList, cmd = m.workspaceList.Update(msg)
                        return m, cmd
                }

                if m.modal == modalCaptureTemplates {
                        return m.updateCaptureTemplatesModal(msg)
                }

                if m.modal == modalConfirmDeleteCaptureTemplate {
                        if km, ok := msg.(tea.KeyMsg); ok {
                                switch km.String() {
                                case "esc", "n":
                                        m.modal = modalCaptureTemplates
                                        m.captureTemplateDeleteIdx = -1
                                        m.modalForID = ""
                                        m.modalForKey = ""
                                        return m, nil
                                case "enter", "y":
                                        if err := m.confirmDeleteCaptureTemplate(); err != nil {
                                                m.showMinibuffer("Delete failed: " + err.Error())
                                        } else {
                                                m.showMinibuffer("Template deleted")
                                        }
                                        m.modal = modalCaptureTemplates
                                        m.modalForID = ""
                                        m.modalForKey = ""
                                        m.refreshCaptureTemplatesList("")
                                        return m, nil
                                }
                        }
                        return m, nil
                }

                if m.modal == modalCaptureTemplatePickWorkspace {
                        switch km := msg.(type) {
                        case tea.KeyMsg:
                                switch km.String() {
                                case "esc":
                                        m.modal = modalCaptureTemplates
                                        return m, nil
                                case "enter":
                                        if m.captureTemplateEdit == nil {
                                                m.modal = modalCaptureTemplates
                                                return m, nil
                                        }
                                        ws := ""
                                        if it, ok := m.captureTemplateWorkspaceList.SelectedItem().(captureTemplateWorkspaceItem); ok {
                                                ws = strings.TrimSpace(it.name)
                                        }
                                        if ws == "" {
                                                return m, nil
                                        }
                                        m.captureTemplateEdit.tmpl.Target.Workspace = ws
                                        m.captureTemplateEdit.stage = captureTemplateEditOutline
                                        m.openCaptureTemplateOutlinePicker(ws, m.captureTemplateEdit.tmpl.Target.OutlineID)
                                        return m, nil
                                }
                        }
                        var cmd tea.Cmd
                        m.captureTemplateWorkspaceList, cmd = m.captureTemplateWorkspaceList.Update(msg)
                        return m, cmd
                }

                if m.modal == modalCaptureTemplatePickOutline {
                        switch km := msg.(type) {
                        case tea.KeyMsg:
                                switch km.String() {
                                case "esc":
                                        m.modal = modalCaptureTemplates
                                        return m, nil
                                case "enter":
                                        if m.captureTemplateEdit == nil {
                                                m.modal = modalCaptureTemplates
                                                return m, nil
                                        }
                                        oid := ""
                                        if it, ok := m.captureTemplateOutlineList.SelectedItem().(captureTemplateOutlineItem); ok {
                                                oid = strings.TrimSpace(it.outline.ID)
                                        }
                                        if oid == "" {
                                                return m, nil
                                        }
                                        m.captureTemplateEdit.tmpl.Target.OutlineID = oid
                                        if err := m.saveCaptureTemplateEdit(); err != nil {
                                                m.showMinibuffer("Save failed: " + err.Error())
                                                m.modal = modalCaptureTemplates
                                                return m, nil
                                        }
                                        keys := strings.Join(m.captureTemplateEdit.tmpl.Keys, "")
                                        m.captureTemplateEdit = nil
                                        m.showMinibuffer("Template saved")
                                        m.modal = modalCaptureTemplates
                                        m.refreshCaptureTemplatesList(keys)
                                        return m, nil
                                }
                        }
                        var cmd tea.Cmd
                        m.captureTemplateOutlineList, cmd = m.captureTemplateOutlineList.Update(msg)
                        return m, cmd
                }

                switch km := msg.(type) {
                case tea.KeyMsg:
                        switch km.String() {
                        case "esc", "ctrl+g":
                                if m.modal == modalCaptureTemplateName || m.modal == modalCaptureTemplateKeys {
                                        m.modal = modalCaptureTemplates
                                        m.captureTemplateEdit = nil
                                        m.modalForID = ""
                                        m.modalForKey = ""
                                        m.textFocus = textFocusBody
                                        m.input.Placeholder = "Title"
                                        m.input.SetValue("")
                                        m.input.Blur()
                                        return m, nil
                                }
                                m.modal = modalNone
                                m.modalForID = ""
                                m.modalForKey = ""
                                m.textFocus = textFocusBody
                                m.input.Placeholder = "Title"
                                m.input.SetValue("")
                                m.input.Blur()
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
                                        m.input.Focus()
                                } else {
                                        m.input.Blur()
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
                                        m.input.Focus()
                                } else {
                                        m.input.Blur()
                                }
                                return m, nil
                        case "enter":
                                if m.textFocus == textFocusCancel {
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        m.modalForKey = ""
                                        m.textFocus = textFocusBody
                                        m.input.Placeholder = "Title"
                                        m.input.SetValue("")
                                        m.input.Blur()
                                        return m, nil
                                }
                                fallthrough
                        case "ctrl+s":
                                val := strings.TrimSpace(m.input.Value())
                                switch m.modal {
                                case modalGitSetupRemote:
                                        remoteURL := strings.TrimSpace(val)
                                        m.modal = modalNone
                                        m.modalForID = ""
                                        m.modalForKey = ""
                                        m.textFocus = textFocusBody
                                        m.input.Placeholder = "Title"
                                        m.input.SetValue("")
                                        m.input.Blur()
                                        return m, (&m).syncSetupCmd(remoteURL)
                                case modalCaptureTemplateName:
                                        if m.captureTemplateEdit == nil {
                                                m.modal = modalCaptureTemplates
                                                return m, nil
                                        }
                                        if val == "" {
                                                return m, nil
                                        }
                                        m.captureTemplateEdit.tmpl.Name = val
                                        m.captureTemplateEdit.stage = captureTemplateEditKeys
                                        m.openInputModal(modalCaptureTemplateKeys, "", "Keys (e.g. w i or wi)", strings.Join(m.captureTemplateEdit.tmpl.Keys, " "))
                                        return m, nil
                                case modalCaptureTemplateKeys:
                                        if m.captureTemplateEdit == nil {
                                                m.modal = modalCaptureTemplates
                                                return m, nil
                                        }
                                        keys, err := parseCaptureKeysInput(val)
                                        if err != nil {
                                                m.showMinibuffer("Keys: " + err.Error())
                                                return m, nil
                                        }
                                        m.captureTemplateEdit.tmpl.Keys = keys
                                        m.captureTemplateEdit.stage = captureTemplateEditWorkspace
                                        m.openCaptureTemplateWorkspacePicker(m.captureTemplateEdit.tmpl.Target.Workspace)
                                        return m, nil
                                case modalJumpToItem:
                                        val = normalizeJumpItemID(val)
                                        if val == "" {
                                                return m, nil
                                        }
                                        if err := (&m).jumpToItemByID(val); err != nil {
                                                m.showMinibuffer("Jump: " + err.Error())
                                                return m, nil
                                        }
                                case modalNewProject:
                                        if val == "" {
                                                return m, nil
                                        }
                                        if err := m.createProjectFromModal(val); err != nil {
                                                m.showMinibuffer("Error: " + err.Error())
                                                return m, nil
                                        }
                                case modalRenameProject:
                                        if val == "" {
                                                return m, nil
                                        }
                                        if err := m.renameProjectFromModal(val); err != nil {
                                                m.showMinibuffer("Error: " + err.Error())
                                                return m, nil
                                        }
                                case modalNewOutline:
                                        // Name optional.
                                        if err := m.createOutlineFromModal(val); err != nil {
                                                m.showMinibuffer("Error: " + err.Error())
                                                return m, nil
                                        }
                                case modalNewWorkspace:
                                        if val == "" {
                                                return m, nil
                                        }
                                        nm, err := m.switchWorkspaceTo(val)
                                        if err != nil {
                                                m.showMinibuffer("Workspace error: " + err.Error())
                                                return m, nil
                                        }
                                        (&nm).showMinibuffer("Workspace: " + strings.TrimSpace(val))
                                        return nm, nil
                                case modalRenameWorkspace:
                                        old := strings.TrimSpace(m.modalForKey)
                                        if old == "" {
                                                old = strings.TrimSpace(m.workspace)
                                        }
                                        if old == "" {
                                                m.showMinibuffer("Workspace: no current workspace")
                                                return m, nil
                                        }
                                        if val == "" {
                                                return m, nil
                                        }
                                        nm, err := m.renameWorkspaceTo(old, val)
                                        if err != nil {
                                                m.showMinibuffer("Workspace error: " + err.Error())
                                                return m, nil
                                        }
                                        (&nm).showMinibuffer("Workspace: " + strings.TrimSpace(val))
                                        return nm, nil
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
                                m.modalForKey = ""
                                m.input.Placeholder = "Title"
                                m.input.SetValue("")
                                m.input.Blur()
                                m.textFocus = textFocusBody
                                return m, nil
                        }
                }
                var cmd tea.Cmd
                if m.textFocus == textFocusBody {
                        m.input, cmd = m.input.Update(msg)
                        return m, cmd
                }
                return m, nil
        }

        switch msg := msg.(type) {
        case tea.KeyMsg:
                if m.curOutlineViewMode() == outlineViewModeColumns && m.modal == modalNone {
                        if handled, cmd := (&m).updateOutlineColumns(msg); handled {
                                return m, cmd
                        }
                }

                // Outline key: open Outline… submenu in the action panel.
                if m.modal == modalNone && msg.String() == "O" {
                        m.openActionPanel(actionPanelContext)
                        (&m).pushActionPanel(actionPanelOutline)
                        return m, nil
                }

                // Enter outline filtering mode (Bubble list default is "/"), and do it early so it's not
                // impacted by any other outline key handling.
                if msg.String() == "/" && m.modal == modalNone {
                        before := m.itemsList.SettingFilter()
                        var cmd tea.Cmd
                        m.itemsList, cmd = m.itemsList.Update(msg)
                        // Give a tiny hint so it's obvious the app is now capturing keystrokes for filtering.
                        if !before && m.itemsList.SettingFilter() {
                                m.showMinibuffer("Filter: type to search titles (fuzzy). enter: apply  esc: cancel")
                        }
                        return m, cmd
                }

                // When the user is editing the filter input, the list should own keystrokes.
                // Otherwise keys like Enter/j/k would be interpreted as "open item"/navigation.
                if m.itemsList.SettingFilter() {
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

                if msg.String() == "f12" && m.debugEnabled {
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
                                txt := m.clipboardItemRef(it.row.item.ID)
                                if err := copyToClipboard(txt); err != nil {
                                        m.showMinibuffer("Clipboard error: " + err.Error())
                                } else {
                                        m.showMinibuffer("Copied: " + txt)
                                }
                                return m, nil
                        }
                case "Y":
                        // Copy a helpful CLI command for the selected item.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                cmd := m.clipboardShowCmd(it.row.item.ID)
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
                                m.openTextModal(modalAddComment, it.row.item.ID, "Write a comment…", "")
                                return m, nil
                        }
                case "w":
                        // Add worklog entry to selected item.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                m.openTextModal(modalAddWorklog, it.row.item.ID, "Log work…", "")
                                return m, nil
                        }
                case "p":
                        // Toggle priority for selected item.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                if err := m.togglePriority(it.row.item.ID); err != nil {
                                        return m, m.reportError(it.row.item.ID, err)
                                }
                                return m, nil
                        }
                case "o":
                        // Toggle on-hold for selected item.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                if err := m.toggleOnHold(it.row.item.ID); err != nil {
                                        return m, m.reportError(it.row.item.ID, err)
                                }
                                return m, nil
                        }
                case "a":
                        // Assign selected item.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                m.openAssigneePicker(it.row.item.ID)
                                return m, nil
                        }
                case "t":
                        // Edit tags for selected item.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                m.openTagsEditor(it.row.item.ID)
                                return m, nil
                        }
                case "d":
                        // Set due for selected item.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                m.openDateModal(modalSetDue, it.row.item.ID, it.row.item.Due)
                                return m, nil
                        }
                case "s":
                        // Set schedule for selected item.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                m.openDateModal(modalSetSchedule, it.row.item.ID, it.row.item.Schedule)
                                return m, nil
                        }
                case "D":
                        // Edit description for selected item (multiline/markdown).
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                m.openTextModal(modalEditDescription, it.row.item.ID, "Markdown description…", it.row.item.Description)
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
                case "m":
                        // Move selected item to another outline (outline pane only).
                        if m.pane == paneOutline {
                                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                        m.openMoveOutlinePicker(it.row.item.ID)
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
                                        m.itemArchivedReadOnly = false
                                        (&m).recordRecentItemVisit(m.openItemID)
                                        m.itemFocus = itemFocusTitle
                                        m.itemCommentIdx = 0
                                        m.itemWorklogIdx = 0
                                        m.itemHistoryIdx = 0
                                        m.itemSideScroll = 0
                                        // Leaving preview mode when entering the full item page.
                                        m.showPreview = false
                                        m.previewCacheForID = ""
                                        return m, nil
                                }
                                return m, nil
                        case addItemRow:
                                m.openInputModal(modalNewSibling, "", "Title", "")
                                return m, nil
                        }
                case "v":
                        // Cycle outline view modes (list -> list+preview -> columns).
                        m.cycleOutlineViewMode()
                        if m.selectedOutline != nil {
                                m.refreshItems(*m.selectedOutline)
                        }
                        // Preview may have become visible; compute it.
                        if m.splitPreviewVisible() {
                                return m, m.schedulePreviewCompute()
                        }
                        return m, nil
                case "S":
                        // Edit outline status definitions.
                        oid := strings.TrimSpace(m.selectedOutlineID)
                        if oid == "" {
                                m.showMinibuffer("No outline selected")
                                return m, nil
                        }
                        if o, ok := m.db.FindOutline(oid); ok && o != nil {
                                m.openOutlineStatusDefsEditor(*o, "")
                                return m, nil
                        }
                        m.showMinibuffer("Outline not found")
                        return m, nil
                case "e":
                        // Edit title for selected item.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                m.openInputModal(modalEditTitle, it.row.item.ID, "Title", it.row.item.Title)
                                return m, nil
                        }
                case "n":
                        // New sibling (after selected) in outline pane.
                        if m.pane == paneOutline {
                                forID := ""
                                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                        forID = it.row.item.ID
                                }
                                m.openInputModal(modalNewSibling, forID, "Title", "")
                                return m, nil
                        }
                case "N":
                        // New child (under selected) in either pane. If "+ Add item" selected, fall back to root.
                        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                                m.openInputModal(modalNewChild, it.row.item.ID, "Title", "")
                        } else {
                                m.openInputModal(modalNewSibling, "", "Title", "")
                        }
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
                        if m.splitPreviewVisible() {
                                return m, m.schedulePreviewCompute()
                        }
                        return m, nil
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
        switch it := m.itemsList.SelectedItem().(type) {
        case outlineRowItem:
                beforeID = strings.TrimSpace(it.row.item.ID)
        }
        var cmd tea.Cmd
        m.itemsList, cmd = m.itemsList.Update(msg)
        afterID := ""
        switch it := m.itemsList.SelectedItem().(type) {
        case outlineRowItem:
                afterID = strings.TrimSpace(it.row.item.ID)
        }
        if beforeID != afterID {
                if m.splitPreviewVisible() {
                        return m, tea.Batch(cmd, m.schedulePreviewCompute())
                }
        }
        return m, cmd
}

func (m *appModel) updateOutlineColumns(msg tea.KeyMsg) (bool, tea.Cmd) {
        if m == nil || m.db == nil {
                return false, nil
        }

        outline, ok := m.db.FindOutline(m.selectedOutlineID)
        if !ok || outline == nil {
                return false, nil
        }

        its := make([]model.Item, 0, 64)
        for _, it := range m.db.Items {
                if it.Archived {
                        continue
                }
                if it.OutlineID != outline.ID {
                        continue
                }
                its = append(its, it)
        }

        board := buildOutlineColumnsBoard(m.db, *outline, its)
        if len(board.cols) == 0 {
                return false, nil
        }

        oid := strings.TrimSpace(outline.ID)
        if m.columnsSel == nil {
                m.columnsSel = map[string]outlineColumnsSelection{}
        }
        sel := board.clamp(m.columnsSel[oid])
        if sel.ItemID == "" {
                if picked, ok := board.selectedItem(sel); ok {
                        sel.ItemID = strings.TrimSpace(picked.Item.ID)
                }
        }

        nCols := len(board.cols)
        switch msg.String() {
        case "tab", "right", "l", "ctrl+f":
                // Explicit navigation should not be pinned by ItemID; clear it so we can move.
                sel.ItemID = ""
                sel.Col = (sel.Col + 1) % nCols
                sel = board.clamp(sel)
                m.columnsSel[oid] = sel
                if it, ok := board.selectedItem(sel); ok {
                        sel.ItemID = strings.TrimSpace(it.Item.ID)
                        m.columnsSel[oid] = sel
                        selectListItemByID(&m.itemsList, strings.TrimSpace(it.Item.ID))
                }
                return true, nil
        case "shift+tab", "left", "h", "ctrl+b":
                sel.ItemID = ""
                sel.Col = (sel.Col - 1 + nCols) % nCols
                sel = board.clamp(sel)
                m.columnsSel[oid] = sel
                if it, ok := board.selectedItem(sel); ok {
                        sel.ItemID = strings.TrimSpace(it.Item.ID)
                        m.columnsSel[oid] = sel
                        selectListItemByID(&m.itemsList, strings.TrimSpace(it.Item.ID))
                }
                return true, nil
        case "down", "j", "ctrl+n":
                sel.ItemID = ""
                sel.Item++
                sel = board.clamp(sel)
                m.columnsSel[oid] = sel
                if it, ok := board.selectedItem(sel); ok {
                        sel.ItemID = strings.TrimSpace(it.Item.ID)
                        m.columnsSel[oid] = sel
                        selectListItemByID(&m.itemsList, strings.TrimSpace(it.Item.ID))
                }
                return true, nil
        case "up", "k", "ctrl+p":
                sel.ItemID = ""
                sel.Item--
                sel = board.clamp(sel)
                m.columnsSel[oid] = sel
                if it, ok := board.selectedItem(sel); ok {
                        sel.ItemID = strings.TrimSpace(it.Item.ID)
                        m.columnsSel[oid] = sel
                        selectListItemByID(&m.itemsList, strings.TrimSpace(it.Item.ID))
                }
                return true, nil
        }

        // Item actions (when an item is selected).
        it, ok := board.selectedItem(sel)
        if !ok {
                return false, nil
        }

        switch msg.String() {
        case "y":
                txt := m.clipboardItemRef(it.Item.ID)
                if err := copyToClipboard(txt); err != nil {
                        m.showMinibuffer("Clipboard error: " + err.Error())
                } else {
                        m.showMinibuffer("Copied: " + txt)
                }
                return true, nil
        case "Y":
                cmd := m.clipboardShowCmd(it.Item.ID)
                if err := copyToClipboard(cmd); err != nil {
                        m.showMinibuffer("Clipboard error: " + err.Error())
                } else {
                        m.showMinibuffer("Copied: " + cmd)
                }
                return true, nil
        case "C":
                m.openTextModal(modalAddComment, it.Item.ID, "Write a comment…", "")
                return true, nil
        case "w":
                m.openTextModal(modalAddWorklog, it.Item.ID, "Log work…", "")
                return true, nil
        case "p":
                if err := m.togglePriority(it.Item.ID); err != nil {
                        return true, m.reportError(it.Item.ID, err)
                }
                return true, nil
        case "o":
                if err := m.toggleOnHold(it.Item.ID); err != nil {
                        return true, m.reportError(it.Item.ID, err)
                }
                return true, nil
        case "a":
                m.openAssigneePicker(it.Item.ID)
                return true, nil
        case "t":
                m.openTagsEditor(it.Item.ID)
                return true, nil
        case "d":
                m.openDateModal(modalSetDue, it.Item.ID, it.Item.Due)
                return true, nil
        case "s":
                m.openDateModal(modalSetSchedule, it.Item.ID, it.Item.Schedule)
                return true, nil
        case "D":
                m.openTextModal(modalEditDescription, it.Item.ID, "Markdown description…", it.Item.Description)
                return true, nil
        case " ":
                m.openStatusPicker(*outline, it.Item.ID, it.Item.StatusID)
                m.modal = modalPickStatus
                m.modalForID = it.Item.ID
                return true, nil
        case "m":
                m.openMoveOutlinePicker(it.Item.ID)
                return true, nil
        case "shift+right":
                if err := m.cycleItemStatus(*outline, it.Item.ID, +1); err != nil {
                        return true, m.reportError(it.Item.ID, err)
                }
                sel.ItemID = strings.TrimSpace(it.Item.ID)
                m.columnsSel[oid] = sel
                return true, nil
        case "shift+left":
                if err := m.cycleItemStatus(*outline, it.Item.ID, -1); err != nil {
                        return true, m.reportError(it.Item.ID, err)
                }
                sel.ItemID = strings.TrimSpace(it.Item.ID)
                m.columnsSel[oid] = sel
                return true, nil
        case "e":
                m.openInputModal(modalEditTitle, it.Item.ID, "Title", it.Item.Title)
                return true, nil
        case "n":
                // New sibling (after selected).
                m.openInputModal(modalNewSibling, it.Item.ID, "Title", "")
                return true, nil
        case "N":
                // New child (under selected).
                m.openInputModal(modalNewChild, it.Item.ID, "Title", "")
                return true, nil
        case "r":
                m.modal = modalConfirmArchive
                m.modalForID = it.Item.ID
                m.archiveFor = archiveTargetItem
                m.input.Blur()
                return true, nil
        case "enter":
                m.view = viewItem
                m.openItemID = it.Item.ID
                m.itemArchivedReadOnly = false
                m.recordRecentItemVisit(m.openItemID)
                m.itemFocus = itemFocusTitle
                m.itemCommentIdx = 0
                m.itemWorklogIdx = 0
                m.itemHistoryIdx = 0
                m.itemSideScroll = 0
                m.showPreview = false
                m.previewCacheForID = ""
                return true, nil
        }

        return false, nil
}

func (m appModel) updateAgenda(msg tea.Msg) (tea.Model, tea.Cmd) {
        switch km := msg.(type) {
        case tea.KeyMsg:
                switch km.String() {
                case "ctrl+c", "q":
                        return m, m.quitWithStateCmd()
                case "x", "?":
                        m.openActionPanel(actionPanelContext)
                        return m, nil
                case "O":
                        // Outline actions menu (like Agenda Commands): open action panel directly in the Outline… subpanel.
                        if m.view == viewOutline || m.view == viewOutlines {
                                m.openActionPanel(actionPanelContext)
                                (&m).pushActionPanel(actionPanelOutline)
                                return m, nil
                        }
                case "g":
                        m.openActionPanel(actionPanelNav)
                        return m, nil
                case "a":
                        m.openActionPanel(actionPanelAgenda)
                        return m, nil
                case "c":
                        m.openActionPanel(actionPanelCapture)
                        return m, nil
                case "tab":
                        if m.splitPreviewVisible() {
                                if m.pane == paneOutline {
                                        m.pane = paneDetail
                                } else {
                                        m.pane = paneOutline
                                }
                                return m, nil
                        }
                case "backspace", "esc":
                        if m.hasAgendaReturnView {
                                m.view = m.agendaReturnView
                                m.hasAgendaReturnView = false
                        } else {
                                m.view = viewProjects
                        }
                        return m, nil
                }

                // Disallow structural/move keys in agenda.
                if strings.HasPrefix(km.String(), "alt+") {
                        m.showMinibuffer("Move/indent is not available in agenda")
                        return m, nil
                }

                // Outline-style navigation (parent/child) for agenda.
                if m.navAgenda(km) {
                        return m, nil
                }

                // Item actions (only when an agenda row is selected).
                it, ok := m.agendaList.SelectedItem().(agendaRowItem)
                if !ok {
                        // Let list handle moving between headings/items.
                        var cmd tea.Cmd
                        m.agendaList, cmd = m.agendaList.Update(msg)
                        return m, cmd
                }
                // Keep outline context in sync so shared helpers behave correctly.
                m.selectedProjectID = it.row.item.ProjectID
                m.selectedOutlineID = it.row.item.OutlineID
                m.selectedOutline = &it.outline

                switch km.String() {
                case "enter":
                        m.selectedProjectID = it.row.item.ProjectID
                        m.selectedOutlineID = it.row.item.OutlineID
                        if o, ok := m.db.FindOutline(it.row.item.OutlineID); ok {
                                m.selectedOutline = o
                        }
                        m.openItemID = it.row.item.ID
                        m.view = viewItem
                        m.itemArchivedReadOnly = false
                        (&m).recordRecentItemVisit(m.openItemID)
                        m.itemFocus = itemFocusTitle
                        m.itemCommentIdx = 0
                        m.itemWorklogIdx = 0
                        m.itemHistoryIdx = 0
                        m.itemSideScroll = 0
                        m.hasReturnView = true
                        m.returnView = viewAgenda
                        m.showPreview = false
                        m.pane = paneOutline
                        return m, nil
                case "y":
                        txt := m.clipboardItemRef(it.row.item.ID)
                        if err := copyToClipboard(txt); err != nil {
                                m.showMinibuffer("Clipboard error: " + err.Error())
                        } else {
                                m.showMinibuffer("Copied: " + txt)
                        }
                        return m, nil
                case "Y":
                        cmd := m.clipboardShowCmd(it.row.item.ID)
                        if err := copyToClipboard(cmd); err != nil {
                                m.showMinibuffer("Clipboard error: " + err.Error())
                        } else {
                                m.showMinibuffer("Copied: " + cmd)
                        }
                        return m, nil
                case "C":
                        m.openTextModal(modalAddComment, it.row.item.ID, "Write a comment…", "")
                        return m, nil
                case "w":
                        m.openTextModal(modalAddWorklog, it.row.item.ID, "Log work…", "")
                        return m, nil
                case "e":
                        m.openInputModal(modalEditTitle, it.row.item.ID, "Title", it.row.item.Title)
                        return m, nil
                case "D":
                        m.openTextModal(modalEditDescription, it.row.item.ID, "Markdown description…", it.row.item.Description)
                        return m, nil
                case " ":
                        m.openStatusPicker(it.outline, it.row.item.ID, it.row.item.StatusID)
                        m.modal = modalPickStatus
                        m.modalForID = it.row.item.ID
                        return m, nil
                case "shift+right":
                        if err := m.cycleItemStatus(it.outline, it.row.item.ID, +1); err != nil {
                                return m, m.reportError(it.row.item.ID, err)
                        }
                        return m, nil
                case "shift+left":
                        if err := m.cycleItemStatus(it.outline, it.row.item.ID, -1); err != nil {
                                return m, m.reportError(it.row.item.ID, err)
                        }
                        return m, nil
                case "r":
                        m.modal = modalConfirmArchive
                        m.modalForID = it.row.item.ID
                        m.archiveFor = archiveTargetItem
                        m.input.Blur()
                        return m, nil
                case "z":
                        m.toggleAgendaCollapseSelected()
                        return m, nil
                case "Z":
                        m.toggleAgendaCollapseAll()
                        return m, nil
                }
        }

        var cmd tea.Cmd
        m.agendaList, cmd = m.agendaList.Update(msg)
        return m, cmd
}

func agendaDepth(it list.Item) int {
        switch t := it.(type) {
        case agendaHeadingItem:
                _ = t
                return 0
        case agendaRowItem:
                return 1 + t.row.depth
        default:
                return 0
        }
}

func (m *appModel) navAgenda(msg tea.KeyMsg) bool {
        if m == nil {
                return false
        }
        items := m.agendaList.Items()
        if len(items) == 0 {
                return false
        }
        idx := m.agendaList.Index()
        if idx < 0 {
                idx = 0
        }
        if idx >= len(items) {
                idx = len(items) - 1
        }
        curDepth := agendaDepth(items[idx])

        switch msg.String() {
        case "right", "l", "ctrl+f":
                // Expand if collapsed.
                if it, ok := items[idx].(agendaRowItem); ok {
                        if it.row.hasChildren && it.row.collapsed {
                                m.agendaCollapsed[it.row.item.ID] = false
                                m.refreshAgenda()
                                selectListItemByID(&m.agendaList, it.row.item.ID)
                                return true
                        }
                }
                // Move to first child if the next row is deeper.
                if idx+1 < len(items) && agendaDepth(items[idx+1]) > curDepth {
                        m.agendaList.Select(idx + 1)
                        return true
                }
        case "left", "h", "ctrl+b":
                // Go to parent (previous item with depth == curDepth-1).
                want := curDepth - 1
                if want < 0 {
                        want = 0
                }
                for i := idx - 1; i >= 0; i-- {
                        if agendaDepth(items[i]) == want {
                                m.agendaList.Select(i)
                                return true
                        }
                }
        }

        return false
}

func (m *appModel) toggleAgendaCollapseSelected() {
        if m == nil {
                return
        }
        it, ok := m.agendaList.SelectedItem().(agendaRowItem)
        if !ok {
                return
        }
        if !it.row.hasChildren {
                return
        }
        id := strings.TrimSpace(it.row.item.ID)
        if id == "" {
                return
        }
        m.agendaCollapsed[id] = !m.agendaCollapsed[id]
        m.refreshAgenda()
        selectListItemByID(&m.agendaList, id)
}

func (m *appModel) toggleAgendaCollapseAll() {
        if m == nil {
                return
        }
        items := m.agendaList.Items()
        anyCollapsed := false
        for _, it := range items {
                if r, ok := it.(agendaRowItem); ok {
                        if r.row.hasChildren && r.row.collapsed {
                                anyCollapsed = true
                                break
                        }
                }
        }
        if anyCollapsed {
                // Expand all.
                for _, it := range items {
                        if r, ok := it.(agendaRowItem); ok {
                                if r.row.hasChildren {
                                        m.agendaCollapsed[r.row.item.ID] = false
                                }
                        }
                }
        } else {
                // Collapse all nodes with children.
                for _, it := range items {
                        if r, ok := it.(agendaRowItem); ok {
                                if r.row.hasChildren {
                                        m.agendaCollapsed[r.row.item.ID] = true
                                }
                        }
                }
        }
        m.refreshAgenda()
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
                res, err := mutate.SetItemArchived(m.db, actorID, t.ID, true)
                if err != nil || !res.Changed {
                        continue
                }
                res.Item.UpdatedAt = now
                if err := m.appendEvent(actorID, "item.archive", res.Item.ID, res.EventPayload); err != nil {
                        return archived, err
                }
                archived++
        }

        if err := m.store.Save(m.db); err != nil {
                return archived, err
        }
        m.refreshEventsTail()
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
                res, err := mutate.SetItemArchived(m.db, actorID, it.ID, true)
                if err != nil || !res.Changed {
                        continue
                }
                res.Item.UpdatedAt = now
                if err := m.appendEvent(actorID, "item.archive", res.Item.ID, res.EventPayload); err != nil {
                        return itemsArchived, err
                }
                itemsArchived++
        }

        o.Archived = true
        if err := m.appendEvent(actorID, "outline.archive", o.ID, map[string]any{"archived": true}); err != nil {
                return itemsArchived, err
        }

        if err := m.store.Save(m.db); err != nil {
                return itemsArchived, err
        }
        m.refreshEventsTail()
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
                if err := m.appendEvent(actorID, "outline.archive", o.ID, map[string]any{"archived": true}); err != nil {
                        return outlinesArchived, 0, err
                }
                outlinesArchived++
        }

        now := time.Now().UTC()
        itemsArchived := 0
        for i := range m.db.Items {
                it := &m.db.Items[i]
                if it.ProjectID != projectID {
                        continue
                }
                res, err := mutate.SetItemArchived(m.db, actorID, it.ID, true)
                if err != nil || !res.Changed {
                        continue
                }
                res.Item.UpdatedAt = now
                if err := m.appendEvent(actorID, "item.archive", res.Item.ID, res.EventPayload); err != nil {
                        return outlinesArchived, itemsArchived, err
                }
                itemsArchived++
        }

        p.Archived = true
        if err := m.appendEvent(actorID, "project.archive", p.ID, map[string]any{"archived": true}); err != nil {
                return outlinesArchived, itemsArchived, err
        }

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
        shadowStyle := lipgloss.NewStyle().Background(colorShadow)
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
        if lipgloss.HasDarkBackground() {
                return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Faint(true).Render(s)
        }
        // On light terminals, faint often becomes illegible; just soften the foreground.
        return lipgloss.NewStyle().Foreground(lipgloss.Color("248")).Render(s)
}

func renderModalBox(screenWidth int, title, body string) string {
        w := modalBoxWidth(screenWidth)

        header := lipgloss.NewStyle().Bold(true).Render(title)
        content := header + "\n\n" + body

        box := lipgloss.NewStyle().
                Width(w).
                Padding(1, 2).
                Border(lipgloss.RoundedBorder()).
                BorderForeground(colorAccent).
                Foreground(colorSurfaceFg).
                Background(colorSurfaceBg)
        return box.Render(content)
}

func modalBoxWidth(screenWidth int) int {
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
        return w
}

func modalBodyWidth(screenWidth int) int {
        w := modalBoxWidth(screenWidth)
        bodyW := w - 6 // renderModalBox has padding 1,2 and a border.
        if bodyW < 10 {
                bodyW = 10
        }
        return bodyW
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

func (m *appModel) navOutline(msg tea.KeyMsg) bool {
        switch msg.String() {
        case "down", "j", "ctrl+n":
                // Skip non-item rows (e.g. outline preface and document-mode inline description rows).
                items := m.itemsList.Items()
                if len(items) == 0 {
                        return true
                }
                idx := m.itemsList.Index()
                if idx < 0 {
                        idx = 0
                        m.itemsList.Select(idx)
                } else if idx >= len(items) {
                        idx = len(items) - 1
                        m.itemsList.Select(idx)
                }
                for i := idx + 1; i < len(items); i++ {
                        switch items[i].(type) {
                        case outlineRowItem, addItemRow:
                                m.itemsList.Select(i)
                                return true
                        }
                }
                return true
        case "up", "k", "ctrl+p":
                items := m.itemsList.Items()
                if len(items) == 0 {
                        return true
                }
                idx := m.itemsList.Index()
                if idx < 0 {
                        idx = 0
                        m.itemsList.Select(idx)
                } else if idx >= len(items) {
                        idx = len(items) - 1
                        m.itemsList.Select(idx)
                }
                for i := idx - 1; i >= 0; i-- {
                        switch items[i].(type) {
                        case outlineRowItem, addItemRow:
                                m.itemsList.Select(i)
                                return true
                        }
                }
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
                if it.row.hasDescription && it.row.collapsed {
                        m.collapsed[it.row.item.ID] = false
                        m.refreshItems(it.outline)
                }
                return
        }
        if it.row.collapsed {
                m.collapsed[it.row.item.ID] = false
                m.refreshItems(it.outline)
        }
        idx := m.itemsList.Index()
        // In our flattening, the first child (if visible) is the next outlineRowItem with depth+1.
        items := m.itemsList.Items()
        for i := idx + 1; i < len(items); i++ {
                next, ok := items[i].(outlineRowItem)
                if !ok {
                        continue
                }
                if next.row.depth == it.row.depth+1 {
                        m.itemsList.Select(i)
                }
                return
        }
}

func (m *appModel) navToParent() {
        it, ok := m.itemsList.SelectedItem().(outlineRowItem)
        if !ok {
                return
        }
        idx := m.itemsList.Index()
        items := m.itemsList.Items()
        if idx >= len(items) {
                idx = len(items) - 1
                if idx < 0 {
                        return
                }
                m.itemsList.Select(idx)
        }
        if idx <= 0 || it.row.depth <= 0 {
                return
        }
        wantDepth := it.row.depth - 1
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
        if !it.row.hasChildren && !it.row.hasDescription {
                return
        }
        m.collapsed[it.row.item.ID] = !m.collapsed[it.row.item.ID]
        m.refreshItems(it.outline)
}

func (m *appModel) toggleCollapseAll() {
        if m.selectedOutline == nil {
                return
        }

        // If anything collapsible is expanded, collapse all; otherwise expand all.
        collapsible := map[string]bool{}
        mode := m.curOutlineViewMode()
        for _, it := range m.db.Items {
                if it.Archived || it.OutlineID != m.selectedOutline.ID {
                        continue
                }
                if it.ParentID != nil && *it.ParentID != "" {
                        collapsible[*it.ParentID] = true
                }
                if mode != outlineViewModeColumns && strings.TrimSpace(it.Description) != "" {
                        collapsible[it.ID] = true
                }
        }

        anyExpanded := false
        for id := range collapsible {
                if !m.collapsed[id] {
                        anyExpanded = true
                        break
                }
        }

        for id := range collapsible {
                m.collapsed[id] = anyExpanded
        }
        m.refreshItems(*m.selectedOutline)
}

func (m *appModel) mutateOutlineByKey(msg tea.KeyMsg) (bool, tea.Cmd) {
        // Move item down/up.
        if isMoveDown(msg) {
                itemID := ""
                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                        itemID = it.row.item.ID
                }
                if err := m.moveSelected("down"); err != nil {
                        return true, m.reportError(itemID, err)
                }
                return true, nil
        }
        if isMoveUp(msg) {
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
        if isIndent(msg) {
                itemID := ""
                if it, ok := m.itemsList.SelectedItem().(outlineRowItem); ok {
                        itemID = it.row.item.ID
                }
                if err := m.indentSelected(); err != nil {
                        return true, m.reportError(itemID, err)
                }
                return true, nil
        }
        if isOutdent(msg) {
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

// Move/indent key helpers.
//
// Alt+Arrows work in terminals that either:
// - report modifier keys (Bubble Tea sets msg.Alt + arrow type), or
// - send ESC then <key> (handled via m.pendingEsc earlier in Update()).
//
// macOS Terminal.app often sends CSI sequences with a different modifier code
// (e.g. ";9") that Bubble Tea treats as unknown CSI. As a reliable
// cross-terminal fallback we also support Ctrl+Shift+Arrows for move/indent.
func isMoveDown(msg tea.KeyMsg) bool {
        // Fallbacks that don't shadow navigation keys.
        if msg.String() == "ctrl+j" {
                return true
        }
        return isAltDown(msg) || msg.Type == tea.KeyShiftDown
}

func isMoveUp(msg tea.KeyMsg) bool {
        if msg.String() == "ctrl+k" {
                return true
        }
        return isAltUp(msg) || msg.Type == tea.KeyShiftUp
}

func isIndent(msg tea.KeyMsg) bool {
        // Keep indent/outdent on Alt+Left/Right; user reports this already works in Terminal.app.
        // Also support Ctrl+L / Ctrl+H since Ctrl+Arrow is often unreliable across terminals.
        return isAltRight(msg) || msg.Type == tea.KeyCtrlL || msg.String() == "ctrl+l"
}

func isOutdent(msg tea.KeyMsg) bool {
        return isAltLeft(msg) || msg.Type == tea.KeyCtrlH || msg.String() == "ctrl+h"
}

// keyMsgFromUnknownCSIString attempts to interpret Bubble Tea's "unknown CSI"
// debug strings (e.g. "?CSI[49 59 57 65]?") for terminals that emit modifier
// sequences Bubble Tea doesn't map (notably macOS Terminal.app Option+Arrow).
func keyMsgFromUnknownCSIString(s string) (tea.KeyMsg, bool) {
        seq, ok := decodeUnknownCSIString(s)
        if !ok {
                return tea.KeyMsg{}, false
        }

        // Typical arrow CSI payloads look like: "1;9A" (up), "1;9B" (down), etc.
        // We interpret ";9" as Alt for the purpose of outline structure operations.
        if strings.Contains(seq, ";9") {
                switch {
                case strings.HasSuffix(seq, "A"):
                        return tea.KeyMsg{Type: tea.KeyUp, Alt: true}, true
                case strings.HasSuffix(seq, "B"):
                        return tea.KeyMsg{Type: tea.KeyDown, Alt: true}, true
                case strings.HasSuffix(seq, "C"):
                        return tea.KeyMsg{Type: tea.KeyRight, Alt: true}, true
                case strings.HasSuffix(seq, "D"):
                        return tea.KeyMsg{Type: tea.KeyLeft, Alt: true}, true
                }
        }

        // Ctrl+Arrow is typically reported as ";5" (xterm style). Bubble Tea
        // already maps common sequences, but some terminals end up in "unknown CSI".
        if strings.Contains(seq, ";5") {
                switch {
                case strings.HasSuffix(seq, "C"):
                        return tea.KeyMsg{Type: tea.KeyCtrlRight}, true
                case strings.HasSuffix(seq, "D"):
                        return tea.KeyMsg{Type: tea.KeyCtrlLeft}, true
                }
        }

        // Some terminals report Ctrl+Shift arrows with a different modifier code than
        // Bubble Tea's built-in ";6" mapping. Best-effort support if we see ";10".
        if strings.Contains(seq, ";10") {
                switch {
                case strings.HasSuffix(seq, "A"):
                        return tea.KeyMsg{Type: tea.KeyCtrlShiftUp}, true
                case strings.HasSuffix(seq, "B"):
                        return tea.KeyMsg{Type: tea.KeyCtrlShiftDown}, true
                case strings.HasSuffix(seq, "C"):
                        return tea.KeyMsg{Type: tea.KeyCtrlShiftRight}, true
                case strings.HasSuffix(seq, "D"):
                        return tea.KeyMsg{Type: tea.KeyCtrlShiftLeft}, true
                }
        }

        return tea.KeyMsg{}, false
}

func decodeUnknownCSIString(s string) (string, bool) {
        // Bubble Tea formats unknown CSI strings like: "?CSI[49 59 57 65]?"
        const prefix = "?CSI["
        const suffix = "]?"
        if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, suffix) {
                return "", false
        }
        body := strings.TrimSuffix(strings.TrimPrefix(s, prefix), suffix)
        body = strings.TrimSpace(body)
        if body == "" {
                return "", false
        }
        parts := strings.Fields(body)
        out := make([]byte, 0, len(parts))
        for _, p := range parts {
                n, err := strconv.Atoi(p)
                if err != nil || n < 0 || n > 255 {
                        return "", false
                }
                out = append(out, byte(n))
        }
        return string(out), true
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
        actorID := m.currentWriteActorID()
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
                StatusID:           store.FirstStatusID(outline.StatusDefs),
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

        if err := m.appendEvent(actorID, "item.create", newItem.ID, newItem); err != nil {
                return err
        }
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        m.refreshEventsTail()
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

type itemMutationResult struct {
        eventType    string
        eventPayload map[string]any
        minibuffer   string
        // refreshPreview clears the preview cache (useful when description/fields affecting
        // the preview pane are updated).
        refreshPreview bool
}

type projectMutationResult struct {
        eventType    string
        eventPayload map[string]any
        minibuffer   string
}

type outlineMutationResult struct {
        eventType    string
        eventPayload map[string]any
        minibuffer   string
}

// mutateItem centralizes the common mutation flow:
// load db → resolve edit actor → permission check → mutate → save → append event → minibuffer → refresh UI.
//
// If mutate returns changed=false, no save/event/minibuffer happens.
func (m *appModel) mutateItem(itemID string, mutate func(db *store.DB, it *model.Item) (changed bool, res itemMutationResult, err error)) error {
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

        it, ok := m.db.FindItem(itemID)
        if !ok {
                return nil
        }
        if !canEditItem(m.db, actorID, it) {
                return errors.New("permission denied")
        }

        changed, res, err := mutate(m.db, it)
        if err != nil {
                return err
        }
        if !changed {
                return nil
        }

        it.UpdatedAt = time.Now().UTC()
        if strings.TrimSpace(res.eventType) != "" {
                if err := m.appendEvent(actorID, res.eventType, it.ID, res.eventPayload); err != nil {
                        return err
                }
                // Keep in-memory history fresh for the item detail "History" section.
                m.refreshEventsTail()
        }
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        m.captureStoreModTimes()
        if strings.TrimSpace(res.minibuffer) != "" {
                m.showMinibuffer(res.minibuffer)
        }
        if res.refreshPreview {
                m.previewCacheForID = ""
        }

        if m.selectedOutline != nil {
                if o, ok := m.db.FindOutline(m.selectedOutline.ID); ok {
                        m.selectedOutline = o
                }
                m.refreshItems(*m.selectedOutline)
                selectListItemByID(&m.itemsList, it.ID)
        }
        // If we're in agenda, immediately refresh so edits are visible (and filtering/grouping
        // updates, e.g. when an item becomes DONE and disappears).
        if m.view == viewAgenda {
                m.refreshAgenda()
                selectListItemByID(&m.agendaList, it.ID)
        }

        return nil
}

// mutateProject applies a mutation to a project and centralizes:
// load db → resolve edit actor → mutate → save → append event → minibuffer → refresh UI.
//
// If mutate returns changed=false, no save/event/minibuffer happens.
func (m *appModel) mutateProject(projectID string, mutate func(db *store.DB, p *model.Project) (changed bool, res projectMutationResult, err error)) error {
        projectID = strings.TrimSpace(projectID)
        if projectID == "" {
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

        p, ok := m.db.FindProject(projectID)
        if !ok {
                return nil
        }

        changed, res, err := mutate(m.db, p)
        if err != nil {
                return err
        }
        if !changed {
                return nil
        }

        if strings.TrimSpace(res.eventType) != "" {
                if err := m.appendEvent(actorID, res.eventType, p.ID, res.eventPayload); err != nil {
                        return err
                }
                m.refreshEventsTail()
        }
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        m.captureStoreModTimes()
        if strings.TrimSpace(res.minibuffer) != "" {
                m.showMinibuffer(res.minibuffer)
        }

        // Refresh visible lists.
        m.refreshProjects()
        selectListItemByID(&m.projectsList, p.ID)
        if m.view == viewOutlines || m.view == viewOutline || m.view == viewItem {
                // Breadcrumb depends on project name; also keep outlines list stable if visible.
                if strings.TrimSpace(m.selectedProjectID) == strings.TrimSpace(p.ID) || strings.TrimSpace(m.selectedProjectID) == "" {
                        m.refreshOutlines(p.ID)
                }
        }
        if m.view == viewAgenda {
                m.refreshAgenda()
        }

        return nil
}

// mutateOutline applies a mutation to an outline and centralizes:
// load db → resolve edit actor → mutate → save → append event → minibuffer → refresh UI.
//
// If mutate returns changed=false, no save/event/minibuffer happens.
func (m *appModel) mutateOutline(outlineID string, mutate func(db *store.DB, o *model.Outline) (changed bool, res outlineMutationResult, err error)) error {
        outlineID = strings.TrimSpace(outlineID)
        if outlineID == "" {
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

        o, ok := m.db.FindOutline(outlineID)
        if !ok {
                return nil
        }

        changed, res, err := mutate(m.db, o)
        if err != nil {
                return err
        }
        if !changed {
                return nil
        }

        if strings.TrimSpace(res.eventType) != "" {
                if err := m.appendEvent(actorID, res.eventType, o.ID, res.eventPayload); err != nil {
                        return err
                }
                m.refreshEventsTail()
        }
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        m.captureStoreModTimes()
        if strings.TrimSpace(res.minibuffer) != "" {
                m.showMinibuffer(res.minibuffer)
        }

        // Refresh visible lists.
        if strings.TrimSpace(o.ProjectID) != "" {
                m.refreshOutlines(o.ProjectID)
                selectListItemByID(&m.outlinesList, o.ID)
        }
        if m.view == viewOutline || m.view == viewItem {
                if strings.TrimSpace(m.selectedOutlineID) == strings.TrimSpace(o.ID) {
                        m.refreshItems(*o)
                }
        }
        if m.view == viewAgenda {
                m.refreshAgenda()
        }

        return nil
}

func (m *appModel) setTitleFromModal(title string) error {
        itemID := strings.TrimSpace(m.modalForID)
        if itemID == "" {
                return nil
        }

        newTitle := strings.TrimSpace(title)
        return m.mutateItem(itemID, func(_ *store.DB, it *model.Item) (bool, itemMutationResult, error) {
                prev := strings.TrimSpace(it.Title)
                if prev == newTitle {
                        return false, itemMutationResult{}, nil
                }
                it.Title = newTitle
                return true, itemMutationResult{
                        eventType:    "item.set_title",
                        eventPayload: map[string]any{"title": it.Title},
                        minibuffer:   "Title updated",
                }, nil
        })
}

func (m *appModel) setDescriptionFromModal(description string) error {
        itemID := strings.TrimSpace(m.modalForID)
        if itemID == "" {
                return nil
        }

        newDesc := strings.TrimSpace(description)
        return m.mutateItem(itemID, func(_ *store.DB, it *model.Item) (bool, itemMutationResult, error) {
                prev := strings.TrimSpace(it.Description)
                if prev == newDesc {
                        return false, itemMutationResult{}, nil
                }
                it.Description = newDesc
                return true, itemMutationResult{
                        eventType:      "item.set_description",
                        eventPayload:   map[string]any{"description": it.Description},
                        minibuffer:     "Description updated",
                        refreshPreview: true,
                }, nil
        })
}

func (m *appModel) togglePriority(itemID string) error {
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return nil
        }
        return m.mutateItem(itemID, func(_ *store.DB, it *model.Item) (bool, itemMutationResult, error) {
                it.Priority = !it.Priority
                return true, itemMutationResult{
                        eventType:    "item.set_priority",
                        eventPayload: map[string]any{"priority": it.Priority},
                        minibuffer:   fmt.Sprintf("Priority: %v", it.Priority),
                }, nil
        })
}

func (m *appModel) toggleOnHold(itemID string) error {
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return nil
        }
        return m.mutateItem(itemID, func(_ *store.DB, it *model.Item) (bool, itemMutationResult, error) {
                it.OnHold = !it.OnHold
                return true, itemMutationResult{
                        eventType:    "item.set_on_hold",
                        eventPayload: map[string]any{"onHold": it.OnHold},
                        minibuffer:   fmt.Sprintf("On hold: %v", it.OnHold),
                }, nil
        })
}

func (m *appModel) setAssignedActor(itemID string, assignedActorID *string) error {
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return nil
        }
        next := ""
        if assignedActorID != nil {
                next = strings.TrimSpace(*assignedActorID)
        }
        return m.mutateItem(itemID, func(db *store.DB, it *model.Item) (bool, itemMutationResult, error) {
                actorID := m.editActorID()
                if actorID == "" {
                        return false, itemMutationResult{}, errors.New("no current actor")
                }

                var target *string
                if next != "" {
                        tmp := next
                        target = &tmp
                }
                res, err := mutate.SetAssignedActor(db, actorID, it.ID, target, mutate.AssignOpts{TakeAssigned: true})
                if err != nil {
                        switch e := err.(type) {
                        case mutate.NotFoundError:
                                if strings.TrimSpace(e.Kind) == "actor" {
                                        return false, itemMutationResult{}, errors.New("actor not found")
                                }
                                return false, itemMutationResult{}, nil
                        case mutate.OwnerOnlyError:
                                return false, itemMutationResult{}, errors.New("permission denied")
                        default:
                                return false, itemMutationResult{}, err
                        }
                }
                if !res.Changed {
                        return false, itemMutationResult{}, nil
                }
                if target == nil {
                        return true, itemMutationResult{
                                eventType:    "item.set_assign",
                                eventPayload: res.EventPayload,
                                minibuffer:   "Unassigned",
                        }, nil
                }
                return true, itemMutationResult{
                        eventType:    "item.set_assign",
                        eventPayload: res.EventPayload,
                        minibuffer:   "Assigned: @" + actorDisplayLabel(db, next),
                }, nil
        })
}

func (m *appModel) setTagChecked(itemID, tag string, checked bool) error {
        itemID = strings.TrimSpace(itemID)
        tag = normalizeTag(tag)
        if itemID == "" || tag == "" {
                return nil
        }
        return m.mutateItem(itemID, func(_ *store.DB, it *model.Item) (bool, itemMutationResult, error) {
                has := false
                for _, t := range it.Tags {
                        if normalizeTag(t) == tag {
                                has = true
                                break
                        }
                }
                if checked {
                        if has {
                                return false, itemMutationResult{}, nil
                        }
                        it.Tags = append(it.Tags, tag)
                        it.Tags = uniqueSortedStrings(it.Tags)
                        return true, itemMutationResult{
                                eventType:    "item.tags_add",
                                eventPayload: map[string]any{"tag": tag},
                                minibuffer:   "Tag added: #" + tag,
                        }, nil
                }
                if !has {
                        return false, itemMutationResult{}, nil
                }
                nextTags := make([]string, 0, len(it.Tags))
                for _, t := range it.Tags {
                        if normalizeTag(t) == tag {
                                continue
                        }
                        t = normalizeTag(t)
                        if t == "" {
                                continue
                        }
                        nextTags = append(nextTags, t)
                }
                it.Tags = uniqueSortedStrings(nextTags)
                return true, itemMutationResult{
                        eventType:    "item.tags_remove",
                        eventPayload: map[string]any{"tag": tag},
                        minibuffer:   "Tag removed: #" + tag,
                }, nil
        })
}

func (m *appModel) setDue(itemID string, dt *model.DateTime) error {
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return nil
        }
        return m.mutateItem(itemID, func(_ *store.DB, it *model.Item) (bool, itemMutationResult, error) {
                // Treat structurally-equal values as no-op.
                same := func(a, b *model.DateTime) bool {
                        if a == nil && b == nil {
                                return true
                        }
                        if a == nil || b == nil {
                                return false
                        }
                        if strings.TrimSpace(a.Date) != strings.TrimSpace(b.Date) {
                                return false
                        }
                        at := ""
                        bt := ""
                        if a.Time != nil {
                                at = strings.TrimSpace(*a.Time)
                        }
                        if b.Time != nil {
                                bt = strings.TrimSpace(*b.Time)
                        }
                        return at == bt
                }
                if same(it.Due, dt) {
                        return false, itemMutationResult{}, nil
                }
                it.Due = dt
                msg := "Due cleared"
                if dt != nil {
                        msg = "Due: " + formatDateTimeOutline(dt)
                }
                return true, itemMutationResult{
                        eventType:    "item.set_due",
                        eventPayload: map[string]any{"due": it.Due},
                        minibuffer:   msg,
                }, nil
        })
}

func (m *appModel) setSchedule(itemID string, dt *model.DateTime) error {
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return nil
        }
        return m.mutateItem(itemID, func(_ *store.DB, it *model.Item) (bool, itemMutationResult, error) {
                same := func(a, b *model.DateTime) bool {
                        if a == nil && b == nil {
                                return true
                        }
                        if a == nil || b == nil {
                                return false
                        }
                        if strings.TrimSpace(a.Date) != strings.TrimSpace(b.Date) {
                                return false
                        }
                        at := ""
                        bt := ""
                        if a.Time != nil {
                                at = strings.TrimSpace(*a.Time)
                        }
                        if b.Time != nil {
                                bt = strings.TrimSpace(*b.Time)
                        }
                        return at == bt
                }
                if same(it.Schedule, dt) {
                        return false, itemMutationResult{}, nil
                }
                it.Schedule = dt
                msg := "Schedule cleared"
                if dt != nil {
                        msg = "Schedule: " + formatDateTimeOutline(dt)
                }
                return true, itemMutationResult{
                        eventType:    "item.set_schedule",
                        eventPayload: map[string]any{"schedule": it.Schedule},
                        minibuffer:   msg,
                }, nil
        })
}

func (m *appModel) setOutlineNameFromModal(name string) error {
        outlineID := strings.TrimSpace(m.modalForID)
        if outlineID == "" {
                return nil
        }
        trim := strings.TrimSpace(name)
        return m.mutateOutline(outlineID, func(_ *store.DB, o *model.Outline) (bool, outlineMutationResult, error) {
                prev := ""
                if o.Name != nil {
                        prev = strings.TrimSpace(*o.Name)
                }
                next := trim
                if prev == next {
                        return false, outlineMutationResult{}, nil
                }
                if next == "" {
                        o.Name = nil
                } else {
                        tmp := next
                        o.Name = &tmp
                }
                return true, outlineMutationResult{
                        eventType:    "outline.rename",
                        eventPayload: map[string]any{"name": o.Name},
                        minibuffer:   "Renamed outline " + o.ID,
                }, nil
        })
}

func (m *appModel) setOutlineDescriptionFromModal(description string) error {
        outlineID := strings.TrimSpace(m.modalForID)
        if outlineID == "" {
                return nil
        }
        newDesc := strings.TrimSpace(description)
        return m.mutateOutline(outlineID, func(_ *store.DB, o *model.Outline) (bool, outlineMutationResult, error) {
                prev := strings.TrimSpace(o.Description)
                if prev == newDesc {
                        return false, outlineMutationResult{}, nil
                }
                o.Description = newDesc
                return true, outlineMutationResult{
                        eventType:    "outline.set_description",
                        eventPayload: map[string]any{"description": o.Description},
                        minibuffer:   "Outline description updated",
                }, nil
        })
}

func (m *appModel) createProjectFromModal(name string) error {
        name = strings.TrimSpace(name)
        if name == "" {
                return nil
        }
        actorID := m.editActorID()
        if actorID == "" {
                return errors.New("no current actor")
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
        if err := m.appendEvent(actorID, "project.create", p.ID, p); err != nil {
                return err
        }
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        m.refreshEventsTail()
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
        return m.mutateProject(projectID, func(_ *store.DB, p *model.Project) (bool, projectMutationResult, error) {
                prev := strings.TrimSpace(p.Name)
                if prev == name {
                        return false, projectMutationResult{}, nil
                }
                p.Name = name
                return true, projectMutationResult{
                        eventType:    "project.rename",
                        eventPayload: map[string]any{"name": p.Name},
                        minibuffer:   "Renamed project " + p.ID,
                }, nil
        })
}

func (m *appModel) createOutlineFromModal(name string) error {
        actorID := m.editActorID()
        if actorID == "" {
                return errors.New("no current actor")
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
        if err := m.appendEvent(actorID, "outline.create", o.ID, o); err != nil {
                return err
        }
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        m.refreshEventsTail()
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
        _ = itemID // reserved for future (e.g. contextual hints)
        m.openStatusPickerForOutline(outline, currentStatusID, true)
}

func (m *appModel) openStatusPickerForOutline(outline model.Outline, currentStatusID string, includeEmpty bool) {
        opts := []list.Item{}
        if includeEmpty {
                opts = append(opts, statusOptionItem{id: "", label: "(no status)"})
        }
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

func (m *appModel) openAssigneePicker(itemID string) {
        if m == nil || m.db == nil {
                return
        }
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return
        }
        it, ok := m.db.FindItem(itemID)
        if !ok || it == nil {
                return
        }
        cur := ""
        if it.AssignedActorID != nil {
                cur = strings.TrimSpace(*it.AssignedActorID)
        }

        opts := []list.Item{assigneeOptionItem{id: "", label: ""}}
        actors := append([]model.Actor(nil), m.db.Actors...)
        sort.Slice(actors, func(i, j int) bool {
                ai := strings.ToLower(strings.TrimSpace(actors[i].Name))
                aj := strings.ToLower(strings.TrimSpace(actors[j].Name))
                if ai == aj {
                        return actors[i].ID < actors[j].ID
                }
                if ai == "" {
                        return false
                }
                if aj == "" {
                        return true
                }
                return ai < aj
        })
        for _, a := range actors {
                lbl := actorDisplayLabel(m.db, a.ID)
                opts = append(opts, assigneeOptionItem{id: a.ID, label: lbl})
        }

        m.assigneeList.Title = ""
        m.assigneeList.SetItems(opts)
        bodyW := modalBodyWidth(m.width)
        h := len(opts) + 2
        if h > 14 {
                h = 14
        }
        if h < 6 {
                h = 6
        }
        m.assigneeList.SetSize(bodyW, h)

        selected := 0
        for i := 0; i < len(opts); i++ {
                if o, ok := opts[i].(assigneeOptionItem); ok && strings.TrimSpace(o.id) == cur {
                        selected = i
                        break
                }
        }
        m.assigneeList.Select(selected)

        m.modal = modalPickAssignee
        m.modalForID = itemID
}

func (m *appModel) openTagsEditor(itemID string) {
        if m == nil || m.db == nil {
                return
        }
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return
        }
        m.refreshTagsEditor(itemID, "")
        m.tagsFocus = tagsFocusInput
        if m.tagsListActive != nil {
                *m.tagsListActive = false
        }
        m.input.Placeholder = "Add tag"
        m.input.SetValue("")
        m.input.Focus()
        m.modal = modalEditTags
        m.modalForID = itemID
}

func (m *appModel) refreshTagsEditor(itemID string, preferredTag string) {
        if m == nil || m.db == nil {
                return
        }
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return
        }
        it, ok := m.db.FindItem(itemID)
        if !ok || it == nil {
                return
        }

        // Collect available tags from the outline (plus current item tags).
        all := make([]string, 0, 32)
        for _, x := range m.db.Items {
                if x.Archived {
                        continue
                }
                if strings.TrimSpace(x.OutlineID) != strings.TrimSpace(it.OutlineID) {
                        continue
                }
                for _, t := range x.Tags {
                        t = normalizeTag(t)
                        if t == "" {
                                continue
                        }
                        all = append(all, t)
                }
        }
        for _, t := range it.Tags {
                t = normalizeTag(t)
                if t == "" {
                        continue
                }
                all = append(all, t)
        }
        all = uniqueSortedStrings(all)

        checked := map[string]bool{}
        for _, t := range it.Tags {
                t = normalizeTag(t)
                if t == "" {
                        continue
                }
                checked[t] = true
        }

        opts := make([]list.Item, 0, len(all))
        for _, tag := range all {
                opts = append(opts, tagOptionItem{tag: tag, checked: checked[tag]})
        }

        // Preserve selection when possible.
        selectedTag := strings.TrimSpace(preferredTag)
        if selectedTag == "" {
                if cur, ok := m.tagsList.SelectedItem().(tagOptionItem); ok {
                        selectedTag = strings.TrimSpace(cur.tag)
                }
        }

        m.tagsList.Title = ""
        m.tagsList.SetItems(opts)
        bodyW := modalBodyWidth(m.width)
        h := len(opts) + 2
        if h > 12 {
                h = 12
        }
        if h < 4 {
                h = 4
        }
        m.tagsList.SetSize(bodyW, h)

        selectedIdx := 0
        if selectedTag != "" {
                for i := 0; i < len(opts); i++ {
                        if o, ok := opts[i].(tagOptionItem); ok && strings.TrimSpace(o.tag) == selectedTag {
                                selectedIdx = i
                                break
                        }
                }
        }
        m.tagsList.Select(selectedIdx)
}

func (m *appModel) openWorkspacePicker() {
        if m == nil {
                return
        }
        ws, err := store.ListWorkspaceEntries()
        if err != nil {
                m.showMinibuffer("Workspace error: " + err.Error())
                return
        }

        cur := strings.TrimSpace(m.workspace)
        if cur == "" {
                if cfg, err := store.LoadConfig(); err == nil {
                        cur = strings.TrimSpace(cfg.CurrentWorkspace)
                }
        }
        if cur == "" {
                cur = "default"
        }

        seen := map[string]bool{}
        names := []string{}
        descByName := map[string]string{}

        // Keep ListWorkspaceEntries ordering but ensure we include current.
        for _, e := range ws {
                n := strings.TrimSpace(e.Name)
                if n == "" || seen[n] {
                        continue
                }
                seen[n] = true
                names = append(names, n)
                if !e.Legacy {
                        if p := strings.TrimSpace(e.Ref.Path); p != "" {
                                descByName[n] = p
                        }
                }
        }
        if !seen[cur] {
                names = append([]string{cur}, names...)
        }

        items := make([]list.Item, 0, len(names))
        for _, n := range names {
                items = append(items, workspaceItem{name: n, desc: descByName[n], current: n == cur})
        }
        m.workspaceList.Title = ""
        m.workspaceList.SetItems(items)

        // Size similarly to other pickers.
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
        h := len(items) + 2
        if h > 16 {
                h = 16
        }
        if h < 8 {
                h = 8
        }
        m.workspaceList.SetSize(modalW-6, h)

        selectListItemByID(&m.workspaceList, cur)

        m.modal = modalPickWorkspace
        m.modalForID = ""
        m.modalForKey = ""
}

func (m appModel) switchWorkspaceTo(name string) (appModel, error) {
        name, err := store.NormalizeWorkspaceName(name)
        if err != nil {
                return m, err
        }
        dir, err := store.WorkspaceDir(name)
        if err != nil {
                return m, err
        }
        s := store.Store{Dir: dir}
        db, err := s.Load()
        if err != nil {
                return m, err
        }

        // If this workspace has no identity yet, seed it from the current workspace so
        // the user can immediately create projects/items.
        if strings.TrimSpace(db.CurrentActorID) == "" && len(db.Actors) == 0 {
                srcID := (&m).editActorID()
                if srcID != "" && m.db != nil {
                        if a, ok := m.db.FindActor(srcID); ok && a != nil {
                                db.Actors = append(db.Actors, *a)
                                db.CurrentActorID = srcID
                                if err := s.AppendEvent(srcID, "identity.seed", srcID, map[string]any{
                                        "fromWorkspace": strings.TrimSpace(m.workspace),
                                        "toWorkspace":   name,
                                        "ts":            time.Now().UTC(),
                                }); err != nil {
                                        return m, err
                                }
                                if err := s.Save(db); err != nil {
                                        return m, err
                                }
                        }
                }
        }

        if cfg, err := store.LoadConfig(); err == nil {
                if cfg.Workspaces != nil {
                        if ref, ok := cfg.Workspaces[name]; ok {
                                ref.LastOpened = time.Now().UTC().Format(time.RFC3339Nano)
                                cfg.Workspaces[name] = ref
                        }
                }
                cfg.CurrentWorkspace = name
                if err := store.SaveConfig(cfg); err != nil {
                        return m, err
                }
        }

        nm := newAppModelWithWorkspace(dir, db, name)
        nm.width = m.width
        nm.height = m.height
        nm.seenWindowSize = m.seenWindowSize
        return nm, nil
}

func (m appModel) renameWorkspaceTo(oldName, newName string) (appModel, error) {
        oldName, err := store.NormalizeWorkspaceName(oldName)
        if err != nil {
                return m, err
        }
        newName, err = store.NormalizeWorkspaceName(newName)
        if err != nil {
                return m, err
        }
        if oldName == newName {
                return m.switchWorkspaceTo(newName)
        }

        cfg, err := store.LoadConfig()
        if err != nil {
                return m, err
        }
        if strings.TrimSpace(cfg.CurrentWorkspace) == "" {
                cfg.CurrentWorkspace = "default"
        }

        ref, hasRef := store.WorkspaceRef{}, false
        if cfg.Workspaces != nil {
                ref, hasRef = cfg.Workspaces[oldName]
        }
        isRegistered := hasRef && strings.TrimSpace(ref.Path) != ""

        // For legacy workspaces (not registered), rename the directory on disk.
        // For registered workspaces, renaming is logical only (the directory path stays the same).
        if !isRegistered {
                oldDir, err := store.LegacyWorkspaceDir(oldName)
                if err != nil {
                        return m, err
                }
                newDir, err := store.LegacyWorkspaceDir(newName)
                if err != nil {
                        return m, err
                }
                if err := os.Rename(oldDir, newDir); err != nil {
                        return m, err
                }
        }

        // Update registry entry (if present).
        if cfg.Workspaces != nil && hasRef {
                delete(cfg.Workspaces, oldName)
                cfg.Workspaces[newName] = ref
        }
        if cfg.CurrentWorkspace == oldName {
                cfg.CurrentWorkspace = newName
        }
        if err := store.SaveConfig(cfg); err != nil {
                return m, err
        }

        return m.switchWorkspaceTo(newName)
}

func (m *appModel) openOutlineStatusDefsEditor(outline model.Outline, selectStatusID string) {
        if m == nil {
                return
        }
        oid := strings.TrimSpace(outline.ID)
        if oid == "" {
                return
        }

        items := make([]list.Item, 0, len(outline.StatusDefs))
        for _, def := range outline.StatusDefs {
                items = append(items, outlineStatusDefItem{def: def})
        }
        m.outlineStatusDefsList.Title = ""
        m.outlineStatusDefsList.SetItems(items)

        // Size similarly to the pickers, but allow more height.
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
        h := len(items) + 2
        if h > 18 {
                h = 18
        }
        if h < 8 {
                h = 8
        }
        m.outlineStatusDefsList.SetSize(modalW-6, h)

        // Preselect.
        selected := 0
        if strings.TrimSpace(selectStatusID) != "" {
                for i := 0; i < len(items); i++ {
                        if it, ok := items[i].(outlineStatusDefItem); ok && strings.TrimSpace(it.def.ID) == strings.TrimSpace(selectStatusID) {
                                selected = i
                                break
                        }
                }
        }
        m.outlineStatusDefsList.Select(selected)

        m.modal = modalEditOutlineStatuses
        m.modalForID = oid
        m.modalForKey = ""
}

func (m *appModel) refreshOutlineStatusDefsEditor(outlineID, selectStatusID string) {
        if m == nil {
                return
        }
        oid := strings.TrimSpace(outlineID)
        if oid == "" {
                return
        }
        if m.db == nil {
                return
        }
        if o, ok := m.db.FindOutline(oid); ok && o != nil {
                m.selectedOutline = o
                m.openOutlineStatusDefsEditor(*o, selectStatusID)
        }
}

func (m *appModel) addOutlineStatusDef(outlineID, label string, end bool) (string, error) {
        label = strings.TrimSpace(label)
        if label == "" {
                return "", errors.New("missing label")
        }
        createdID := ""
        err := m.mutateOutline(outlineID, func(db *store.DB, o *model.Outline) (bool, outlineMutationResult, error) {
                for _, def := range o.StatusDefs {
                        if strings.TrimSpace(def.Label) == label {
                                return false, outlineMutationResult{}, errors.New("status label already exists on this outline")
                        }
                }
                id := store.NewStatusIDFromLabel(o, label)
                createdID = id
                o.StatusDefs = append(o.StatusDefs, model.OutlineStatusDef{ID: id, Label: label, IsEndState: end})
                return true, outlineMutationResult{
                        eventType:    "outline.status.add",
                        eventPayload: map[string]any{"id": id, "label": label, "isEndState": end},
                        minibuffer:   "Added status " + id,
                }, nil
        })
        return createdID, err
}

func (m *appModel) renameOutlineStatusDef(outlineID, statusID, label string) error {
        label = strings.TrimSpace(label)
        if label == "" {
                return errors.New("missing label")
        }
        statusID = strings.TrimSpace(statusID)
        if statusID == "" {
                return errors.New("missing status id")
        }
        return m.mutateOutline(outlineID, func(db *store.DB, o *model.Outline) (bool, outlineMutationResult, error) {
                for _, def := range o.StatusDefs {
                        if strings.TrimSpace(def.Label) == label {
                                return false, outlineMutationResult{}, errors.New("status label already exists on this outline")
                        }
                }
                for i := range o.StatusDefs {
                        if strings.TrimSpace(o.StatusDefs[i].ID) != statusID {
                                continue
                        }
                        if strings.TrimSpace(o.StatusDefs[i].Label) == label {
                                return false, outlineMutationResult{}, nil
                        }
                        o.StatusDefs[i].Label = label
                        return true, outlineMutationResult{
                                eventType:    "outline.status.update",
                                eventPayload: map[string]any{"id": statusID, "label": label, "ts": time.Now().UTC()},
                                minibuffer:   "Renamed status " + statusID,
                        }, nil
                }
                return false, outlineMutationResult{}, errors.New("status not found")
        })
}

func (m *appModel) toggleOutlineStatusEndState(outlineID, statusID string) error {
        statusID = strings.TrimSpace(statusID)
        if statusID == "" {
                return errors.New("missing status id")
        }
        return m.mutateOutline(outlineID, func(db *store.DB, o *model.Outline) (bool, outlineMutationResult, error) {
                for i := range o.StatusDefs {
                        if strings.TrimSpace(o.StatusDefs[i].ID) != statusID {
                                continue
                        }
                        o.StatusDefs[i].IsEndState = !o.StatusDefs[i].IsEndState
                        return true, outlineMutationResult{
                                eventType:    "outline.status.update",
                                eventPayload: map[string]any{"id": statusID, "isEndState": o.StatusDefs[i].IsEndState, "ts": time.Now().UTC()},
                                minibuffer:   "Updated status " + statusID,
                        }, nil
                }
                return false, outlineMutationResult{}, errors.New("status not found")
        })
}

func (m *appModel) toggleOutlineStatusRequiresNote(outlineID, statusID string) error {
        statusID = strings.TrimSpace(statusID)
        if statusID == "" {
                return errors.New("missing status id")
        }
        return m.mutateOutline(outlineID, func(db *store.DB, o *model.Outline) (bool, outlineMutationResult, error) {
                for i := range o.StatusDefs {
                        if strings.TrimSpace(o.StatusDefs[i].ID) != statusID {
                                continue
                        }
                        o.StatusDefs[i].RequiresNote = !o.StatusDefs[i].RequiresNote
                        return true, outlineMutationResult{
                                eventType:    "outline.status.update",
                                eventPayload: map[string]any{"id": statusID, "requiresNote": o.StatusDefs[i].RequiresNote, "ts": time.Now().UTC()},
                                minibuffer:   "Updated status " + statusID,
                        }, nil
                }
                return false, outlineMutationResult{}, errors.New("status not found")
        })
}

func (m *appModel) removeOutlineStatusDef(outlineID, statusID string) error {
        statusID = strings.TrimSpace(statusID)
        if statusID == "" {
                return errors.New("missing status id")
        }
        return m.mutateOutline(outlineID, func(db *store.DB, o *model.Outline) (bool, outlineMutationResult, error) {
                // Block removal if any item uses it.
                for _, it := range db.Items {
                        if strings.TrimSpace(it.OutlineID) == strings.TrimSpace(o.ID) && strings.TrimSpace(it.StatusID) == statusID {
                                return false, outlineMutationResult{}, errors.New("cannot remove status: in use by items")
                        }
                }
                next := make([]model.OutlineStatusDef, 0, len(o.StatusDefs))
                removed := false
                for _, def := range o.StatusDefs {
                        if strings.TrimSpace(def.ID) == statusID {
                                removed = true
                                continue
                        }
                        next = append(next, def)
                }
                if !removed {
                        return false, outlineMutationResult{}, errors.New("status not found")
                }
                o.StatusDefs = next
                return true, outlineMutationResult{
                        eventType:    "outline.status.remove",
                        eventPayload: map[string]any{"id": statusID},
                        minibuffer:   "Removed status " + statusID,
                }, nil
        })
}

func (m *appModel) moveOutlineStatusDef(outlineID, statusID string, delta int) error {
        statusID = strings.TrimSpace(statusID)
        if statusID == "" || delta == 0 {
                return nil
        }
        return m.mutateOutline(outlineID, func(db *store.DB, o *model.Outline) (bool, outlineMutationResult, error) {
                idx := -1
                for i := range o.StatusDefs {
                        if strings.TrimSpace(o.StatusDefs[i].ID) == statusID {
                                idx = i
                                break
                        }
                }
                if idx < 0 {
                        return false, outlineMutationResult{}, errors.New("status not found")
                }
                nextIdx := idx + delta
                if nextIdx < 0 || nextIdx >= len(o.StatusDefs) {
                        return false, outlineMutationResult{}, nil
                }
                defs := o.StatusDefs
                defs[idx], defs[nextIdx] = defs[nextIdx], defs[idx]
                o.StatusDefs = defs
                labels := make([]string, 0, len(o.StatusDefs))
                for _, d := range o.StatusDefs {
                        labels = append(labels, d.Label)
                }
                return true, outlineMutationResult{
                        eventType:    "outline.status.reorder",
                        eventPayload: map[string]any{"labels": labels},
                        minibuffer:   "Reordered statuses",
                }, nil
        })
}

func (m *appModel) openMoveOutlinePicker(itemID string) {
        itemID = strings.TrimSpace(itemID)
        if itemID == "" || m == nil || m.db == nil {
                return
        }
        it, ok := m.db.FindItem(itemID)
        if !ok {
                return
        }
        opts := []list.Item{}
        for _, o := range m.db.Outlines {
                if o.Archived {
                        continue
                }
                opts = append(opts, outlineItem{outline: o})
        }
        if len(opts) <= 1 {
                m.showMinibuffer("No other outlines")
                return
        }

        m.outlinePickList.Title = ""
        m.outlinePickList.SetItems(opts)

        // Size the picker similarly to the status picker, but allow a bit more height.
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
        if h > 18 {
                h = 18
        }
        if h < 6 {
                h = 6
        }
        m.outlinePickList.SetSize(modalW-6, h)

        // Preselect current outline.
        selected := 0
        for i := 0; i < len(opts); i++ {
                if oi, ok := opts[i].(outlineItem); ok && strings.TrimSpace(oi.outline.ID) == strings.TrimSpace(it.OutlineID) {
                        selected = i
                        break
                }
        }
        m.outlinePickList.Select(selected)

        m.pendingMoveOutlineTo = ""
        m.modal = modalPickOutline
        m.modalForID = itemID
}

func (m *appModel) moveItemToOutline(itemID, toOutlineID, statusOverride string, applyStatusToInvalidSubtree bool) error {
        itemID = strings.TrimSpace(itemID)
        toOutlineID = strings.TrimSpace(toOutlineID)
        statusOverride = strings.TrimSpace(statusOverride)
        if itemID == "" || toOutlineID == "" {
                return nil
        }

        err := m.mutateItem(itemID, func(db *store.DB, it *model.Item) (bool, itemMutationResult, error) {
                o, ok := db.FindOutline(toOutlineID)
                if !ok {
                        return false, itemMutationResult{}, errors.New("outline not found")
                }

                actorID := m.editActorID()
                if actorID == "" {
                        return false, itemMutationResult{}, errors.New("no current actor")
                }

                // If the caller wants to apply a chosen status to invalid subtree items, validate it first.
                if applyStatusToInvalidSubtree {
                        if statusOverride == "" {
                                return false, itemMutationResult{}, errors.New("missing status")
                        }
                        if !statusutil.ValidateStatusID(*o, statusOverride) {
                                return false, itemMutationResult{}, errors.New("invalid status id for target outline")
                        }
                }

                // Collect the subtree (root + descendants). We must move children too to avoid cross-outline parent links.
                ids := collectSubtreeItemIDs(db, it.ID)
                if len(ids) == 0 {
                        return false, itemMutationResult{}, nil
                }

                // Permission check: all items in the subtree must be editable by the current actor.
                for _, id := range ids {
                        x, ok := db.FindItem(id)
                        if !ok {
                                continue
                        }
                        if !canEditItem(db, actorID, x) {
                                return false, itemMutationResult{}, errors.New("permission denied")
                        }
                }

                changed := false
                now := time.Now().UTC()

                // Move every item in the subtree.
                for _, id := range ids {
                        x, ok := db.FindItem(id)
                        if !ok {
                                continue
                        }

                        // Determine status to use:
                        // - root item: use override if provided, else keep.
                        // - descendants: keep current unless invalid; if invalid and applyStatusToInvalidSubtree=true, apply override.
                        nextStatus := strings.TrimSpace(x.StatusID)
                        if id == it.ID && statusOverride != "" {
                                nextStatus = statusOverride
                        }
                        if nextStatus == "" || !statusutil.ValidateStatusID(*o, nextStatus) {
                                if applyStatusToInvalidSubtree {
                                        nextStatus = statusOverride
                                } else {
                                        return false, itemMutationResult{}, errors.New("invalid status id for target outline; pick a compatible status")
                                }
                        }

                        if strings.TrimSpace(x.OutlineID) != strings.TrimSpace(o.ID) {
                                x.OutlineID = o.ID
                                changed = true
                        }
                        if strings.TrimSpace(x.ProjectID) != strings.TrimSpace(o.ProjectID) {
                                x.ProjectID = o.ProjectID
                                changed = true
                        }
                        if strings.TrimSpace(x.StatusID) != strings.TrimSpace(nextStatus) {
                                x.StatusID = nextStatus
                                changed = true
                        }
                        if !x.UpdatedAt.Equal(now) {
                                x.UpdatedAt = now
                                changed = true
                        }
                }

                // Root-specific adjustments: detach and re-rank under destination root.
                if it.ParentID != nil {
                        it.ParentID = nil
                        changed = true
                }
                nextRank := nextSiblingRank(db, o.ID, nil)
                if strings.TrimSpace(it.Rank) != strings.TrimSpace(nextRank) {
                        it.Rank = nextRank
                        changed = true
                }

                if !changed {
                        return false, itemMutationResult{}, nil
                }

                name := "(unnamed outline)"
                if o.Name != nil && strings.TrimSpace(*o.Name) != "" {
                        name = strings.TrimSpace(*o.Name)
                }

                return true, itemMutationResult{
                        eventType:    "item.move_outline",
                        eventPayload: map[string]any{"to": o.ID, "status": it.StatusID},
                        minibuffer:   fmt.Sprintf("Moved %d item(s) to outline: %s", len(ids), name),
                }, nil
        })
        if err != nil {
                return err
        }

        // If the moved item is currently open, keep the item-view context consistent.
        if m.view == viewItem && strings.TrimSpace(m.openItemID) == itemID {
                m.selectedOutlineID = toOutlineID
                if m.db != nil {
                        if o, ok := m.db.FindOutline(toOutlineID); ok {
                                m.selectedOutline = o
                        }
                }
        }

        return nil
}

func collectSubtreeItemIDs(db *store.DB, rootID string) []string {
        rootID = strings.TrimSpace(rootID)
        if db == nil || rootID == "" {
                return nil
        }
        out := []string{}
        seen := map[string]bool{}
        var walk func(id string)
        walk = func(id string) {
                id = strings.TrimSpace(id)
                if id == "" || seen[id] {
                        return
                }
                seen[id] = true
                out = append(out, id)
                for _, ch := range db.ChildrenOf(id) {
                        walk(ch.ID)
                }
        }
        walk(rootID)
        return out
}

func subtreeHasInvalidStatusInOutline(db *store.DB, rootID, outlineID string) bool {
        rootID = strings.TrimSpace(rootID)
        outlineID = strings.TrimSpace(outlineID)
        if db == nil || rootID == "" || outlineID == "" {
                return false
        }
        o, ok := db.FindOutline(outlineID)
        if !ok || o == nil {
                return true
        }
        ids := collectSubtreeItemIDs(db, rootID)
        for _, id := range ids {
                it, ok := db.FindItem(id)
                if !ok {
                        continue
                }
                sid := strings.TrimSpace(it.StatusID)
                if sid == "" || !statusutil.ValidateStatusID(*o, sid) {
                        return true
                }
        }
        return false
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
        nextStatus := opts[next]
        if statusutil.IsEndState(outline, nextStatus) {
                if reason := explainCompletionBlockers(m.db, itemID); strings.TrimSpace(reason) != "" {
                        return completionBlockedError{taskID: itemID, reason: reason}
                }
        }
        if statusutil.RequiresNote(outline, nextStatus) {
                m.openTextModal(modalStatusNote, itemID, "Status note…", "")
                m.modalForKey = strings.TrimSpace(nextStatus)
                return nil
        }
        return m.setStatusForItem(itemID, nextStatus)
}

func (m *appModel) setStatusForItem(itemID, statusID string) error {
        return m.setStatusForItemWithNote(itemID, statusID, nil)
}

func (m *appModel) setStatusForItemWithNote(itemID, statusID string, note *string) error {
        itemID = strings.TrimSpace(itemID)
        statusID = strings.TrimSpace(statusID)
        return m.mutateItem(itemID, func(db *store.DB, it *model.Item) (bool, itemMutationResult, error) {
                var outline model.Outline
                if o, ok := db.FindOutline(strings.TrimSpace(it.OutlineID)); ok && o != nil {
                        outline = *o
                }

                // If setting an end-state status, ensure we can complete (no incomplete children/deps).
                if statusutil.IsEndState(outline, statusID) {
                        if reason := explainCompletionBlockers(db, it.ID); strings.TrimSpace(reason) != "" {
                                return false, itemMutationResult{}, completionBlockedError{taskID: it.ID, reason: reason}
                        }
                }
                actorID := m.editActorID()
                if actorID == "" {
                        return false, itemMutationResult{}, errors.New("no current actor")
                }
                res, err := mutate.SetItemStatus(db, actorID, it.ID, statusID, note)
                if err != nil {
                        if err == mutate.ErrInvalidStatus {
                                return false, itemMutationResult{}, errors.New("invalid status")
                        }
                        if err == mutate.ErrStatusNoteRequired {
                                return false, itemMutationResult{}, errors.New("status requires a note")
                        }
                        switch err.(type) {
                        case mutate.OwnerOnlyError:
                                return false, itemMutationResult{}, errors.New("permission denied")
                        default:
                                return false, itemMutationResult{}, err
                        }
                }
                if !res.Changed {
                        return false, itemMutationResult{}, nil
                }

                msg := "Status: "
                if statusID == "" {
                        msg += "(none)"
                } else if o, ok := db.FindOutline(it.OutlineID); ok {
                        lbl := strings.TrimSpace(statusLabel(*o, statusID))
                        if lbl != "" {
                                msg += strings.ToUpper(lbl)
                        } else {
                                msg += statusID
                        }
                } else {
                        msg += statusID
                }

                return true, itemMutationResult{
                        eventType:    "item.set_status",
                        eventPayload: res.EventPayload,
                        minibuffer:   msg,
                }, nil
        })
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
        afterID = strings.TrimSpace(afterID)
        beforeID = strings.TrimSpace(beforeID)
        if (afterID == "" && beforeID == "") || (afterID != "" && beforeID != "") {
                return nil
        }

        // Build the current sibling order (includes t). This ordering must match the rendered order.
        full := siblingItems(m.db, t.OutlineID, t.ParentID)
        full = filterItems(full, func(x *model.Item) bool { return !x.Archived })

        // Compute insert position in the "after removing t" coordinate system.
        rest := filterItems(full, func(x *model.Item) bool { return x.ID != t.ID })
        refID := beforeID
        mode := "before"
        if afterID != "" {
                refID = afterID
                mode = "after"
        }
        refIdx := indexOfItem(rest, refID)
        if refIdx < 0 {
                return nil
        }
        insertAt := refIdx
        if mode == "after" {
                insertAt = refIdx + 1
        }

        res, err := store.PlanReorderRanks(full, t.ID, insertAt)
        if err != nil {
                return err
        }
        if len(res.RankByID) == 0 {
                return nil
        }

        now := time.Now().UTC()
        for id, r := range res.RankByID {
                it, ok := m.db.FindItem(id)
                if !ok {
                        continue
                }
                if strings.TrimSpace(it.Rank) == strings.TrimSpace(r) {
                        continue
                }
                it.Rank = r
                it.UpdatedAt = now
        }

        // Single event, even if we had to rebalance a local window.
        actorID := m.currentWriteActorID()
        payload := map[string]any{"before": beforeID, "after": afterID, "rank": strings.TrimSpace(t.Rank)}
        if res.UsedFallback && len(res.RankByID) > 1 {
                rebalance := map[string]string{}
                for id, r := range res.RankByID {
                        if id == t.ID {
                                continue
                        }
                        rebalance[id] = r
                }
                if len(rebalance) > 0 {
                        payload["rebalance"] = rebalance
                        payload["rebalanceCount"] = len(rebalance)
                }
        }
        if err := m.appendEvent(actorID, "item.move", t.ID, payload); err != nil {
                return err
        }
        if err := m.store.Save(m.db); err != nil {
                return err
        }

        m.refreshEventsTail()
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
        if err := m.appendEvent(actorID, "item.set_parent", t.ID, map[string]any{"parent": newParentID, "rank": t.Rank}); err != nil {
                return err
        }
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        m.refreshEventsTail()
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
        payload := map[string]any{"rank": t.Rank}
        if destParentID == nil {
                payload["parent"] = "none"
        } else {
                payload["parent"] = *destParentID
        }
        if err := m.appendEvent(actorID, "item.set_parent", t.ID, payload); err != nil {
                return err
        }
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        m.refreshEventsTail()
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

func (m *appModel) openTextModal(kind modalKind, itemID, placeholder, initial string) {
        if strings.TrimSpace(itemID) == "" {
                return
        }
        m.modal = kind
        m.modalForID = itemID
        m.textFocus = textFocusBody
        bodyW := modalBodyWidth(m.width)

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
        m.textarea.SetValue(initial)
        m.textarea.Focus()
}

func (m *appModel) openInputModal(kind modalKind, forID, placeholder, initial string) {
        m.modal = kind
        m.modalForID = strings.TrimSpace(forID)
        m.textFocus = textFocusBody

        // Size to modal body width so the input fills the dialog.
        bodyW := modalBodyWidth(m.width)
        inputW := bodyW - 2 // input "surface" has horizontal padding
        if inputW < 10 {
                inputW = 10
        }
        m.input.Width = inputW

        // Make the input visually distinct from the modal background.
        st := lipgloss.NewStyle().Foreground(colorSurfaceFg).Background(colorInputBg)
        m.input.Prompt = ""
        m.input.PromptStyle = st
        m.input.TextStyle = st
        m.input.PlaceholderStyle = styleMuted().Background(colorInputBg)
        m.input.CursorStyle = lipgloss.NewStyle().Foreground(colorSelectedFg).Background(colorAccent)

        m.input.Placeholder = placeholder
        m.input.SetValue(initial)
        m.input.Focus()
}

func (m *appModel) openDateModal(kind modalKind, itemID string, initial *model.DateTime) {
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return
        }
        m.modal = kind
        m.modalForID = itemID
        m.dateFocus = dateFocusYear
        m.timeEnabled = initial != nil && initial.Time != nil && strings.TrimSpace(*initial.Time) != ""

        // Seed values from existing field (if any); default to today.
        y, mo, d, h, mi := parseDateTimeFieldsOrNow(initial)
        m.yearInput.SetValue(fmt.Sprintf("%04d", y))
        m.monthInput.SetValue(fmt.Sprintf("%02d", mo))
        m.dayInput.SetValue(fmt.Sprintf("%02d", d))
        if m.timeEnabled {
                m.hourInput.SetValue(fmt.Sprintf("%02d", h))
                m.minuteInput.SetValue(fmt.Sprintf("%02d", mi))
        } else {
                // No time semantics.
                m.hourInput.SetValue("")
                m.minuteInput.SetValue("")
        }

        // Style inputs similarly to other modals.
        st := lipgloss.NewStyle().Foreground(colorSurfaceFg).Background(colorInputBg)
        for _, in := range []*textinput.Model{&m.yearInput, &m.monthInput, &m.dayInput, &m.hourInput, &m.minuteInput} {
                in.Prompt = ""
                in.PromptStyle = st
                in.TextStyle = st
                in.PlaceholderStyle = styleMuted().Background(colorInputBg)
                in.CursorStyle = lipgloss.NewStyle().Foreground(colorSelectedFg).Background(colorAccent)
        }

        // Focus day by default (clear cursor elsewhere).
        m.yearInput.Blur()
        m.monthInput.Blur()
        m.dayInput.Focus()
        m.hourInput.Blur()
        m.minuteInput.Blur()
}

func (m *appModel) jumpToItemByID(itemID string) error {
        itemID = normalizeJumpItemID(itemID)
        if m == nil || m.db == nil || itemID == "" {
                return nil
        }
        it, ok := m.db.FindItem(itemID)
        if !ok || it == nil || it.Archived {
                return fmt.Errorf("item not found")
        }

        // Preserve a return path when jumping from agenda.
        if m.view == viewAgenda {
                m.hasReturnView = true
                m.returnView = viewAgenda
        }

        m.selectedProjectID = strings.TrimSpace(it.ProjectID)
        m.selectedOutlineID = strings.TrimSpace(it.OutlineID)

        // Refresh lists so selection exists.
        if m.selectedProjectID != "" {
                m.refreshOutlines(m.selectedProjectID)
                selectListItemByID(&m.outlinesList, m.selectedOutlineID)
        }

        if ol, ok := m.db.FindOutline(m.selectedOutlineID); ok && ol != nil {
                m.selectedOutline = ol
                m.refreshItems(*ol)
                // Clear any active outline filter so selection is predictable.
                if m.itemsList.SettingFilter() || m.itemsList.IsFiltered() {
                        m.itemsList.ResetFilter()
                }
                selectListItemByID(&m.itemsList, itemID)
        }

        m.view = viewItem
        m.openItemID = itemID
        m.itemArchivedReadOnly = false
        m.recordRecentItemVisit(m.openItemID)
        m.itemFocus = itemFocusTitle
        m.itemCommentIdx = 0
        m.itemWorklogIdx = 0
        m.itemHistoryIdx = 0
        m.itemSideScroll = 0
        m.itemChildIdx = 0
        m.itemChildOff = 0
        return nil
}

func normalizeJumpItemID(s string) string {
        s = strings.TrimSpace(s)
        if s == "" {
                return ""
        }
        if strings.HasPrefix(s, "item-") {
                return s
        }
        // If it already looks like an id with a prefix, keep it as-is.
        if strings.Contains(s, "-") {
                return s
        }
        return "item-" + s
}

func (m *appModel) addComment(itemID, body string, replyToCommentID *string) (string, error) {
        itemID = strings.TrimSpace(itemID)
        body = strings.TrimSpace(body)
        if itemID == "" || body == "" {
                return "", nil
        }
        actorID := m.currentWriteActorID()
        if actorID == "" {
                return "", nil
        }

        db, err := m.store.Load()
        if err != nil {
                return "", err
        }
        m.db = db

        if _, ok := m.db.FindItem(itemID); !ok {
                return "", nil
        }

        var replyPtr *string
        if replyToCommentID != nil {
                if v := strings.TrimSpace(*replyToCommentID); v != "" {
                        replyPtr = &v
                }
        }

        c := model.Comment{
                ID:               m.store.NextID(m.db, "cmt"),
                ItemID:           itemID,
                AuthorID:         actorID,
                ReplyToCommentID: replyPtr,
                Body:             body,
                CreatedAt:        time.Now().UTC(),
        }
        m.db.Comments = append(m.db.Comments, c)
        if err := m.appendEvent(actorID, "comment.add", c.ID, c); err != nil {
                return "", err
        }
        if err := m.store.Save(m.db); err != nil {
                return "", err
        }
        m.refreshEventsTail()
        m.captureStoreModTimes()

        // If we're currently viewing this item, keep the comments panel selection on the new entry.
        if strings.TrimSpace(m.openItemID) == itemID {
                rows := buildCommentThreadRows(m.db.CommentsForItem(itemID))
                m.itemCommentIdx = indexOfCommentRow(rows, c.ID)
                m.itemSideScroll = 0
        }

        if m.selectedOutline != nil {
                if o, ok := m.db.FindOutline(m.selectedOutline.ID); ok {
                        m.selectedOutline = o
                }
                m.refreshItems(*m.selectedOutline)
                selectListItemByID(&m.itemsList, itemID)
        }
        m.showMinibuffer("Comment added")
        return c.ID, nil
}

func (m *appModel) addWorklog(itemID, body string) error {
        itemID = strings.TrimSpace(itemID)
        body = strings.TrimSpace(body)
        if itemID == "" || body == "" {
                return nil
        }
        actorID := m.currentWriteActorID()
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
        if err := m.appendEvent(actorID, "worklog.add", w.ID, w); err != nil {
                return err
        }
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        m.refreshEventsTail()
        m.captureStoreModTimes()

        // If we're currently viewing this item, keep the worklog panel pinned to the newest entry.
        if strings.TrimSpace(m.openItemID) == itemID {
                m.itemWorklogIdx = 0
                m.itemSideScroll = 0
        }

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
