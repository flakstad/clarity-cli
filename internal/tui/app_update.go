package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// The bubbles file picker emits internal message types (readDirMsg/errorMsg) that
	// we can't switch on directly. When the picker modal is active, forward all
	// non-window-size messages into updateOutline so the picker can handle them.
	if m.modal == modalPickAttachmentFile {
		if _, ok := msg.(tea.WindowSizeMsg); !ok {
			mmAny, cmd := m.updateOutline(msg)
			if mm, ok := mmAny.(appModel); ok {
				return mm, cmd
			}
			return m, cmd
		}
	}

	switch msg := msg.(type) {
	case externalEditorDoneMsg:
		m.applyExternalEditorResult(msg)
		// If we're still in a text modal, keep the body focused after returning from the editor.
		if m.modal == modalAddComment || m.modal == modalReplyComment || m.modal == modalAddWorklog || m.modal == modalEditDescription || m.modal == modalEditOutlineDescription || m.modal == modalStatusNote {
			m.textFocus = textFocusBody
			m.textarea.Focus()
		}
		return m, nil

	case externalViewEditorDoneMsg:
		m.applyExternalViewEditorResult(msg)
		return m, nil

	case attachmentOpenDoneMsg:
		if msg.err != nil {
			m.showMinibuffer("Open failed: " + msg.err.Error())
		} else {
			m.showMinibuffer("Opened attachment")
		}
		return m, nil

	case urlOpenDoneMsg:
		if msg.err != nil {
			m.showMinibuffer("Open failed: " + msg.err.Error())
		} else {
			m.showMinibuffer("Opened link")
		}
		return m, nil

	case tea.WindowSizeMsg:
		// Avoid rendering into the last terminal column: some terminals autowrap when
		// writing a character in the final column, which can visually corrupt box
		// borders (e.g. right border wrapping onto the next line).
		m.width = msg.Width
		if m.width > 0 {
			m.width--
		}
		m.height = msg.Height
		m.resizeLists()
		if m.modal == modalCapture && m.capture != nil {
			mmAny, _ := m.capture.Update(msg)
			if mm, ok := mmAny.(captureModel); ok {
				*m.capture = mm
			}
		}
		var filePickerCmd tea.Cmd
		if m.modal == modalPickAttachmentFile {
			m.attachmentFilePicker.Height = attachmentFilePickerHeight(m.height)
			m.attachmentFilePicker, filePickerCmd = m.attachmentFilePicker.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		}
		// Don't show the resize overlay on startup; only after we've seen an initial size.
		if !m.seenWindowSize {
			m.seenWindowSize = true
			m.resizing = false
			return m, nil
		}
		m.resizing = true
		m.resizeSeq++
		seq := m.resizeSeq
		cmds := []tea.Cmd{
			tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return resizeDoneMsg{seq: seq} }),
		}
		if filePickerCmd != nil {
			cmds = append(cmds, filePickerCmd)
		}
		return m, tea.Batch(cmds...)

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

	case captureFinishedMsg:
		// Embedded capture flow completed; return to the main TUI.
		m.modal = modalNone
		m.capture = nil
		m.returnToCaptureAfterTemplates = false
		if msg.canceled {
			return m, nil
		}
		id := strings.TrimSpace(msg.result.ItemID)
		if id != "" {
			m.showMinibuffer("Captured: " + id)
		} else {
			m.showMinibuffer("Captured")
		}
		// Best-effort: if the capture targeted this workspace, reload so the new items appear immediately.
		if strings.TrimSpace(msg.result.Dir) != "" && strings.TrimSpace(msg.result.Dir) == strings.TrimSpace(m.dir) {
			_ = m.reloadFromDisk()
			if id != "" {
				m.recordRecentCapturedItem(id)
			}
		}
		return m, nil

	case captureOpenTemplatesMsg:
		// Capture modal requested opening the templates manager; keep capture state so we can return.
		m.returnToCaptureAfterTemplates = true
		m.openCaptureTemplatesModal()
		return m, nil

	case previewComputeMsg:
		return m, nil

	case reloadTickMsg:
		if m.storeChanged() {
			_ = m.reloadFromDisk()
		}
		cmds := []tea.Cmd{tickReload()}
		if (&m).shouldRefreshGitStatus() {
			cmds = append(cmds, (&m).startGitStatusRefresh())
		}
		return m, tea.Batch(cmds...)

	case gitStatusMsg:
		if msg.seq != m.gitStatusFetchSeq {
			// Stale response (workspace switched mid-flight).
			return m, nil
		}
		m.gitStatus = msg.status
		m.gitStatusAt = time.Now()
		m.gitStatusErr = strings.TrimSpace(msg.err)
		m.gitStatusFetching = false
		return m, nil

	case syncOpDoneMsg:
		m.gitStatus = msg.status
		m.gitStatusAt = time.Now()
		m.gitStatusErr = strings.TrimSpace(msg.err)
		m.gitStatusFetching = false
		if strings.TrimSpace(msg.err) != "" {
			m.showMinibuffer("Sync: " + msg.op + ": " + msg.err)
		} else {
			m.showMinibuffer("Sync: " + msg.op)
		}
		return m, nil

	case escTimeoutMsg:
		if m.pendingEsc && m.modal == modalNone {
			// Treat a lone ESC as "back" in the outline view.
			m.pendingEsc = false
			if m.view == viewOutline {
				m.view = viewOutlines
				m.refreshOutlines(m.selectedProjectID)
				return m, nil
			}
			if m.view == viewItem {
				(&m).widenItemView()
				return m, nil
			}
		}
		return m, nil

	case ctrlXTimeoutMsg:
		if m.pendingCtrlX {
			m.pendingCtrlX = false
		}
		return m, nil

	case tea.KeyMsg:
		if m.debugEnabled {
			m.inputDbg.lastAt = time.Now()
			m.inputDbg.lastType = fmt.Sprintf("%T", msg)
			m.inputDbg.lastStr = msg.String()
		}
		// Write every key event to the debug log (if configured).
		m.debugKeyMsg(msg)
		// If a modal is open, route all keys to the modal handler so text inputs behave
		// normally (e.g. backspace edits).
		if m.modal != modalNone {
			return m.updateOutline(msg)
		}
		// When filtering the outline list, capture all keystrokes for the filter input.
		// This prevents global bindings like "a" (agenda) from triggering while typing.
		if m.view == viewOutline && m.itemsList.SettingFilter() {
			switch msg.String() {
			case "ctrl+c":
				return m, m.quitWithStateCmd()
			default:
				return m.updateOutline(msg)
			}
		}
		if m.view == viewAgenda {
			return m.updateAgenda(msg)
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, m.quitWithStateCmd()
		case "x", "?":
			m.openActionPanel(actionPanelContext)
			return m, nil
		case "g":
			m.openActionPanel(actionPanelNav)
			return m, nil
		case "a":
			// Agenda dispatcher: open the agenda commands panel, then choose a command (e.g. 't').
			m.openActionPanel(actionPanelAgenda)
			return m, nil
		case "c":
			return (&m).openCaptureModal()
		case "y":
			if m.view == viewItem && strings.TrimSpace(m.openItemID) != "" {
				id := selectedOutlineListItemID(&m.itemsList)
				if strings.TrimSpace(id) == "" {
					id = strings.TrimSpace(m.openItemID)
				}
				txt := m.clipboardItemRef(id)
				if err := copyToClipboard(txt); err != nil {
					m.showMinibuffer("Clipboard error: " + err.Error())
				} else {
					m.showMinibuffer("Copied: " + txt)
				}
				return m, nil
			}
		case "Y":
			if m.view == viewItem && strings.TrimSpace(m.openItemID) != "" {
				id := selectedOutlineListItemID(&m.itemsList)
				if strings.TrimSpace(id) == "" {
					id = strings.TrimSpace(m.openItemID)
				}
				cmd := m.clipboardShowCmd(id)
				if err := copyToClipboard(cmd); err != nil {
					m.showMinibuffer("Clipboard error: " + err.Error())
				} else {
					m.showMinibuffer("Copied: " + cmd)
				}
				return m, nil
			}
		case "ctrl+x":
			if m.view == viewItem && m.modal == modalNone && strings.TrimSpace(m.openItemID) != "" {
				m.pendingCtrlX = true
				return m, tea.Tick(600*time.Millisecond, func(time.Time) tea.Msg { return ctrlXTimeoutMsg{} })
			}
		case "backspace":
			// While filtering the outline list, backspace edits the filter input.
			if m.view == viewOutline && m.itemsList.SettingFilter() {
				break
			}
			if m.view == viewItem {
				(&m).widenItemView()
				return m, nil
			}
			switch m.view {
			case viewAgenda:
				if m.hasAgendaReturnView {
					m.view = m.agendaReturnView
					m.hasAgendaReturnView = false
				} else {
					m.view = viewProjects
				}
				return m, nil
			case viewOutline:
				m.view = viewOutlines
				m.refreshOutlines(m.selectedProjectID)
				m.showPreview = false
				return m, nil
			case viewProjectAttachments:
				m.view = viewOutlines
				m.refreshOutlines(m.selectedProjectID)
				return m, nil
			case viewOutlines:
				m.view = viewProjects
				m.refreshProjects()
				return m, nil
			case viewArchived:
				if m.hasArchivedReturnView {
					m.view = m.archivedReturnView
					m.hasArchivedReturnView = false
				} else {
					m.view = viewProjects
				}
				switch m.view {
				case viewProjects:
					m.refreshProjects()
				case viewOutlines:
					m.refreshOutlines(m.selectedProjectID)
				case viewAgenda:
					m.refreshAgenda()
				case viewOutline:
					if o, ok := m.db.FindOutline(m.selectedOutlineID); ok {
						m.refreshItems(*o)
					}
				case viewArchived:
					m.refreshArchived()
				}
				return m, nil
			}
		case "esc":
			// When the outline list is filtering or filtered, ESC should cancel/clear the filter
			// instead of navigating "back".
			if m.view == viewOutline && m.modal == modalNone && (m.itemsList.SettingFilter() || m.itemsList.IsFiltered()) {
				break
			}
			if m.view == viewItem {
				// Delay treating ESC as "back" so we can interpret ESC+<key> as Alt+<key>.
				if m.modal == modalNone && m.pane == paneOutline {
					m.pendingEsc = true
					return m, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return escTimeoutMsg{} })
				}
				(&m).widenItemView()
				return m, nil
			}
			if m.view == viewAgenda {
				if m.hasAgendaReturnView {
					m.view = m.agendaReturnView
					m.hasAgendaReturnView = false
				} else {
					m.view = viewProjects
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
			case viewProjectAttachments:
				m.view = viewOutlines
				m.refreshOutlines(m.selectedProjectID)
				return m, nil
			case viewOutlines:
				m.view = viewProjects
				m.refreshProjects()
				return m, nil
			case viewArchived:
				if m.hasArchivedReturnView {
					m.view = m.archivedReturnView
					m.hasArchivedReturnView = false
				} else {
					m.view = viewProjects
				}
				switch m.view {
				case viewProjects:
					m.refreshProjects()
				case viewOutlines:
					m.refreshOutlines(m.selectedProjectID)
				case viewAgenda:
					m.refreshAgenda()
				case viewOutline:
					if o, ok := m.db.FindOutline(m.selectedOutlineID); ok {
						m.refreshItems(*o)
					}
				case viewArchived:
					m.refreshArchived()
				}
				return m, nil
			}
		case "enter":
			switch m.view {
			case viewProjects:
				switch it := m.projectsList.SelectedItem().(type) {
				case projectItem:
					m.selectedProjectID = it.project.ID
					m.db.CurrentProjectID = it.project.ID
					_ = m.store.Save(m.db)
					m.captureStoreModTimes()
					m.view = viewOutlines
					m.refreshOutlines(it.project.ID)
					return m, nil
				case addProjectRow:
					m.openInputModal(modalNewProject, "", "Project name", "")
					return m, nil
				}
			case viewOutlines:
				switch it := m.outlinesList.SelectedItem().(type) {
				case outlineItem:
					m.selectedOutlineID = it.outline.ID
					m.selectedOutline = &it.outline
					m.view = viewOutline
					// Apply per-outline view mode (includes preview state).
					m.setOutlineViewMode(it.outline.ID, m.outlineViewModeForID(it.outline.ID))
					m.openItemID = ""
					m.itemArchivedReadOnly = false
					m.collapsed = map[string]bool{}
					m.refreshItems(it.outline)
					return m, nil
				case projectUploadsRow:
					m.view = viewProjectAttachments
					m.refreshProjectAttachments(m.selectedProjectID)
					return m, nil
				case addOutlineRow:
					m.openInputModal(modalNewOutline, "", "Outline name (optional)", "")
					return m, nil
				}
			case viewProjectAttachments:
				if it, ok := m.projectAttachmentsList.SelectedItem().(projectAttachmentListItem); ok {
					return m, m.openAttachment(it.Attachment)
				}
			case viewAgenda:
				if it, ok := m.agendaList.SelectedItem().(agendaRowItem); ok {
					m.selectedProjectID = it.row.item.ProjectID
					m.selectedOutlineID = it.row.item.OutlineID
					m.selectedOutline = &it.outline
					m.openItemID = it.row.item.ID
					(&m).recordRecentItemVisit(m.openItemID)
					m.view = viewItem
					m.itemCollapsed = copyBoolMap(m.collapsed)
					m.itemArchivedReadOnly = false
					m.itemFocus = itemFocusComments
					m.itemCommentIdx = 0
					m.itemWorklogIdx = 0
					m.itemHistoryIdx = 0
					m.itemSideScroll = 0
					m.itemDetailScroll = 0
					m.itemNavStack = nil
					m.hasReturnView = true
					m.returnView = viewAgenda
					m.showPreview = false
					m.pane = paneOutline
					if m.itemsListActive != nil {
						*m.itemsListActive = true
					}
					m.itemListRootID = ""
					(&m).expandOneLevelInItemView(m.openItemID)
					(&m).refreshItemSubtree(it.outline, m.openItemID)
					selectListItemByID(&m.itemsList, m.openItemID)
					return m, nil
				}
			case viewArchived:
				if it, ok := m.archivedList.SelectedItem().(archivedItemRowItem); ok {
					id := strings.TrimSpace(it.itemID)
					if id == "" {
						return m, nil
					}
					if item, ok := m.db.FindItem(id); ok && item != nil {
						m.selectedProjectID = item.ProjectID
						m.selectedOutlineID = item.OutlineID
						if o, ok := m.db.FindOutline(item.OutlineID); ok && o != nil {
							m.selectedOutline = o
						}
					}
					m.openItemID = id
					m.view = viewItem
					m.itemCollapsed = copyBoolMap(m.collapsed)
					m.itemArchivedReadOnly = true
					m.itemFocus = itemFocusComments
					m.itemCommentIdx = 0
					m.itemWorklogIdx = 0
					m.itemHistoryIdx = 0
					m.itemSideScroll = 0
					m.itemDetailScroll = 0
					m.itemNavStack = nil
					m.hasReturnView = true
					m.returnView = viewArchived
					m.showPreview = false
					m.pane = paneOutline
					if m.itemsListActive != nil {
						*m.itemsListActive = true
					}
					if m.selectedOutline != nil {
						m.itemListRootID = ""
						(&m).expandOneLevelInItemView(m.openItemID)
						(&m).refreshItemSubtree(*m.selectedOutline, m.openItemID)
						selectListItemByID(&m.itemsList, m.openItemID)
					}
					return m, nil
				}
			}
		case "S":
			// Edit outline status definitions (outline list view).
			if m.view == viewOutlines {
				if it, ok := m.outlinesList.SelectedItem().(outlineItem); ok {
					m.selectedOutlineID = strings.TrimSpace(it.outline.ID)
					m.selectedOutline = &it.outline
					m.openOutlineStatusDefsEditor(it.outline, "")
					return m, nil
				}
			}
		case "m":
			// Move open item to another outline (item view).
			if m.view == viewItem && strings.TrimSpace(m.openItemID) != "" {
				m.openMoveOutlinePicker(strings.TrimSpace(m.openItemID))
				return m, nil
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
					m.openInputModal(modalRenameProject, it.project.ID, "Project name", strings.TrimSpace(it.project.Name))
					return m, nil
				}
			}
			if m.view == viewOutlines {
				// Rename outline.
				if it, ok := m.outlinesList.SelectedItem().(outlineItem); ok {
					name := ""
					if it.outline.Name != nil {
						name = strings.TrimSpace(*it.outline.Name)
					}
					m.openInputModal(modalEditOutlineName, it.outline.ID, "Outline name (optional)", name)
					return m, nil
				}
			}
			if m.view == viewProjectAttachments {
				if it, ok := m.projectAttachmentsList.SelectedItem().(projectAttachmentListItem); ok {
					a := it.Attachment
					m.attachmentEditID = strings.TrimSpace(a.ID)
					m.attachmentEditTitle = strings.TrimSpace(a.Title)
					m.attachmentEditAlt = strings.TrimSpace(a.Alt)
					m.openInputModal(modalEditAttachmentTitle, "", "Title (recommended)", m.attachmentEditTitle)
					return m, nil
				}
			}
		case "i":
			if m.view == viewProjectAttachments {
				if it, ok := m.projectAttachmentsList.SelectedItem().(projectAttachmentListItem); ok {
					id := strings.TrimSpace(it.ItemID)
					if id == "" {
						return m, nil
					}
					m.selectedProjectID = strings.TrimSpace(it.ProjectID)
					m.selectedOutlineID = strings.TrimSpace(it.OutlineID)
					if o, ok := m.db.FindOutline(m.selectedOutlineID); ok && o != nil {
						m.selectedOutline = o
						m.refreshItems(*o)
						selectListItemByID(&m.itemsList, id)
					}
					m.openItemID = id
					(&m).recordRecentItemVisit(m.openItemID)
					m.view = viewItem
					m.itemArchivedReadOnly = false
					m.itemFocus = itemFocusTitle
					m.itemCommentIdx = 0
					m.itemWorklogIdx = 0
					m.itemHistoryIdx = 0
					m.itemSideScroll = 0
					m.itemDetailScroll = 0
					m.hasReturnView = true
					m.returnView = viewProjectAttachments
					m.showPreview = false
					m.pane = paneOutline
					if m.itemsListActive != nil {
						*m.itemsListActive = true
					}
					return m, nil
				}
			}
		case "U":
			if m.view == viewOutlines {
				if !m.projectHasUploads(m.selectedProjectID) {
					m.showMinibuffer("No uploads")
					return m, nil
				}
				m.view = viewProjectAttachments
				m.refreshProjectAttachments(m.selectedProjectID)
				return m, nil
			}
		case "D":
			// Edit outline description (outline list view).
			if m.view == viewOutlines {
				if it, ok := m.outlinesList.SelectedItem().(outlineItem); ok {
					m.openTextModal(modalEditOutlineDescription, it.outline.ID, "Markdown outline description…", it.outline.Description)
					return m, nil
				}
			}
		case "n":
			if m.view == viewProjects {
				// New project.
				m.openInputModal(modalNewProject, "", "Project name", "")
				return m, nil
			}
			if m.view == viewOutlines {
				// New outline (name optional).
				m.openInputModal(modalNewOutline, "", "Outline name (optional)", "")
				return m, nil
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
		case viewProjectAttachments:
			var cmd tea.Cmd
			m.projectAttachmentsList, cmd = m.projectAttachmentsList.Update(msg)
			return m, cmd
		case viewAgenda:
			var cmd tea.Cmd
			m.agendaList, cmd = m.agendaList.Update(msg)
			return m, cmd
		case viewArchived:
			var cmd tea.Cmd
			m.archivedList, cmd = m.archivedList.Update(msg)
			return m, cmd
		case viewOutline:
			return m.updateOutline(msg)
		case viewItem:
			return m.updateItem(msg)
		default:
			return m, nil
		}

	default:
		if m.debugEnabled {
			m.inputDbg.lastAt = time.Now()
			m.inputDbg.lastType = fmt.Sprintf("%T", msg)
			if s, ok := any(msg).(fmt.Stringer); ok {
				m.inputDbg.lastStr = s.String()
			} else {
				m.inputDbg.lastStr = ""
			}
		}
		// Terminal.app Option+Arrow often arrives as an unknown CSI sequence in Bubble Tea.
		// Log it (and decoded bytes) so we can map it reliably.
		if m.debugEnabled && strings.TrimSpace(m.debugLogPath) != "" {
			if s, ok := any(msg).(fmt.Stringer); ok {
				raw := s.String()
				if strings.HasPrefix(raw, "?CSI[") {
					decoded, _ := decodeUnknownCSIString(raw)
					(&m).debugLogf("csi view=%s pane=%s modal=%d str=%q decoded=%q",
						viewToString(m.view), paneToString(m.pane), int(m.modal), raw, decoded)
				}
			}
		}
		// Bubble list filtering (and related UI like cursor blinking) emits non-key messages
		// via Cmds. If we're filtering (or have a filter applied) in the outline view, we must
		// forward these messages back into the list model or filtering will appear "stuck".
		if m.view == viewOutline && (m.itemsList.SettingFilter() || m.itemsList.IsFiltered()) {
			return m.updateOutline(msg)
		}
		// Some terminals (notably macOS Terminal.app) emit Option/Alt+Arrow as CSI
		// sequences Bubble Tea doesn't map (it reports them as "unknown CSI").
		// Best-effort: interpret those sequences for outline move/indent/outdent.
		if s, ok := any(msg).(fmt.Stringer); ok {
			if m.view == viewOutline && m.modal == modalNone && m.pane == paneOutline && !m.itemsList.SettingFilter() {
				if km, ok := keyMsgFromUnknownCSIString(s.String()); ok {
					if handled, cmd := m.mutateOutlineByKey(km); handled {
						return m, cmd
					}
				}
			}
		}
	}

	return m, nil
}

func (m appModel) updateItem(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch km := msg.(type) {
	case tea.KeyMsg:
		rootID := strings.TrimSpace(m.openItemID)
		if rootID == "" {
			return m, nil
		}
		outline, ok := m.db.FindOutline(m.selectedOutlineID)
		if !ok || outline == nil {
			return m, nil
		}

		m.pane = paneOutline
		if m.itemsListActive != nil {
			*m.itemsListActive = true
		}

		if strings.TrimSpace(m.itemListRootID) != rootID {
			(&m).refreshItemSubtree(*outline, rootID)
			selectListItemByID(&m.itemsList, rootID)
		}

		// Handle ESC-prefix Alt sequences (ESC then key) for movement/indent/outdent.
		if m.pendingEsc {
			m.pendingEsc = false
			if m.itemArchivedReadOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			switch km.String() {
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
		}

		switch km.String() {
		case "H":
			itemID := strings.TrimSpace(selectedOutlineListItemID(&m.itemsList))
			if itemID == "" {
				if it, ok := m.itemsList.SelectedItem().(outlineActivityRowItem); ok {
					itemID = strings.TrimSpace(it.itemID)
				}
			}
			if itemID == "" {
				itemID = rootID
			}
			(&m).openHistoryModal(itemID)
			return m, nil
		case "tab":
			if act, ok := m.itemsList.SelectedItem().(outlineActivityRowItem); ok && (act.hasChildren || act.hasDescription) {
				collapsed := m.collapsedState()
				collapsed[act.id] = !collapsed[act.id]
				m.refreshItemSubtree(*outline, rootID)
				selectListItemByID(&m.itemsList, act.id)
				return m, nil
			}
			m.toggleCollapseSelected()
			return m, nil
		case "shift+tab", "backtab":
			m.toggleCollapseAll()
			return m, nil
		}

		// Left pane: outline-style navigation within the current subtree.
		if m.pane == paneOutline {
			// Outline navigation keys (parent/child) should keep working.
			if m.navOutline(km) {
				return m, nil
			}
			if handled, cmd := m.mutateOutlineByKey(km); handled {
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				return m, cmd
			}

			rowID := strings.TrimSpace(selectedOutlineListItemID(&m.itemsList))
			if rowID == "" {
				rowID = rootID
			}

			switch km.String() {
			case "enter":
				if act, ok := m.itemsList.SelectedItem().(outlineActivityRowItem); ok {
					(&m).openModalForActivityRow(act)
					return m, nil
				}
				// Narrow further to the selected row.
				toID := rowID
				if strings.TrimSpace(toID) == "" || strings.TrimSpace(toID) == rootID {
					return m, nil
				}
				m.itemNavStack = append(m.itemNavStack, itemNavEntry{fromID: rootID, toID: strings.TrimSpace(toID)})
				if len(m.itemNavStack) > 64 {
					m.itemNavStack = m.itemNavStack[len(m.itemNavStack)-64:]
				}
				m.openItemID = strings.TrimSpace(toID)
				(&m).recordRecentItemVisit(m.openItemID)
				m.itemListRootID = ""
				(&m).expandOneLevelInItemView(m.openItemID)
				m.refreshItemSubtree(*outline, m.openItemID)
				selectListItemByID(&m.itemsList, m.openItemID)
				return m, nil
			case "e":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				if it, ok := m.db.FindItem(rowID); ok && it != nil {
					m.openInputModal(modalEditTitle, rowID, "Title", it.Title)
				}
				return m, nil
			case "D":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				if it, ok := m.db.FindItem(rowID); ok && it != nil {
					m.openTextModal(modalEditDescription, rowID, "Markdown description…", it.Description)
				}
				return m, nil
			case "p":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				if err := m.togglePriority(rowID); err != nil {
					return m, m.reportError(rowID, err)
				}
				return m, nil
			case "o":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				if err := m.toggleOnHold(rowID); err != nil {
					return m, m.reportError(rowID, err)
				}
				return m, nil
			case " ":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				cur := ""
				if it, ok := m.db.FindItem(rowID); ok && it != nil {
					cur = it.StatusID
				}
				m.openStatusPicker(*outline, rowID, cur)
				m.modal = modalPickStatus
				m.modalForID = rowID
				return m, nil
			case "shift+right":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				if err := m.cycleItemStatus(*outline, rowID, +1); err != nil {
					return m, m.reportError(rowID, err)
				}
				return m, nil
			case "shift+left":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				if err := m.cycleItemStatus(*outline, rowID, -1); err != nil {
					return m, m.reportError(rowID, err)
				}
				return m, nil
			case "n":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				m.openInputModal(modalNewSibling, rowID, "Title", "")
				return m, nil
			case "N":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				m.openInputModal(modalNewChild, rowID, "Title", "")
				return m, nil
			case "m":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				m.openMoveOutlinePicker(rowID)
				return m, nil
			case "r":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				m.modal = modalConfirmArchive
				m.modalForID = rowID
				m.archiveFor = archiveTargetItem
				m.input.Blur()
				return m, nil
			case "V":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				if _, err := (&m).duplicateItem(rowID, true); err != nil {
					return m, m.reportError(rowID, err)
				}
				m.pane = paneOutline
				return m, nil
			case "A":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				m.openAssigneePicker(rowID)
				return m, nil
			case "t":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				m.openTagsEditor(rowID)
				return m, nil
			case "d":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				if it, ok := m.db.FindItem(rowID); ok && it != nil {
					m.openDateModal(modalSetDue, rowID, it.Due)
				}
				return m, nil
			case "s":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				if it, ok := m.db.FindItem(rowID); ok && it != nil {
					m.openDateModal(modalSetSchedule, rowID, it.Schedule)
				}
				return m, nil
			case "C":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				m.openTextModal(modalAddComment, rowID, "Comment…", "")
				return m, nil
			case "w":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				m.openTextModal(modalAddWorklog, rowID, "My worklog…", "")
				return m, nil
			case "R":
				if m.itemArchivedReadOnly {
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
				act, ok := m.itemsList.SelectedItem().(outlineActivityRowItem)
				if !ok || act.kind != outlineActivityComment {
					return m, nil
				}
				itemID := strings.TrimSpace(act.itemID)
				if itemID == "" {
					itemID = rootID
				}
				c, ok := findCommentByID(m.db.CommentsForItem(itemID), act.commentID)
				if !ok {
					return m, nil
				}
				quote := truncateInline(c.Body, 280)
				m.replyQuoteMD = fmt.Sprintf("> %s  %s\n> %s", fmtTS(c.CreatedAt), actorLabel(m.db, c.AuthorID), quote)
				m.openTextModal(modalReplyComment, itemID, "Reply…", "")
				m.modalForKey = strings.TrimSpace(c.ID)
				return m, nil
			}

			prevSelID := selectedOutlineListSelectionID(&m.itemsList)
			var cmd tea.Cmd
			m.itemsList, cmd = m.itemsList.Update(msg)
			_ = prevSelID
			return m, cmd
		}
	}
	return m, nil
}
