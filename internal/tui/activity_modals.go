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
		title := fmt.Sprintf("Comment — %s — %s", fmtTS(c.CreatedAt), actorAtLabel(m.db, c.AuthorID))
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
		title := fmt.Sprintf("My worklog — %s — %s", fmtTS(w.CreatedAt), actorAtLabel(m.db, w.AuthorID))
		m.openViewEntryModal(title, strings.TrimSpace(w.Body))
		return true
	case outlineActivityDepsRoot:
		// Enter toggles deps expansion in the outline list (read-only).
		m.toggleCollapseSelected()
		return true
	case outlineActivityDepEdge:
		otherID := strings.TrimSpace(act.depOtherItemID)
		if otherID == "" {
			m.showMinibuffer("Dep: item not found")
			return true
		}
		if err := m.jumpToItemByID(otherID); err != nil {
			m.showMinibuffer("Dep: " + err.Error())
		}
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

	m.modal = modalActivityList
	m.activityModalKind = kind
	m.activityModalItemID = itemID
	m.activityModalCollapsed = map[string]bool{}
	m.activityModalContentW = 0
	m.refreshActivityModalItems()
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
	if m.activityModalContentW != bodyW {
		m.activityModalContentW = bodyW
		m.refreshActivityModalItems()
	}

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
		label := fmt.Sprintf("%s %s %s", fmtTS(c.CreatedAt), actorAtLabel(db, c.AuthorID), truncateInline(first, 120))
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
		label := fmt.Sprintf("%s %s %s", fmtTS(w.CreatedAt), actorAtLabel(db, w.AuthorID), truncateInline(first, 120))
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
		label := fmt.Sprintf("%s %s %s", fmtTS(ev.TS), actorAtLabel(db, ev.ActorID), eventSummary(ev))
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

func (m *appModel) refreshActivityModalItems() {
	if m == nil || m.db == nil {
		return
	}
	itemID := strings.TrimSpace(m.activityModalItemID)
	if itemID == "" {
		return
	}
	if m.activityModalCollapsed == nil {
		m.activityModalCollapsed = map[string]bool{}
	}

	contentW := m.activityModalContentW
	if contentW <= 0 {
		contentW = 80
	}

	curSel := selectedOutlineListSelectionID(&m.activityModalList)

	items := []list.Item{}
	switch m.activityModalKind {
	case activityModalKindComments:
		items = buildActivityCommentItems(m.db, itemID, m.activityModalCollapsed, contentW)
	case activityModalKindWorklog:
		items = buildActivityWorklogItems(m.db, itemID, m.activityModalCollapsed, contentW)
	case activityModalKindHistory:
		items = buildActivityHistoryItems(m.db, m.eventsTail, itemID, m.activityModalCollapsed, contentW)
	default:
		items = []list.Item{outlineDescRowItem{parentID: itemID, depth: 0, line: "(empty)"}}
	}
	if len(items) == 0 {
		items = []list.Item{outlineDescRowItem{parentID: itemID, depth: 0, line: "(empty)"}}
	}

	_ = m.activityModalList.SetItems(items)

	if strings.TrimSpace(curSel) != "" {
		selectListItemByID(&m.activityModalList, curSel)
	} else {
		m.activityModalList.Select(0)
	}
}

func (m *appModel) toggleActivityModalCollapseSelected() {
	if m == nil {
		return
	}
	act, ok := m.activityModalList.SelectedItem().(outlineActivityRowItem)
	if !ok {
		return
	}
	if !act.hasChildren && !act.hasDescription {
		return
	}
	if m.activityModalCollapsed == nil {
		m.activityModalCollapsed = map[string]bool{}
	}
	collapsed := m.activityModalCollapsed

	switch act.kind {
	case outlineActivityCommentsRoot:
		comments := m.db.CommentsForItem(m.activityModalItemID)
		cids := make([]string, 0, len(comments))
		for _, c := range comments {
			cid := strings.TrimSpace(c.ID)
			if cid != "" {
				cids = append(cids, cid)
			}
		}
		anyExpanded := false
		allCollapsed := true
		for _, cid := range cids {
			if !collapsed[cid] {
				anyExpanded = true
				allCollapsed = false
			}
		}
		if collapsed[act.id] {
			collapsed[act.id] = false
			for _, cid := range cids {
				collapsed[cid] = true
			}
		} else if anyExpanded {
			collapsed[act.id] = true
			for _, cid := range cids {
				collapsed[cid] = true
			}
		} else if allCollapsed {
			for _, cid := range cids {
				collapsed[cid] = false
			}
		} else {
			collapsed[act.id] = true
		}

	case outlineActivityComment:
		comments := m.db.CommentsForItem(m.activityModalItemID)
		desc := commentDescendantIDs(comments, act.id)
		descExpanded := false
		for _, id := range desc {
			if !collapsed[id] {
				descExpanded = true
				break
			}
		}
		if collapsed[act.id] {
			collapsed[act.id] = false
			for _, id := range desc {
				collapsed[id] = true
			}
		} else {
			if len(desc) == 0 {
				collapsed[act.id] = true
			} else if descExpanded {
				collapsed[act.id] = true
				for _, id := range desc {
					collapsed[id] = true
				}
			} else {
				for _, id := range desc {
					collapsed[id] = false
				}
			}
		}

	case outlineActivityWorklogRoot:
		worklog := m.db.WorklogForItem(m.activityModalItemID)
		wids := make([]string, 0, len(worklog))
		for _, w := range worklog {
			wid := strings.TrimSpace(w.ID)
			if wid != "" {
				wids = append(wids, wid)
			}
		}
		anyExpanded := false
		allCollapsed := true
		for _, wid := range wids {
			if !collapsed[wid] {
				anyExpanded = true
				allCollapsed = false
			}
		}
		if collapsed[act.id] {
			collapsed[act.id] = false
			for _, wid := range wids {
				collapsed[wid] = true
			}
		} else if anyExpanded {
			collapsed[act.id] = true
			for _, wid := range wids {
				collapsed[wid] = true
			}
		} else if allCollapsed {
			for _, wid := range wids {
				collapsed[wid] = false
			}
		} else {
			collapsed[act.id] = true
		}

	case outlineActivityWorklogEntry:
		collapsed[act.id] = !collapsed[act.id]

	default:
		collapsed[act.id] = !collapsed[act.id]
	}

	m.refreshActivityModalItems()
	selectListItemByID(&m.activityModalList, act.id)
}

func (m *appModel) toggleActivityModalCollapseAll() {
	if m == nil {
		return
	}
	if m.activityModalCollapsed == nil {
		m.activityModalCollapsed = map[string]bool{}
	}
	items := m.activityModalList.Items()
	allCollapsed := true
	collapsibleIDs := make([]string, 0, len(items))
	for _, li := range items {
		act, ok := li.(outlineActivityRowItem)
		if !ok {
			continue
		}
		if !act.hasChildren && !act.hasDescription {
			continue
		}
		collapsibleIDs = append(collapsibleIDs, act.id)
		if !m.activityModalCollapsed[act.id] {
			allCollapsed = false
		}
	}
	if len(collapsibleIDs) == 0 {
		return
	}
	next := true // collapse
	if allCollapsed {
		next = false // expand all
	}
	for _, id := range collapsibleIDs {
		m.activityModalCollapsed[id] = next
	}
	sel := selectedOutlineListSelectionID(&m.activityModalList)
	m.refreshActivityModalItems()
	if sel != "" {
		selectListItemByID(&m.activityModalList, sel)
	}
}

func buildActivityCommentItems(db *store.DB, itemID string, collapsed map[string]bool, contentW int) []list.Item {
	comments := db.CommentsForItem(itemID)
	rows := buildCommentThreadRows(comments)
	if len(rows) == 0 {
		return nil
	}

	commentKids := map[string]int{}
	for _, c := range comments {
		if c.ReplyToCommentID == nil {
			continue
		}
		p := strings.TrimSpace(*c.ReplyToCommentID)
		if p == "" {
			continue
		}
		commentKids[p]++
	}

	out := make([]list.Item, 0, len(rows)*2)
	skipDepth := -1
	for _, r := range rows {
		if skipDepth >= 0 && r.Depth > skipDepth {
			continue
		}
		if skipDepth >= 0 && r.Depth <= skipDepth {
			skipDepth = -1
		}

		c := r.Comment
		cid := strings.TrimSpace(c.ID)
		if cid == "" {
			continue
		}
		hasChildren := commentKids[cid] > 0
		bodyMD := strings.TrimSpace(commentMarkdownWithAttachments(db, c))
		hasDescription := bodyMD != ""

		if hasChildren || hasDescription {
			if _, ok := collapsed[cid]; !ok {
				collapsed[cid] = true
			}
		}

		label := fmt.Sprintf("%s %s", fmtTS(c.CreatedAt), actorAtLabel(db, c.AuthorID))
		out = append(out, outlineActivityRowItem{
			id:             cid,
			itemID:         itemID,
			kind:           outlineActivityComment,
			depth:          max(0, r.Depth),
			label:          label,
			commentID:      cid,
			hasChildren:    hasChildren,
			hasDescription: hasDescription,
			collapsed:      collapsed[cid],
		})

		if hasDescription && !collapsed[cid] {
			descDepth := max(0, r.Depth)
			leadW := (2 * descDepth) + 2
			avail := contentW - leadW
			if avail < 0 {
				avail = 0
			}
			descLines := outlineDescriptionLinesMarkdown(bodyMD, avail)
			for _, line := range descLines {
				out = append(out, outlineDescRowItem{parentID: cid, depth: descDepth, line: line})
			}
		}

		if (hasChildren || hasDescription) && collapsed[cid] {
			skipDepth = r.Depth
		}
	}
	return out
}

func buildActivityWorklogItems(db *store.DB, itemID string, collapsed map[string]bool, contentW int) []list.Item {
	worklog := db.WorklogForItem(itemID)
	if len(worklog) == 0 {
		return nil
	}
	sort.SliceStable(worklog, func(i, j int) bool { return worklog[i].CreatedAt.After(worklog[j].CreatedAt) })

	out := make([]list.Item, 0, len(worklog)*2)
	for _, w := range worklog {
		wid := strings.TrimSpace(w.ID)
		if wid == "" {
			continue
		}
		body := strings.TrimSpace(w.Body)
		hasDescription := body != ""
		if hasDescription {
			if _, ok := collapsed[wid]; !ok {
				collapsed[wid] = true
			}
		}

		label := fmt.Sprintf("%s %s", fmtTS(w.CreatedAt), actorAtLabel(db, w.AuthorID))
		out = append(out, outlineActivityRowItem{
			id:             wid,
			itemID:         itemID,
			kind:           outlineActivityWorklogEntry,
			depth:          0,
			label:          label,
			worklogID:      wid,
			hasDescription: hasDescription,
			collapsed:      collapsed[wid],
		})

		if hasDescription && !collapsed[wid] {
			descDepth := 0
			leadW := (2 * descDepth) + 2
			avail := contentW - leadW
			if avail < 0 {
				avail = 0
			}
			descLines := outlineDescriptionLinesMarkdown(body, avail)
			for _, line := range descLines {
				out = append(out, outlineDescRowItem{parentID: wid, depth: descDepth, line: line})
			}
		}
	}
	return out
}

func buildActivityHistoryItems(db *store.DB, events []model.Event, itemID string, collapsed map[string]bool, contentW int) []list.Item {
	history := filterEventsForItem(db, events, itemID)
	if len(history) == 0 {
		return nil
	}

	out := make([]list.Item, 0, len(history)*2)
	for _, ev := range history {
		eid := strings.TrimSpace(ev.ID)
		if eid == "" {
			continue
		}
		body := strings.TrimSpace(historyEventMarkdown(ev))
		hasDescription := body != ""
		if hasDescription {
			if _, ok := collapsed[eid]; !ok {
				collapsed[eid] = true
			}
		}
		label := fmt.Sprintf("%s %s %s", fmtTS(ev.TS), actorAtLabel(db, ev.ActorID), eventSummary(ev))
		out = append(out, outlineActivityRowItem{
			id:             eid,
			itemID:         itemID,
			kind:           outlineActivityHistoryEntry,
			depth:          0,
			label:          label,
			eventID:        eid,
			hasDescription: hasDescription,
			collapsed:      collapsed[eid],
		})
		if hasDescription && !collapsed[eid] {
			descDepth := 0
			leadW := (2 * descDepth) + 2
			avail := contentW - leadW
			if avail < 0 {
				avail = 0
			}
			descLines := outlineDescriptionLinesMarkdown(body, avail)
			for _, line := range descLines {
				out = append(out, outlineDescRowItem{parentID: eid, depth: descDepth, line: line})
			}
		}
	}
	return out
}
