package tui

import (
        "strings"
        "time"

        "clarity-cli/internal/gitrepo"

        tea "github.com/charmbracelet/bubbletea"
)

type view int

const (
        viewProjects view = iota
        viewOutlines
        viewOutline
        viewItem
        viewAgenda
        viewArchived
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

type gitStatusMsg struct {
        seq    int
        status gitrepo.Status
        err    string
}

type syncOpDoneMsg struct {
        op     string
        status gitrepo.Status
        err    string
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

type inputDebug struct {
        lastAt   time.Time
        lastType string
        lastStr  string
}

func (m appModel) debugKeyMsg(k tea.KeyMsg) {
        if !m.debugEnabled {
                return
        }
        // Only write if the user provided a log path.
        if strings.TrimSpace(m.debugLogPath) == "" {
                return
        }
        // Keep compact but high-signal for diagnosing modifier keys.
        (&m).debugLogf(
                "key view=%s pane=%s modal=%d filter(setting=%v filtered=%v) str=%q type=%v alt=%v runes=%q",
                viewToString(m.view),
                paneToString(m.pane),
                int(m.modal),
                m.itemsList.SettingFilter(),
                m.itemsList.IsFiltered(),
                k.String(),
                k.Type,
                k.Alt,
                string(k.Runes),
        )
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
        modalEditDescription
        modalStatusNote
        modalEditOutlineName
        modalEditOutlineDescription
        modalSetDue
        modalSetSchedule
        modalPickStatus
        modalPickOutline
        modalPickAssignee
        modalEditTags
        modalPickWorkspace
        modalNewWorkspace
        modalRenameWorkspace
        modalAddComment
        modalReplyComment
        modalAddWorklog
        modalEditOutlineStatuses
        modalAddOutlineStatus
        modalRenameOutlineStatus
        modalJumpToItem
        modalActionPanel
        modalCaptureTemplates
        modalCaptureTemplateName
        modalCaptureTemplateKeys
        modalCaptureTemplatePickWorkspace
        modalCaptureTemplatePickOutline
        modalCaptureTemplateDefaultTitle
        modalCaptureTemplateDefaultDescription
        modalCaptureTemplateDefaultTags
        modalConfirmDeleteCaptureTemplate
        modalCapture
        modalGitSetupRemote
)

type actionPanelKind int

const (
        actionPanelContext actionPanelKind = iota
        actionPanelNav
        actionPanelAgenda
        actionPanelCapture
        actionPanelSync
        actionPanelOutline
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

type dateModalFocus int

const (
        dateFocusYear dateModalFocus = iota
        dateFocusMonth
        dateFocusDay
        dateFocusTimeToggle
        dateFocusHour
        dateFocusMinute
        dateFocusSave
        dateFocusClear
        dateFocusCancel
)

type tagsModalFocus int

const (
        tagsFocusInput tagsModalFocus = iota
        tagsFocusList
)

func (m *appModel) closeAllModals() {
        if m == nil {
                return
        }
        // Close action panel if open (it has its own stack state).
        if m.modal == modalActionPanel {
                m.closeActionPanel()
        }
        m.modal = modalNone
        m.modalForID = ""
        m.modalForKey = ""
        m.capture = nil
        m.replyQuoteMD = ""
        m.pendingMoveOutlineTo = ""
        m.captureTemplateEdit = nil
        m.captureTemplateDeleteIdx = -1

        m.textFocus = textFocusBody
        m.dateFocus = dateFocusYear
        m.tagsFocus = tagsFocusInput
        m.timeEnabled = false

        // Reset inputs (safe even if not currently used).
        m.input.Placeholder = "Title"
        m.input.SetValue("")
        m.input.Blur()

        m.yearInput.Placeholder = "YYYY"
        m.yearInput.SetValue("")
        m.yearInput.Blur()
        m.monthInput.Placeholder = "MM"
        m.monthInput.SetValue("")
        m.monthInput.Blur()
        m.dayInput.Placeholder = "DD"
        m.dayInput.SetValue("")
        m.dayInput.Blur()
        m.hourInput.Placeholder = "HH"
        m.hourInput.SetValue("")
        m.hourInput.Blur()
        m.minuteInput.Placeholder = "MM"
        m.minuteInput.SetValue("")
        m.minuteInput.Blur()

        m.textarea.SetValue("")
        m.textarea.Blur()
}
