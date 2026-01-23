package tui

import (
	"os"
	"strconv"
	"strings"
	"time"

	"clarity-cli/internal/gitrepo"
	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

type appModel struct {
	dir       string
	workspace string
	store     store.Store
	// jsonlWorkspace indicates whether this workspace uses the JSONL event log backend
	// (the Git-syncable v1 format with `events/events*.jsonl`).
	jsonlWorkspace bool
	db             *store.DB
	// eventsTail caches the last N events from events.jsonl for cheap "recent history" rendering.
	eventsTail []model.Event

	width  int
	height int

	// We treat the very first WindowSizeMsg as "initial sizing" rather than a user-driven
	// resize. Otherwise we briefly render the full-height "Resizing…" overlay on startup.
	seenWindowSize bool

	view view

	projectsList           list.Model
	outlinesList           list.Model
	projectAttachmentsList list.Model
	itemsList              list.Model
	// itemsListActive controls whether the outline list shows a selection highlight.
	// It allows item view / split panes to show focus in only one place at a time.
	itemsListActive        *bool
	statusList             list.Model
	outlinePickList        list.Model
	assigneeList           list.Model
	tagsList               list.Model
	tagsListActive         *bool
	workspaceList          list.Model
	showArchivedWorkspaces bool
	agendaList             list.Model
	archivedList           list.Model
	// outlineStatusDefsList is used in the outline statuses editor modal.
	outlineStatusDefsList list.Model

	captureTemplatesList              list.Model
	captureTemplateWorkspaceList      list.Model
	captureTemplateOutlineList        list.Model
	captureTemplateEdit               *captureTemplateEditState
	captureTemplateDeleteIdx          int
	captureTemplatePromptsList        list.Model
	captureTemplatePromptTypeList     list.Model
	captureTemplatePromptRequiredList list.Model
	captureTemplatePromptEdit         *captureTemplatePromptEditState
	captureTemplatePromptDeleteIdx    int

	selectedProjectID string
	selectedOutlineID string
	selectedOutline   *model.Outline

	pane        pane
	showPreview bool
	openItemID  string
	// recentItemIDs stores most-recently-visited item ids (full item view only), newest first.
	recentItemIDs []string
	// recentCapturedItemIDs stores most-recently-captured item ids (created via Capture), newest first.
	recentCapturedItemIDs []string
	// Return-state for backing out of the item view to "where we came from".
	// Only populated when hasReturnView is true (best-effort; fields may be empty).
	returnSelectedProjectID string
	returnSelectedOutlineID string
	returnOpenItemID        string
	returnView              view
	hasReturnView           bool
	agendaReturnView        view
	hasAgendaReturnView     bool
	archivedReturnView      view
	hasArchivedReturnView   bool
	agendaCollapsed         map[string]bool
	collapsed               map[string]bool
	// itemFocus is used on the full-screen item view to allow Tab navigation across
	// editable fields (title/status/description/comment/worklog).
	itemFocus itemPageFocus
	// itemListRootID tracks which root item the left "subtree outline" list is currently showing
	// in the item view.
	itemListRootID       string
	itemCollapsed        map[string]bool
	itemAttachmentIdx    int
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

	modal                  modalKind
	modalForID             string
	modalForKey            string
	viewModalTitle         string
	viewModalBody          string
	viewModalScroll        int
	viewModalReturn        modalKind
	activityModalKind      activityModalKind
	activityModalItemID    string
	activityModalList      list.Model
	activityModalCollapsed map[string]bool
	activityModalContentW  int
	// capture holds the embedded capture model when modal == modalCapture.
	capture                       *captureModel
	returnToCaptureAfterTemplates bool
	archiveFor                    archiveTarget
	input                         textinput.Model
	textarea                      textarea.Model
	confirmFocus                  confirmModalFocus
	textFocus                     textModalFocus
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

	attachmentAddKind      string
	attachmentAddEntityID  string
	attachmentAddPath      string
	attachmentAddTitle     string
	attachmentAddAlt       string
	attachmentAddTitleHint string

	attachmentEditID    string
	attachmentEditTitle string
	attachmentEditAlt   string

	// commentDraftAttachments stores queued uploads while composing a comment/reply.
	// These are attached to the new comment upon save.
	commentDraftAttachments []attachmentDraft

	attachmentAddFlow attachmentAddFlow
	// When attachmentAddFlow == attachmentAddFlowCommentDraft, these store where to return after
	// finishing the file/title/alt prompts.
	attachmentAddReturnModal  modalKind
	attachmentAddReturnForID  string
	attachmentAddReturnForKey string

	targetPickList    list.Model
	targetPickTargets []targetPickTarget

	attachmentFilePicker        filepicker.Model
	attachmentFilePickerLastDir string

	// externalEditorPath is the temp file used when opening the current textarea
	// content in $VISUAL/$EDITOR.
	externalEditorPath   string
	externalEditorBefore string

	// externalViewEditorPath is the temp file used when opening a read-only/view
	// body (e.g. an existing comment) in $VISUAL/$EDITOR for copying.
	externalViewEditorPath string

	// pendingMoveOutlineTo is set when a move-outline flow needs the user to pick a status
	// compatible with the target outline. While set, the status picker "enter" applies the move.
	pendingMoveOutlineTo string
	// pendingMoveParentTo is set when a move flow targets "make child of <item>" and needs the
	// user to pick a status compatible with the target outline. While set alongside
	// pendingMoveOutlineTo, the status picker "enter" applies the move-under-item.
	pendingMoveParentTo string

	// pendingMove batches repeated reorder keypresses (Alt+Up/Down) into a single persisted event.
	pendingMove    *pendingMoveState
	pendingMoveSeq int

	actionPanelStack []actionPanelKind
	// actionPanelSelectedKey is the current selection in the action panel (for tab/enter navigation).
	actionPanelSelectedKey string
	// captureKeySeq stores the currently-entered org-capture style key sequence while in the Capture panel.
	captureKeySeq []string

	pendingEsc   bool
	pendingCtrlX bool

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

	minibufferText  string
	minibufferSetAt time.Time

	autoCommit *gitrepo.DebouncedCommitter

	gitStatus         gitrepo.Status
	gitStatusAt       time.Time
	gitStatusErr      string
	gitStatusFetchSeq int
	gitStatusFetching bool
}

type pendingMoveState struct {
	itemID    string
	actorID   string
	rebalance map[string]string
	lastAt    time.Time
	seq       int
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

func (m *appModel) applyAppearanceStyles() {
	m.applyListStyle()

	// Rebuild delegates that embed selection colors at construction time.
	m.projectAttachmentsList.SetDelegate(newCompactItemDelegate())
	m.itemsList.SetDelegate(newFocusAwareOutlineItemDelegate(m.itemsListActive))
	m.agendaList.SetDelegate(newCompactItemDelegate())
	m.archivedList.SetDelegate(newCompactItemDelegate())

	m.statusList.SetDelegate(newCompactItemDelegate())
	m.activityModalList.SetDelegate(newOutlineItemDelegate())
	m.outlinePickList.SetDelegate(newCompactItemDelegate())
	m.assigneeList.SetDelegate(newCompactItemDelegate())
	m.tagsList.SetDelegate(newFocusAwareCompactItemDelegate(m.tagsListActive))
	m.workspaceList.SetDelegate(newCompactItemDelegate())
	m.outlineStatusDefsList.SetDelegate(newCompactItemDelegate())

	m.captureTemplatesList.SetDelegate(newCompactItemDelegate())
	m.targetPickList.SetDelegate(newCompactItemDelegate())
	m.captureTemplateWorkspaceList.SetDelegate(newCompactItemDelegate())
	m.captureTemplateOutlineList.SetDelegate(newCompactItemDelegate())
	m.captureTemplatePromptsList.SetDelegate(newCompactItemDelegate())
	m.captureTemplatePromptTypeList.SetDelegate(newCompactItemDelegate())
	m.captureTemplatePromptRequiredList.SetDelegate(newCompactItemDelegate())

	// Inputs/editors: ensure readable, theme-aligned contrast across terminals.
	m.applyEditorStyles()

	// Some views cache rendered (ANSI-colored) strings. Theme changes must invalidate
	// those caches so we don't keep old palette output.
	m.previewCacheForID = ""
	m.previewCacheW = 0
	m.previewCacheH = 0
	m.previewCache = ""
}

func (m *appModel) applyEditorStyles() {
	// Textarea (comments/worklog/description editors).
	base := lipgloss.NewStyle().Foreground(colorSurfaceFg).Background(colorInputBg)
	ln := lipgloss.NewStyle().Foreground(colorChromeSubtleFg).Background(colorInputBg)
	ph := lipgloss.NewStyle().Foreground(colorChromeMutedFg).Background(colorInputBg)

	m.textarea.FocusedStyle.Base = base
	m.textarea.FocusedStyle.Text = lipgloss.NewStyle().Foreground(colorSurfaceFg)
	m.textarea.FocusedStyle.Placeholder = ph
	m.textarea.FocusedStyle.LineNumber = ln
	m.textarea.FocusedStyle.CursorLineNumber = ln.Copy().Bold(true)
	m.textarea.FocusedStyle.EndOfBuffer = ln

	m.textarea.BlurredStyle.Base = base
	m.textarea.BlurredStyle.Text = lipgloss.NewStyle().Foreground(colorSurfaceFg)
	m.textarea.BlurredStyle.Placeholder = ph
	m.textarea.BlurredStyle.LineNumber = ln
	m.textarea.BlurredStyle.CursorLineNumber = ln
	m.textarea.BlurredStyle.EndOfBuffer = ln

	// Keep cursor line styling minimal (full-line highlights can reduce readability).
	m.textarea.FocusedStyle.CursorLine = m.textarea.BlurredStyle.CursorLine
}

func (m *appModel) applyListStyle() {
	switch listStyle() {
	case listStyleRows:
		m.projectsList.SetDelegate(newProjectRowsDelegate(false))
		m.outlinesList.SetDelegate(newOutlineRowsDelegate(false))
	case listStyleMinimal:
		m.projectsList.SetDelegate(newProjectRowsDelegate(true))
		m.outlinesList.SetDelegate(newOutlineRowsDelegate(true))
	default:
		m.projectsList.SetDelegate(newProjectCardDelegate())
		m.outlinesList.SetDelegate(newOutlineCardDelegate())
	}
}

func newAppModelWithWorkspace(dir string, db *store.DB, workspace string) appModel {
	s := store.Store{Dir: dir}
	m := appModel{
		dir:            dir,
		workspace:      strings.TrimSpace(workspace),
		store:          s,
		jsonlWorkspace: s.IsJSONLWorkspace(),
		db:             db,
		view:           viewProjects,
		pane:           paneOutline,
	}

	if shouldAutoCommit() && m.jsonlWorkspace {
		m.autoCommit = gitrepo.NewDebouncedCommitter(gitrepo.DebouncedCommitterOpts{
			WorkspaceDir:   dir,
			Debounce:       2 * time.Second,
			AutoPush:       shouldAutoPush(),
			AutoPullRebase: true,
		})
	}
	m.columnsSel = map[string]outlineColumnsSelection{}
	m.tagsListActive = new(bool)
	m.itemsListActive = new(bool)
	*m.itemsListActive = true

	if strings.TrimSpace(os.Getenv("CLARITY_TUI_DEBUG")) != "" {
		m.debugEnabled = true
		m.debugOverlay = true
	}
	m.debugLogPath = strings.TrimSpace(os.Getenv("CLARITY_TUI_DEBUG_LOG"))

	m.projectsList = newList("Projects", "Select a project", []list.Item{})
	m.projectsList.SetDelegate(newProjectCardDelegate())
	m.outlinesList = newList("Outlines", "Select an outline", []list.Item{})
	m.outlinesList.SetDelegate(newOutlineCardDelegate())
	m.projectAttachmentsList = newList("Uploads", "All project uploads", []list.Item{})
	m.projectAttachmentsList.SetDelegate(newCompactItemDelegate())
	m.projectAttachmentsList.SetFilteringEnabled(true)
	m.projectAttachmentsList.SetShowFilter(true)
	m.itemsList = newList("Outline", "Go to items (split view)", []list.Item{})
	m.itemsList.SetDelegate(newFocusAwareOutlineItemDelegate(m.itemsListActive))
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

	m.activityModalList = newList("", "", []list.Item{})
	m.activityModalList.SetDelegate(newOutlineItemDelegate())
	m.activityModalList.SetFilteringEnabled(false)
	m.activityModalList.SetShowFilter(false)
	m.activityModalList.SetShowHelp(false)
	m.activityModalList.SetShowStatusBar(false)
	m.activityModalList.SetShowPagination(false)

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

	m.targetPickList = newList("Targets", "Select a target", []list.Item{})
	m.targetPickList.SetDelegate(newCompactItemDelegate())
	// Filtering helps when a description has many links.
	m.targetPickList.SetFilteringEnabled(true)
	m.targetPickList.SetShowFilter(true)
	// Keep list chrome minimal inside the modal.
	m.targetPickList.SetShowHelp(false)
	m.targetPickList.SetShowStatusBar(false)
	m.targetPickList.SetShowPagination(false)

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
	m.captureTemplatePromptsList = newList("Prompts", "Edit template prompts", []list.Item{})
	m.captureTemplatePromptsList.SetDelegate(newCompactItemDelegate())
	m.captureTemplatePromptsList.SetFilteringEnabled(true)
	m.captureTemplatePromptsList.SetShowFilter(true)
	m.captureTemplatePromptsList.SetShowHelp(false)
	m.captureTemplatePromptsList.SetShowStatusBar(false)
	m.captureTemplatePromptsList.SetShowPagination(false)

	m.captureTemplatePromptTypeList = newList("Prompt type", "Pick a type", []list.Item{})
	m.captureTemplatePromptTypeList.SetDelegate(newCompactItemDelegate())
	m.captureTemplatePromptTypeList.SetFilteringEnabled(false)
	m.captureTemplatePromptTypeList.SetShowFilter(false)
	m.captureTemplatePromptTypeList.SetShowHelp(false)
	m.captureTemplatePromptTypeList.SetShowStatusBar(false)
	m.captureTemplatePromptTypeList.SetShowPagination(false)

	m.captureTemplatePromptRequiredList = newList("Required?", "Pick required", []list.Item{})
	m.captureTemplatePromptRequiredList.SetDelegate(newCompactItemDelegate())
	m.captureTemplatePromptRequiredList.SetFilteringEnabled(false)
	m.captureTemplatePromptRequiredList.SetShowFilter(false)
	m.captureTemplatePromptRequiredList.SetShowHelp(false)
	m.captureTemplatePromptRequiredList.SetShowStatusBar(false)
	m.captureTemplatePromptRequiredList.SetShowPagination(false)

	m.captureTemplatePromptDeleteIdx = -1

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
	m.textarea.MaxHeight = 0
	m.textarea.SetWidth(72)
	m.textarea.SetHeight(10)
	m.textarea.ShowLineNumbers = true
	// Avoid highlighting the full current line; the cursor is enough for focus.
	m.textarea.FocusedStyle.CursorLine = m.textarea.BlurredStyle.CursorLine

	m.applyAppearanceStyles()
	m.refreshProjects()

	// Best-effort: restore last TUI screen/selection for this workspace.
	if st, err := s.LoadTUIState(); err == nil {
		m.applySavedTUIState(st)
	}

	m.captureStoreModTimes()
	m.refreshEventsTail()
	return m
}
