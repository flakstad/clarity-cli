package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	"github.com/charmbracelet/bubbles/list"
)

func (m *appModel) openCommentsListModal(itemID string) {
	if m == nil || m.db == nil {
		return
	}
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return
	}
	m.openActivityListModal(activityModalKindComments, itemID)
}

func (m *appModel) openWorklogListModal(itemID string) {
	if m == nil || m.db == nil {
		return
	}
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return
	}
	m.openActivityListModal(activityModalKindWorklog, itemID)
}

func (m *appModel) openHistoryModal(itemID string) {
	if m == nil || m.db == nil {
		return
	}
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return
	}
	m.openActivityListModal(activityModalKindHistory, itemID)
}

func (m *appModel) openModalForActivityRow(act outlineActivityRowItem) bool {
	if m == nil || m.db == nil {
		return false
	}
	itemID := strings.TrimSpace(act.itemID)
	if itemID == "" {
		itemID = strings.TrimSpace(m.openItemID)
	}
	switch act.kind {
	case outlineActivityCommentsRoot:
		m.openCommentsListModal(itemID)
		return true
	case outlineActivityComment:
		c, ok := findCommentByID(m.db.CommentsForItem(itemID), act.commentID)
		if !ok {
			m.showMinibuffer("Comment: not found")
			return true
		}
		title := fmt.Sprintf("Comment — %s — %s", fmtTS(c.CreatedAt), actorLabel(m.db, c.AuthorID))
		m.openViewEntryModal(title, commentMarkdownWithAttachments(m.db, c))
		return true
	case outlineActivityWorklogRoot:
		m.openWorklogListModal(itemID)
		return true
	case outlineActivityWorklogEntry:
		worklog := m.db.WorklogForItem(itemID)
		var w model.WorklogEntry
		found := false
		for i := range worklog {
			if strings.TrimSpace(worklog[i].ID) == strings.TrimSpace(act.worklogID) {
				w = worklog[i]
				found = true
				break
			}
		}
		if !found {
			m.showMinibuffer("Worklog: not found")
			return true
		}
		title := fmt.Sprintf("My worklog — %s — %s", fmtTS(w.CreatedAt), actorLabel(m.db, w.AuthorID))
		m.openViewEntryModal(title, strings.TrimSpace(w.Body))
		return true
	default:
		return false
	}
}

type activityModalRowKind int

const (
	activityModalRowComment activityModalRowKind = iota
	activityModalRowWorklog
	activityModalRowHistory
)

type activityModalRowItem struct {
	kind  activityModalRowKind
	title string

	itemID    string
	commentID string
	worklogID string
	event     *model.Event

	body string
}

func (i activityModalRowItem) Title() string       { return i.title }
func (i activityModalRowItem) Description() string { return "" }
func (i activityModalRowItem) FilterValue() string { return strings.TrimSpace(i.title) }

func (m *appModel) openActivityListModal(kind activityModalKind, itemID string) {
	if m == nil || m.db == nil {
		return
	}
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return
	}

	items := []list.Item{}
	switch kind {
	case activityModalKindComments:
		items = commentsModalItems(m.db, itemID)
	case activityModalKindWorklog:
		items = worklogModalItems(m.db, itemID)
	case activityModalKindHistory:
		items = historyModalItems(m.db, m.eventsTail, itemID)
	}
	if len(items) == 0 {
		items = []list.Item{activityModalRowItem{kind: activityModalRowHistory, title: "(empty)"}}
	}

	m.modal = modalActivityList
	m.activityModalKind = kind
	m.activityModalItemID = itemID

	_ = m.activityModalList.SetItems(items)
	m.activityModalList.Select(0)
	m.pendingEsc = false
	m.pendingCtrlX = false
}

func (m *appModel) renderActivityListModal() string {
	if m == nil {
		return ""
	}
	itemID := strings.TrimSpace(m.activityModalItemID)
	title := "Activity"
	switch m.activityModalKind {
	case activityModalKindComments:
		title = "Comments"
	case activityModalKindWorklog:
		title = "My worklog"
	case activityModalKindHistory:
		title = "History"
	}
	if itemID != "" {
		title += " — " + itemID
	}

	bodyW := modalBodyWidth(m.width)
	if bodyW < 10 {
		bodyW = 10
	}
	h := m.height - 12
	if h < 6 {
		h = 6
	}
	if h > 22 {
		h = 22
	}
	m.activityModalList.SetSize(bodyW, h)

	controls := "up/down: move   enter: open   esc/ctrl+g: close"
	content := m.activityModalList.View() + "\n\n" + styleMuted().Render(controls) + "\x1b[0m"
	return renderModalBox(m.width, title, content)
}

func (m *appModel) openViewEntryModalReturning(title, body string, ret modalKind) {
	if m == nil {
		return
	}
	m.openViewEntryModal(title, body)
	m.viewModalReturn = ret
}

func commentsModalItems(db *store.DB, itemID string) []list.Item {
	comments := db.CommentsForItem(itemID)
	rows := buildCommentThreadRows(comments)
	if len(rows) == 0 {
		return nil
	}

	items := make([]list.Item, 0, len(rows))
	for _, r := range rows {
		c := r.Comment
		body := strings.TrimSpace(c.Body)
		first := body
		if nl := strings.Index(first, "\n"); nl >= 0 {
			first = first[:nl]
		}
		first = strings.TrimSpace(first)
		indent := strings.Repeat("  ", max(0, r.Depth))
		label := fmt.Sprintf("%s  %s  %s", fmtTS(c.CreatedAt), actorLabel(db, c.AuthorID), truncateInline(first, 120))
		title := indent + label
		items = append(items, activityModalRowItem{
			kind:      activityModalRowComment,
			title:     title,
			itemID:    itemID,
			commentID: strings.TrimSpace(c.ID),
			body:      commentMarkdownWithAttachments(db, c),
		})
	}
	return items
}

func worklogModalItems(db *store.DB, itemID string) []list.Item {
	worklog := db.WorklogForItem(itemID)
	if len(worklog) == 0 {
		return nil
	}
	sort.SliceStable(worklog, func(i, j int) bool { return worklog[i].CreatedAt.After(worklog[j].CreatedAt) })

	items := make([]list.Item, 0, len(worklog))
	for _, w := range worklog {
		body := strings.TrimSpace(w.Body)
		first := body
		if nl := strings.Index(first, "\n"); nl >= 0 {
			first = first[:nl]
		}
		first = strings.TrimSpace(first)
		label := fmt.Sprintf("%s  %s  %s", fmtTS(w.CreatedAt), actorLabel(db, w.AuthorID), truncateInline(first, 120))
		items = append(items, activityModalRowItem{
			kind:      activityModalRowWorklog,
			title:     label,
			itemID:    itemID,
			worklogID: strings.TrimSpace(w.ID),
			body:      strings.TrimSpace(w.Body),
		})
	}
	return items
}

func historyModalItems(db *store.DB, events []model.Event, itemID string) []list.Item {
	history := filterEventsForItem(db, events, itemID)
	if len(history) == 0 {
		return nil
	}

	items := make([]list.Item, 0, len(history))
	for i := range history {
		ev := history[i]
		label := fmt.Sprintf("%s  %s  %s", fmtTS(ev.TS), actorLabel(db, ev.ActorID), eventSummary(ev))
		body := historyEventMarkdown(ev)
		evCopy := ev
		items = append(items, activityModalRowItem{
			kind:   activityModalRowHistory,
			title:  label,
			itemID: itemID,
			event:  &evCopy,
			body:   body,
		})
	}
	return items
}

func historyEventMarkdown(ev model.Event) string {
	payload, _ := json.MarshalIndent(ev.Payload, "", "  ")
	payloadStr := strings.TrimSpace(string(payload))
	if payloadStr == "" || payloadStr == "null" {
		payloadStr = "(none)"
	}
	lines := []string{
		"Type: " + strings.TrimSpace(ev.Type),
		"Actor: " + strings.TrimSpace(ev.ActorID),
		"At: " + fmtTS(ev.TS),
		"",
		"Payload:",
		"```json",
		payloadStr,
		"```",
	}
	return strings.Join(lines, "\n")
}
