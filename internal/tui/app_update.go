package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"clarity-cli/internal/model"

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
		if m.view == viewOutline && m.pendingEsc && m.modal == modalNone {
			// Treat a lone ESC as "back" in the outline view.
			m.pendingEsc = false
			m.view = viewOutlines
			m.refreshOutlines(m.selectedProjectID)
			return m, nil
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
				id := strings.TrimSpace(m.openItemID)
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
				id := strings.TrimSpace(m.openItemID)
				cmd := m.clipboardShowCmd(id)
				if err := copyToClipboard(cmd); err != nil {
					m.showMinibuffer("Clipboard error: " + err.Error())
				} else {
					m.showMinibuffer("Copied: " + cmd)
				}
				return m, nil
			}
		case "backspace":
			// While filtering the outline list, backspace edits the filter input.
			if m.view == viewOutline && m.itemsList.SettingFilter() {
				break
			}
			if m.view == viewItem {
				// If we navigated within the item view (parent/child), pop back to the previous item.
				if n := len(m.itemNavStack); n > 0 {
					ent := m.itemNavStack[n-1]
					m.itemNavStack = m.itemNavStack[:n-1]
					prevID := strings.TrimSpace(ent.fromID)
					if prevID != "" {
						m.openItemID = prevID
						(&m).recordRecentItemVisit(m.openItemID)
						m.view = viewItem
						m.itemFocus = itemFocusTitle
						m.itemCommentIdx = 0
						m.itemWorklogIdx = 0
						m.itemHistoryIdx = 0
						m.itemSideScroll = 0
						m.itemDetailScroll = 0

						// Restore focus/selection (best-effort).
						toID := strings.TrimSpace(ent.toID)
						m.itemChildIdx = 0
						m.itemChildOff = 0

						// If toID is one of prev's children, focus Children and select it.
						children := m.db.ChildrenOf(prevID)
						sort.Slice(children, func(i, j int) bool { return compareOutlineItems(children[i], children[j]) < 0 })
						foundChild := false
						if toID != "" && len(children) > 0 {
							for i := range children {
								if strings.TrimSpace(children[i].ID) == toID {
									m.itemChildIdx = i
									foundChild = true
									break
								}
							}
						}
						if foundChild {
							m.itemFocus = itemFocusChildren
							const maxRows = 8
							if m.itemChildIdx >= maxRows {
								m.itemChildOff = m.itemChildIdx - maxRows + 1
							}
						} else if toID != "" {
							// Otherwise, if prev's parent is toID, focus Parent.
							if prev, ok := m.db.FindItem(prevID); ok && prev != nil && prev.ParentID != nil && strings.TrimSpace(*prev.ParentID) == toID {
								m.itemFocus = itemFocusParent
							}
						}
					}
					return m, nil
				}
				if m.hasReturnView {
					(&m).returnFromItemView()
				} else {
					m.view = viewOutline
					m.openItemID = ""
					m.itemArchivedReadOnly = false
					m.showPreview = false
					m.pane = paneOutline
					if o, ok := m.db.FindOutline(m.selectedOutlineID); ok {
						m.refreshItems(*o)
					}
				}
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
				// If we navigated within the item view (parent/child), pop back to the previous item.
				if n := len(m.itemNavStack); n > 0 {
					ent := m.itemNavStack[n-1]
					m.itemNavStack = m.itemNavStack[:n-1]
					prevID := strings.TrimSpace(ent.fromID)
					if prevID != "" {
						m.openItemID = prevID
						m.view = viewItem
						m.itemFocus = itemFocusTitle
						m.itemCommentIdx = 0
						m.itemWorklogIdx = 0
						m.itemHistoryIdx = 0
						m.itemSideScroll = 0
						m.itemDetailScroll = 0

						// Restore focus/selection (best-effort).
						toID := strings.TrimSpace(ent.toID)
						m.itemChildIdx = 0
						m.itemChildOff = 0

						// If toID is one of prev's children, focus Children and select it.
						children := m.db.ChildrenOf(prevID)
						sort.Slice(children, func(i, j int) bool { return compareOutlineItems(children[i], children[j]) < 0 })
						foundChild := false
						if toID != "" && len(children) > 0 {
							for i := range children {
								if strings.TrimSpace(children[i].ID) == toID {
									m.itemChildIdx = i
									foundChild = true
									break
								}
							}
						}
						if foundChild {
							m.itemFocus = itemFocusChildren
							const maxRows = 8
							if m.itemChildIdx >= maxRows {
								m.itemChildOff = m.itemChildIdx - maxRows + 1
							}
						} else if toID != "" {
							// Otherwise, if prev's parent is toID, focus Parent.
							if prev, ok := m.db.FindItem(prevID); ok && prev != nil && prev.ParentID != nil && strings.TrimSpace(*prev.ParentID) == toID {
								m.itemFocus = itemFocusParent
							}
						}
					}
					return m, nil
				}
				if m.hasReturnView {
					(&m).returnFromItemView()
				} else {
					m.view = viewOutline
					m.openItemID = ""
					m.itemArchivedReadOnly = false
					m.showPreview = false
					m.pane = paneOutline
					if o, ok := m.db.FindOutline(m.selectedOutlineID); ok {
						m.refreshItems(*o)
					}
				}
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
					m.itemArchivedReadOnly = false
					m.itemFocus = itemFocusTitle
					m.itemCommentIdx = 0
					m.itemWorklogIdx = 0
					m.itemHistoryIdx = 0
					m.itemSideScroll = 0
					m.itemDetailScroll = 0
					m.hasReturnView = true
					m.returnView = viewAgenda
					m.showPreview = false
					m.pane = paneOutline
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
					m.itemArchivedReadOnly = true
					m.itemFocus = itemFocusTitle
					m.itemCommentIdx = 0
					m.itemWorklogIdx = 0
					m.itemHistoryIdx = 0
					m.itemSideScroll = 0
					m.itemDetailScroll = 0
					m.hasReturnView = true
					m.returnView = viewArchived
					m.showPreview = false
					m.pane = paneOutline
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
		itemID := strings.TrimSpace(m.openItemID)
		if itemID == "" {
			return m, nil
		}
		it, ok := m.db.FindItem(itemID)
		if !ok {
			return m, nil
		}
		readOnly := m.itemArchivedReadOnly || it.Archived

		parentID := ""
		hasParent := false
		if it.ParentID != nil {
			parentID = strings.TrimSpace(*it.ParentID)
			if parentID != "" {
				if _, ok := m.db.FindItem(parentID); ok {
					hasParent = true
				}
			}
		}

		comments := m.db.CommentsForItem(it.ID)
		worklog := m.db.WorklogForItem(it.ID)
		history := filterEventsForItem(m.db, m.eventsTail, it.ID)
		commentRows := buildCommentThreadRows(comments)
		attachmentRows := buildAttachmentPanelRows(m.db, *it)
		children := m.db.ChildrenOf(it.ID)
		sort.Slice(children, func(i, j int) bool { return compareOutlineItems(children[i], children[j]) < 0 })
		if m.itemChildIdx < 0 {
			m.itemChildIdx = 0
		}
		if len(children) == 0 {
			m.itemChildIdx = 0
			m.itemChildOff = 0
		} else if m.itemChildIdx >= len(children) {
			m.itemChildIdx = len(children) - 1
		}

		active := it
		activeID := strings.TrimSpace(it.ID)
		if m.itemFocus == itemFocusChildren && len(children) > 0 {
			activeID = strings.TrimSpace(children[m.itemChildIdx].ID)
			if it2, ok := m.db.FindItem(activeID); ok && it2 != nil {
				active = it2
			}
		}

		switch km.String() {
		case "up", "k", "ctrl+p":
			switch m.itemFocus {
			case itemFocusChildren:
				if len(children) > 0 && m.itemChildIdx > 0 {
					m.itemChildIdx--
					const maxRows = 8
					if m.itemChildOff < 0 {
						m.itemChildOff = 0
					}
					if m.itemChildIdx < m.itemChildOff {
						m.itemChildOff = m.itemChildIdx
					}
					maxOff := len(children) - maxRows
					if maxOff < 0 {
						maxOff = 0
					}
					if m.itemChildOff > maxOff {
						m.itemChildOff = maxOff
					}
				}
				return m, nil
			case itemFocusAttachments:
				if len(attachmentRows) > 0 && m.itemAttachmentIdx > 0 {
					m.itemAttachmentIdx--
					m.itemSideScroll = 0
				}
				return m, nil
			case itemFocusComments:
				if len(commentRows) > 0 && m.itemCommentIdx > 0 {
					m.itemCommentIdx--
					m.itemSideScroll = 0
				}
				return m, nil
			case itemFocusWorklog:
				if len(worklog) > 0 && m.itemWorklogIdx > 0 {
					m.itemWorklogIdx--
					m.itemSideScroll = 0
				}
				return m, nil
			case itemFocusHistory:
				if len(history) > 0 && m.itemHistoryIdx > 0 {
					m.itemHistoryIdx--
					m.itemSideScroll = 0
				}
				return m, nil
			}
		case "down", "j", "ctrl+n":
			switch m.itemFocus {
			case itemFocusChildren:
				if n := len(children); n > 0 && m.itemChildIdx < n-1 {
					m.itemChildIdx++
					const maxRows = 8
					if m.itemChildOff < 0 {
						m.itemChildOff = 0
					}
					if m.itemChildIdx >= m.itemChildOff+maxRows {
						m.itemChildOff = m.itemChildIdx - maxRows + 1
					}
					maxOff := len(children) - maxRows
					if maxOff < 0 {
						maxOff = 0
					}
					if m.itemChildOff > maxOff {
						m.itemChildOff = maxOff
					}
				}
				return m, nil
			case itemFocusAttachments:
				if n := len(attachmentRows); n > 0 && m.itemAttachmentIdx < n-1 {
					m.itemAttachmentIdx++
					m.itemSideScroll = 0
				}
				return m, nil
			case itemFocusComments:
				if n := len(commentRows); n > 0 && m.itemCommentIdx < n-1 {
					m.itemCommentIdx++
					m.itemSideScroll = 0
				}
				return m, nil
			case itemFocusWorklog:
				if n := len(worklog); n > 0 && m.itemWorklogIdx < n-1 {
					m.itemWorklogIdx++
					m.itemSideScroll = 0
				}
				return m, nil
			case itemFocusHistory:
				if n := len(history); n > 0 && m.itemHistoryIdx < n-1 {
					m.itemHistoryIdx++
					m.itemSideScroll = 0
				}
				return m, nil
			}
		case "pgup", "ctrl+u":
			m.itemDetailScroll -= 5
			if m.itemDetailScroll < 0 {
				m.itemDetailScroll = 0
			}
			return m, nil
		case "pgdown", "ctrl+d":
			m.itemDetailScroll += 5
			if m.itemDetailScroll < 0 {
				m.itemDetailScroll = 0
			}
			return m, nil
		case "home":
			switch m.itemFocus {
			case itemFocusAttachments, itemFocusComments, itemFocusWorklog, itemFocusHistory:
				// Jump to start of list (top).
				switch m.itemFocus {
				case itemFocusAttachments:
					if len(attachmentRows) > 0 {
						m.itemAttachmentIdx = 0
					}
				case itemFocusComments:
					if len(commentRows) > 0 {
						m.itemCommentIdx = 0
					}
				case itemFocusWorklog:
					if len(worklog) > 0 {
						m.itemWorklogIdx = 0 // worklog rendered newest-first
					}
				case itemFocusHistory:
					if len(history) > 0 {
						m.itemHistoryIdx = 0 // history rendered newest-first
					}
				}
				return m, nil
			}
		case "end":
			switch m.itemFocus {
			case itemFocusAttachments:
				if len(attachmentRows) > 0 {
					m.itemAttachmentIdx = len(attachmentRows) - 1
				}
				return m, nil
			case itemFocusComments:
				if len(commentRows) > 0 {
					m.itemCommentIdx = len(commentRows) - 1
				}
				return m, nil
			case itemFocusWorklog:
				if len(worklog) > 0 {
					m.itemWorklogIdx = len(worklog) - 1 // newest-first
				}
				return m, nil
			case itemFocusHistory:
				if len(history) > 0 {
					m.itemHistoryIdx = len(history) - 1 // newest-first
				}
				return m, nil
			}
		case "tab":
			m.itemFocus = m.itemFocus.nextForItem(hasParent)
			return m, nil
		case "shift+tab", "backtab":
			m.itemFocus = m.itemFocus.prevForItem(hasParent)
			return m, nil
		case "V":
			if _, err := (&m).duplicateItem(activeID, true); err != nil {
				return m, m.reportError(activeID, err)
			}
			return m, nil
		case "enter":
			// Archived view is read-only: allow opening children, but block mutations.
			if readOnly {
				switch m.itemFocus {
				case itemFocusTitle, itemFocusStatus, itemFocusPriority, itemFocusDescription:
					m.showMinibuffer("Archived item: read-only")
					return m, nil
				}
			}
			switch m.itemFocus {
			case itemFocusTitle:
				m.openInputModal(modalEditTitle, activeID, "Title", active.Title)
				return m, nil
			case itemFocusAttachments:
				if len(attachmentRows) == 0 {
					return m, nil
				}
				idx := m.itemAttachmentIdx
				if idx < 0 {
					idx = 0
				}
				if idx >= len(attachmentRows) {
					idx = len(attachmentRows) - 1
				}
				return m, m.openAttachment(attachmentRows[idx].Attachment)
			case itemFocusComments:
				if len(commentRows) == 0 || m.db == nil {
					return m, nil
				}
				idx := m.itemCommentIdx
				if idx < 0 {
					idx = 0
				}
				if idx >= len(commentRows) {
					idx = len(commentRows) - 1
				}
				c := commentRows[idx].Comment
				title := fmt.Sprintf("Comment — %s — %s", fmtTS(c.CreatedAt), actorLabel(m.db, c.AuthorID))
				(&m).openViewEntryModal(title, commentMarkdownWithAttachments(m.db, c))
				return m, nil
			case itemFocusWorklog:
				if len(worklog) == 0 || m.db == nil {
					return m, nil
				}
				idx := m.itemWorklogIdx
				if idx < 0 {
					idx = 0
				}
				if idx >= len(worklog) {
					idx = len(worklog) - 1
				}
				w := worklog[idx]
				title := fmt.Sprintf("My worklog — %s — %s", fmtTS(w.CreatedAt), actorLabel(m.db, w.AuthorID))
				(&m).openViewEntryModal(title, strings.TrimSpace(w.Body))
				return m, nil
			case itemFocusHistory:
				if len(history) == 0 || m.db == nil {
					return m, nil
				}
				idx := m.itemHistoryIdx
				if idx < 0 {
					idx = 0
				}
				if idx >= len(history) {
					idx = len(history) - 1
				}
				ev := history[idx]
				title := fmt.Sprintf("History — %s — %s", fmtTS(ev.TS), actorLabel(m.db, ev.ActorID))
				body := eventSummary(ev)
				if b, err := json.MarshalIndent(ev.Payload, "", "  "); err == nil && len(b) > 0 {
					body = body + "\n\n```json\n" + string(b) + "\n```"
				}
				(&m).openViewEntryModal(title, body)
				return m, nil
			case itemFocusStatus:
				if o, ok := m.db.FindOutline(active.OutlineID); ok {
					m.openStatusPicker(*o, activeID, active.StatusID)
					m.modal = modalPickStatus
					m.modalForID = activeID
				}
				return m, nil
			case itemFocusAssigned:
				m.openAssigneePicker(activeID)
				return m, nil
			case itemFocusTags:
				m.openTagsEditor(activeID)
				return m, nil
			case itemFocusPriority:
				// Toggle priority.
				if err := m.togglePriority(activeID); err != nil {
					return m, m.reportError(activeID, err)
				}
				return m, nil
			case itemFocusDescription:
				m.openTextModal(modalEditDescription, activeID, "Markdown description…", active.Description)
				return m, nil
			case itemFocusParent:
				if !hasParent || parentID == "" {
					return m, nil
				}
				// Record navigation so esc/backspace can return to the child item.
				if cur := strings.TrimSpace(it.ID); cur != "" && strings.TrimSpace(parentID) != "" && strings.TrimSpace(parentID) != cur {
					m.itemNavStack = append(m.itemNavStack, itemNavEntry{fromID: cur, toID: strings.TrimSpace(parentID)})
					if len(m.itemNavStack) > 64 {
						m.itemNavStack = m.itemNavStack[len(m.itemNavStack)-64:]
					}
				}
				// Navigate to the direct parent.
				m.openItemID = strings.TrimSpace(parentID)
				(&m).recordRecentItemVisit(m.openItemID)
				m.itemFocus = itemFocusTitle
				m.itemCommentIdx = 0
				m.itemWorklogIdx = 0
				m.itemHistoryIdx = 0
				m.itemSideScroll = 0
				m.itemDetailScroll = 0
				m.itemChildIdx = 0
				m.itemChildOff = 0
				return m, nil
			case itemFocusChildren:
				if len(children) == 0 || strings.TrimSpace(activeID) == "" {
					return m, nil
				}
				// Record navigation so esc/backspace can return to the parent item.
				if cur := strings.TrimSpace(it.ID); cur != "" && strings.TrimSpace(activeID) != "" && strings.TrimSpace(activeID) != cur {
					m.itemNavStack = append(m.itemNavStack, itemNavEntry{fromID: cur, toID: strings.TrimSpace(activeID)})
					if len(m.itemNavStack) > 64 {
						m.itemNavStack = m.itemNavStack[len(m.itemNavStack)-64:]
					}
				}
				// Navigate to the selected child.
				m.openItemID = strings.TrimSpace(activeID)
				(&m).recordRecentItemVisit(m.openItemID)
				m.itemFocus = itemFocusTitle
				m.itemCommentIdx = 0
				m.itemWorklogIdx = 0
				m.itemHistoryIdx = 0
				m.itemSideScroll = 0
				m.itemDetailScroll = 0
				m.itemChildIdx = 0
				m.itemChildOff = 0
				return m, nil
			default:
				return m, nil
			}
		case "l":
			// Open a picker for targets found in the selected markdown body.
			var md string
			loc := ""
			switch m.itemFocus {
			case itemFocusDescription:
				md = strings.TrimSpace(active.Description)
				loc = "description"
			case itemFocusComments:
				if len(commentRows) == 0 {
					return m, nil
				}
				idx := m.itemCommentIdx
				if idx < 0 {
					idx = 0
				}
				if idx >= len(commentRows) {
					idx = len(commentRows) - 1
				}
				md = commentMarkdownWithAttachments(m.db, commentRows[idx].Comment)
				loc = "comment"
			case itemFocusWorklog:
				if len(worklog) == 0 {
					return m, nil
				}
				idx := m.itemWorklogIdx
				if idx < 0 {
					idx = 0
				}
				if idx >= len(worklog) {
					idx = len(worklog) - 1
				}
				md = strings.TrimSpace(worklog[idx].Body)
				loc = "worklog"
			default:
				return m, nil
			}
			var targets []targetPickTarget
			if loc == "worklog" {
				// Worklog entries support URLs only (no attachment ids).
				targets = m.targetsForMarkdownLinksURLOnly(md)
			} else {
				targets = m.targetsForMarkdownLinks(md)
			}
			if len(targets) == 0 {
				m.showMinibuffer("Links: none")
				return m, nil
			}
			m.startTargetPicker("Links ("+loc+")", targets)
			return m, nil
		case "e":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			// When focusing Attachments, 'e' edits attachment metadata (instead of item title).
			if m.itemFocus == itemFocusAttachments {
				if len(attachmentRows) == 0 {
					return m, nil
				}
				idx := m.itemAttachmentIdx
				if idx < 0 {
					idx = 0
				}
				if idx >= len(attachmentRows) {
					idx = len(attachmentRows) - 1
				}
				a := attachmentRows[idx].Attachment
				m.attachmentEditID = strings.TrimSpace(a.ID)
				m.attachmentEditTitle = strings.TrimSpace(a.Title)
				m.attachmentEditAlt = strings.TrimSpace(a.Alt)
				m.openInputModal(modalEditAttachmentTitle, "", "Title (recommended)", m.attachmentEditTitle)
				return m, nil
			}
			if m.itemFocus == itemFocusChildren && strings.TrimSpace(activeID) != "" && strings.TrimSpace(activeID) != strings.TrimSpace(it.ID) {
				m.itemNavStack = append(m.itemNavStack, itemNavEntry{fromID: strings.TrimSpace(it.ID), toID: strings.TrimSpace(activeID)})
				if len(m.itemNavStack) > 64 {
					m.itemNavStack = m.itemNavStack[len(m.itemNavStack)-64:]
				}
				m.openItemID = strings.TrimSpace(activeID)
				(&m).recordRecentItemVisit(m.openItemID)
				it = active
				m.itemChildIdx = 0
				m.itemChildOff = 0
			}
			m.itemFocus = itemFocusTitle
			m.openInputModal(modalEditTitle, activeID, "Title", active.Title)
			return m, nil
		case "E":
			// Edit attachment metadata (attachments side panel).
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			if m.itemFocus != itemFocusAttachments || len(attachmentRows) == 0 {
				return m, nil
			}
			idx := m.itemAttachmentIdx
			if idx < 0 {
				idx = 0
			}
			if idx >= len(attachmentRows) {
				idx = len(attachmentRows) - 1
			}
			a := attachmentRows[idx].Attachment
			m.attachmentEditID = strings.TrimSpace(a.ID)
			m.attachmentEditTitle = strings.TrimSpace(a.Title)
			m.attachmentEditAlt = strings.TrimSpace(a.Alt)
			m.openInputModal(modalEditAttachmentTitle, "", "Title (recommended)", m.attachmentEditTitle)
			return m, nil
		case "D":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			if m.itemFocus == itemFocusChildren && strings.TrimSpace(activeID) != "" && strings.TrimSpace(activeID) != strings.TrimSpace(it.ID) {
				m.itemNavStack = append(m.itemNavStack, itemNavEntry{fromID: strings.TrimSpace(it.ID), toID: strings.TrimSpace(activeID)})
				if len(m.itemNavStack) > 64 {
					m.itemNavStack = m.itemNavStack[len(m.itemNavStack)-64:]
				}
				m.openItemID = strings.TrimSpace(activeID)
				(&m).recordRecentItemVisit(m.openItemID)
				it = active
				m.itemChildIdx = 0
				m.itemChildOff = 0
			}
			m.itemFocus = itemFocusDescription
			m.openTextModal(modalEditDescription, activeID, "Markdown description…", active.Description)
			return m, nil
		case "p":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			// Toggle priority.
			if err := m.togglePriority(activeID); err != nil {
				return m, m.reportError(activeID, err)
			}
			return m, nil
		case "o":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			// Toggle on-hold.
			if err := m.toggleOnHold(activeID); err != nil {
				return m, m.reportError(activeID, err)
			}
			return m, nil
		case "A":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			if strings.TrimSpace(activeID) == "" {
				return m, nil
			}
			m.itemFocus = itemFocusAssigned
			m.openAssigneePicker(activeID)
			return m, nil
		case "t":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			if strings.TrimSpace(activeID) == "" {
				return m, nil
			}
			m.itemFocus = itemFocusTags
			m.openTagsEditor(activeID)
			return m, nil
		case "d":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			if activeID == "" {
				return m, nil
			}
			var cur *model.DateTime
			if m.db != nil {
				if it, ok := m.db.FindItem(activeID); ok && it != nil {
					cur = it.Due
				}
			}
			m.openDateModal(modalSetDue, activeID, cur)
			return m, nil
		case "s":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			if activeID == "" {
				return m, nil
			}
			var cur *model.DateTime
			if m.db != nil {
				if it, ok := m.db.FindItem(activeID); ok && it != nil {
					cur = it.Schedule
				}
			}
			m.openDateModal(modalSetSchedule, activeID, cur)
			return m, nil
		case "C":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			if m.itemFocus == itemFocusChildren && strings.TrimSpace(activeID) != "" && strings.TrimSpace(activeID) != strings.TrimSpace(it.ID) {
				m.itemNavStack = append(m.itemNavStack, itemNavEntry{fromID: strings.TrimSpace(it.ID), toID: strings.TrimSpace(activeID)})
				if len(m.itemNavStack) > 64 {
					m.itemNavStack = m.itemNavStack[len(m.itemNavStack)-64:]
				}
				m.openItemID = strings.TrimSpace(activeID)
				(&m).recordRecentItemVisit(m.openItemID)
				it = active
				comments = m.db.CommentsForItem(it.ID)
				commentRows = buildCommentThreadRows(comments)
				m.itemChildIdx = 0
				m.itemChildOff = 0
			}
			// Add comment (keep the side panel open by focusing Comments).
			m.itemFocus = itemFocusComments
			if n := len(commentRows); n > 0 {
				m.itemCommentIdx = n - 1
			} else {
				m.itemCommentIdx = 0
			}
			m.itemSideScroll = 0
			m.openTextModal(modalAddComment, it.ID, "Write a comment…", "")
			return m, nil
		case "R":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			// Reply to selected comment (comments side panel).
			if m.itemFocus != itemFocusComments || len(commentRows) == 0 {
				return m, nil
			}
			idx := m.itemCommentIdx
			if idx < 0 {
				idx = 0
			}
			if idx >= len(commentRows) {
				idx = len(commentRows) - 1
			}
			parent := commentRows[idx].Comment
			parentID := strings.TrimSpace(parent.ID)
			if parentID == "" {
				return m, nil
			}
			quote := truncateInline(parent.Body, 280)
			m.replyQuoteMD = fmt.Sprintf("> %s  %s\n> %s", fmtTS(parent.CreatedAt), strings.TrimSpace(parent.AuthorID), quote)
			m.modal = modalReplyComment
			m.modalForID = it.ID
			m.modalForKey = parentID
			m.textFocus = textFocusBody
			m.textarea.SetValue("")
			m.textarea.Placeholder = "Write a reply…"
			m.textarea.Focus()
			return m, nil
		case "w":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			if m.itemFocus == itemFocusChildren && strings.TrimSpace(activeID) != "" && strings.TrimSpace(activeID) != strings.TrimSpace(it.ID) {
				m.itemNavStack = append(m.itemNavStack, itemNavEntry{fromID: strings.TrimSpace(it.ID), toID: strings.TrimSpace(activeID)})
				if len(m.itemNavStack) > 64 {
					m.itemNavStack = m.itemNavStack[len(m.itemNavStack)-64:]
				}
				m.openItemID = strings.TrimSpace(activeID)
				(&m).recordRecentItemVisit(m.openItemID)
				it = active
				m.itemChildIdx = 0
				m.itemChildOff = 0
			}
			// Add worklog entry (keep the side panel open by focusing My worklog).
			m.itemFocus = itemFocusWorklog
			m.itemWorklogIdx = 0
			m.itemSideScroll = 0
			m.openTextModal(modalAddWorklog, it.ID, "Log work…", "")
			return m, nil
		case "u":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}

			kind := "item"
			entityID := strings.TrimSpace(activeID)
			if m.itemFocus == itemFocusComments && len(commentRows) > 0 {
				idx := m.itemCommentIdx
				if idx < 0 {
					idx = 0
				}
				if idx >= len(commentRows) {
					idx = len(commentRows) - 1
				}
				cid := strings.TrimSpace(commentRows[idx].Comment.ID)
				if cid != "" {
					kind = "comment"
					entityID = cid
				}
			}
			if entityID == "" {
				return m, nil
			}

			m.itemFocus = itemFocusAttachments
			m.itemSideScroll = 0
			m.attachmentAddKind = kind
			m.attachmentAddEntityID = entityID
			m.attachmentAddPath = ""
			m.attachmentAddTitle = ""
			m.attachmentAddAlt = ""
			m.attachmentAddTitleHint = ""
			m.attachmentAddFlow = attachmentAddFlowCommit
			return m, m.openAttachmentFilePicker()
		case " ":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			m.itemFocus = itemFocusStatus
			if o, ok := m.db.FindOutline(active.OutlineID); ok {
				m.openStatusPicker(*o, activeID, active.StatusID)
				m.modal = modalPickStatus
				m.modalForID = activeID
			}
			return m, nil
		case "m":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			m.openMoveOutlinePicker(activeID)
			return m, nil
		case "r":
			if readOnly {
				m.showMinibuffer("Archived item: read-only")
				return m, nil
			}
			m.modal = modalConfirmArchive
			m.modalForID = activeID
			m.archiveFor = archiveTargetItem
			m.input.Blur()
			return m, nil
		}
	}
	return m, nil
}
