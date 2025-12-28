package tui

import (
        "os"
        "strconv"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/bubbles/list"
        "github.com/charmbracelet/bubbles/textarea"
        "github.com/charmbracelet/bubbles/textinput"
)

type appModel struct {
        dir       string
        workspace string
        store     store.Store
        db        *store.DB
        // eventsTail caches the last N events from events.jsonl for cheap "recent history" rendering.
        eventsTail []model.Event

        width  int
        height int

        // We treat the very first WindowSizeMsg as "initial sizing" rather than a user-driven
        // resize. Otherwise we briefly render the full-height "Resizing…" overlay on startup.
        seenWindowSize bool

        view view

        projectsList    list.Model
        outlinesList    list.Model
        itemsList       list.Model
        statusList      list.Model
        outlinePickList list.Model
        assigneeList    list.Model
        tagsList        list.Model
        tagsListActive  *bool
        workspaceList   list.Model
        agendaList      list.Model
        archivedList    list.Model
        // outlineStatusDefsList is used in the outline statuses editor modal.
        outlineStatusDefsList list.Model

        captureTemplatesList         list.Model
        captureTemplateWorkspaceList list.Model
        captureTemplateOutlineList   list.Model
        captureTemplateEdit          *captureTemplateEditState
        captureTemplateDeleteIdx     int

        selectedProjectID string
        selectedOutlineID string
        selectedOutline   *model.Outline

        pane        pane
        showPreview bool
        openItemID  string
        // recentItemIDs stores most-recently-visited item ids (full item view only), newest first.
        recentItemIDs         []string
        returnView            view
        hasReturnView         bool
        agendaReturnView      view
        hasAgendaReturnView   bool
        archivedReturnView    view
        hasArchivedReturnView bool
        agendaCollapsed       map[string]bool
        collapsed             map[string]bool
        collapseInitialized   bool
        // itemFocus is used on the full-screen item view to allow Tab navigation across
        // editable fields (title/status/description/comment/worklog).
        itemFocus            itemPageFocus
        itemCommentIdx       int
        itemWorklogIdx       int
        itemHistoryIdx       int
        itemSideScroll       int
        itemDetailScroll     int
        itemChildIdx         int
        itemChildOff         int
        itemNavStack         []itemNavEntry
        itemArchivedReadOnly bool
        // Per-outline display mode for the outline view (experimental).
        outlineViewMode map[string]outlineViewMode
        // Per-outline selection state for columns mode.
        columnsSel map[string]outlineColumnsSelection

        modal       modalKind
        modalForID  string
        modalForKey string
        archiveFor  archiveTarget
        input       textinput.Model
        textarea    textarea.Model
        textFocus   textModalFocus
        // Date modal inputs (due/schedule).
        yearInput    textinput.Model
        monthInput   textinput.Model
        dayInput     textinput.Model
        hourInput    textinput.Model
        minuteInput  textinput.Model
        dateFocus    dateModalFocus
        tagsFocus    tagsModalFocus
        timeEnabled  bool
        replyQuoteMD string

        // externalEditorPath is the temp file used when opening the current textarea
        // content in $VISUAL/$EDITOR.
        externalEditorPath   string
        externalEditorBefore string

        // pendingMoveOutlineTo is set when a move-outline flow needs the user to pick a status
        // compatible with the target outline. While set, the status picker "enter" applies the move.
        pendingMoveOutlineTo string

        actionPanelStack []actionPanelKind
        // actionPanelSelectedKey is the current selection in the action panel (for tab/enter navigation).
        actionPanelSelectedKey string
        // captureKeySeq stores the currently-entered org-capture style key sequence while in the Capture panel.
        captureKeySeq []string

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
        inputDbg     inputDebug

        lastDBModTime     time.Time
        lastEventsModTime time.Time

        minibufferText string
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
        return newAppModelWithWorkspace(dir, db, "")
}

func quoteArgIfNeeded(s string) string {
        s = strings.TrimSpace(s)
        if s == "" {
                return ""
        }
        // For copied commands, prefer a shell-safe representation when whitespace is present.
        // strconv.Quote uses double-quotes + escapes, which is widely portable.
        if strings.ContainsAny(s, " \t\r\n") {
                return strconv.Quote(s)
        }
        return s
}

func (m appModel) clipboardItemRef(itemID string) string {
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return ""
        }
        ws := strings.TrimSpace(m.workspace)
        if ws == "" {
                return itemID
        }
        return itemID + " --workspace " + quoteArgIfNeeded(ws)
}

func (m appModel) clipboardShowCmd(itemID string) string {
        ref := m.clipboardItemRef(itemID)
        if ref == "" {
                return ""
        }
        return "clarity items show " + ref
}

func newAppModelWithWorkspace(dir string, db *store.DB, workspace string) appModel {
        s := store.Store{Dir: dir}
        m := appModel{
                dir:       dir,
                workspace: strings.TrimSpace(workspace),
                store:     s,
                db:        db,
                view:      viewProjects,
                pane:      paneOutline,
        }
        m.columnsSel = map[string]outlineColumnsSelection{}
        m.tagsListActive = new(bool)

        if strings.TrimSpace(os.Getenv("CLARITY_TUI_DEBUG")) != "" {
                m.debugEnabled = true
                m.debugOverlay = true
        }
        m.debugLogPath = strings.TrimSpace(os.Getenv("CLARITY_TUI_DEBUG_LOG"))

        m.projectsList = newList("Projects", "Select a project", []list.Item{})
        m.projectsList.SetDelegate(newCompactItemDelegate())
        m.outlinesList = newList("Outlines", "Select an outline", []list.Item{})
        m.outlinesList.SetDelegate(newCompactItemDelegate())
        m.itemsList = newList("Outline", "Go to items (split view)", []list.Item{})
        m.itemsList.SetDelegate(newOutlineItemDelegate())
        // Enable "/" filtering to quickly scope down large outlines.
        m.itemsList.SetFilteringEnabled(true)
        m.itemsList.SetShowFilter(true)

        m.agendaList = newList("Agenda", "All items (excluding DONE and on hold)", []list.Item{})
        m.agendaList.SetDelegate(newCompactItemDelegate())
        m.agendaCollapsed = map[string]bool{}

        m.archivedList = newList("Archived", "Archived content", []list.Item{})
        m.archivedList.SetDelegate(newCompactItemDelegate())

        m.statusList = newList("Status", "Select a status", []list.Item{})
        m.statusList.SetDelegate(newCompactItemDelegate())
        m.statusList.SetFilteringEnabled(false)
        m.statusList.SetShowFilter(false)
        m.statusList.SetShowHelp(false)
        m.statusList.SetShowStatusBar(false)
        m.statusList.SetShowPagination(false)

        m.outlinePickList = newList("Outlines", "Select an outline", []list.Item{})
        m.outlinePickList.SetDelegate(newCompactItemDelegate())
        // Keep list chrome minimal inside the modal.
        m.outlinePickList.SetShowHelp(false)
        m.outlinePickList.SetShowStatusBar(false)
        m.outlinePickList.SetShowPagination(false)

        m.assigneeList = newList("Assignee", "Select an assignee", []list.Item{})
        m.assigneeList.SetDelegate(newCompactItemDelegate())
        m.assigneeList.SetFilteringEnabled(false)
        m.assigneeList.SetShowFilter(false)
        m.assigneeList.SetShowHelp(false)
        m.assigneeList.SetShowStatusBar(false)
        m.assigneeList.SetShowPagination(false)

        m.tagsList = newList("Tags", "Edit tags", []list.Item{})
        m.tagsList.SetDelegate(newFocusAwareCompactItemDelegate(m.tagsListActive))
        m.tagsList.SetFilteringEnabled(false)
        m.tagsList.SetShowFilter(false)
        m.tagsList.SetShowHelp(false)
        m.tagsList.SetShowStatusBar(false)
        m.tagsList.SetShowPagination(false)

        m.workspaceList = newList("Workspaces", "Select a workspace", []list.Item{})
        m.workspaceList.SetDelegate(newCompactItemDelegate())
        m.workspaceList.SetFilteringEnabled(true)
        m.workspaceList.SetShowFilter(true)
        // Keep list chrome minimal inside the modal.
        m.workspaceList.SetShowHelp(false)
        m.workspaceList.SetShowStatusBar(false)
        m.workspaceList.SetShowPagination(false)

        m.outlineStatusDefsList = newList("Statuses", "Edit outline statuses", []list.Item{})
        m.outlineStatusDefsList.SetDelegate(newCompactItemDelegate())
        m.outlineStatusDefsList.SetFilteringEnabled(false)
        m.outlineStatusDefsList.SetShowFilter(false)
        m.outlineStatusDefsList.SetShowHelp(false)
        m.outlineStatusDefsList.SetShowStatusBar(false)
        m.outlineStatusDefsList.SetShowPagination(false)

        m.captureTemplatesList = newList("Capture templates", "Manage capture templates", []list.Item{})
        m.captureTemplatesList.SetDelegate(newCompactItemDelegate())
        m.captureTemplatesList.SetFilteringEnabled(true)
        m.captureTemplatesList.SetShowFilter(true)

        m.captureTemplateWorkspaceList = newList("Workspaces", "Select a workspace", []list.Item{})
        m.captureTemplateWorkspaceList.SetDelegate(newCompactItemDelegate())
        m.captureTemplateWorkspaceList.SetFilteringEnabled(true)
        m.captureTemplateWorkspaceList.SetShowFilter(true)
        m.captureTemplateWorkspaceList.SetShowHelp(false)
        m.captureTemplateWorkspaceList.SetShowStatusBar(false)
        m.captureTemplateWorkspaceList.SetShowPagination(false)

        m.captureTemplateOutlineList = newList("Outlines", "Select an outline", []list.Item{})
        m.captureTemplateOutlineList.SetDelegate(newCompactItemDelegate())
        m.captureTemplateOutlineList.SetFilteringEnabled(true)
        m.captureTemplateOutlineList.SetShowFilter(true)
        m.captureTemplateOutlineList.SetShowHelp(false)
        m.captureTemplateOutlineList.SetShowStatusBar(false)
        m.captureTemplateOutlineList.SetShowPagination(false)

        m.captureTemplateDeleteIdx = -1

        m.input = textinput.New()
        m.input.Placeholder = "Title"
        m.input.CharLimit = 200
        m.input.Width = 40

        // Date/time inputs for due/schedule (outline.js-style: date required, time optional).
        m.yearInput = textinput.New()
        m.yearInput.Placeholder = "YYYY"
        m.yearInput.CharLimit = 4
        m.yearInput.Width = 6
        m.monthInput = textinput.New()
        m.monthInput.Placeholder = "MM"
        m.monthInput.CharLimit = 2
        m.monthInput.Width = 4
        m.dayInput = textinput.New()
        m.dayInput.Placeholder = "DD"
        m.dayInput.CharLimit = 2
        m.dayInput.Width = 4
        m.hourInput = textinput.New()
        m.hourInput.Placeholder = "HH"
        m.hourInput.CharLimit = 2
        m.hourInput.Width = 4
        m.minuteInput = textinput.New()
        m.minuteInput.Placeholder = "MM"
        m.minuteInput.CharLimit = 2
        m.minuteInput.Width = 4

        m.textarea = textarea.New()
        m.textarea.Placeholder = "Write…"
        m.textarea.CharLimit = 0
        m.textarea.SetWidth(72)
        m.textarea.SetHeight(10)
        m.textarea.ShowLineNumbers = false

        m.refreshProjects()

        // Best-effort: restore last TUI screen/selection for this workspace.
        if st, err := s.LoadTUIState(); err == nil {
                m.applySavedTUIState(st)
        }

        m.captureStoreModTimes()
        m.refreshEventsTail()
        return m
}
