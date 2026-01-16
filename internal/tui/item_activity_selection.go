package tui

import (
	"strings"

	"clarity-cli/internal/model"
)

func indexOfWorklogEntry(worklog []model.WorklogEntry, worklogID string) int {
	worklogID = strings.TrimSpace(worklogID)
	if worklogID == "" {
		return 0
	}
	for i := range worklog {
		if strings.TrimSpace(worklog[i].ID) == worklogID {
			return i
		}
	}
	if len(worklog) > 0 {
		return len(worklog) - 1
	}
	return 0
}

func (m *appModel) syncItemViewActivitySelection() {
	if m == nil || m.db == nil {
		return
	}
	act, ok := m.itemsList.SelectedItem().(outlineActivityRowItem)
	if !ok {
		return
	}

	itemID := strings.TrimSpace(act.itemID)
	if itemID == "" {
		itemID = strings.TrimSpace(m.openItemID)
	}
	if itemID == "" {
		return
	}

	switch act.kind {
	case outlineActivityCommentsRoot:
		m.itemFocus = itemFocusComments
		m.itemCommentIdx = 0
		m.itemSideScroll = 0
	case outlineActivityComment:
		m.itemFocus = itemFocusComments
		rows := buildCommentThreadRows(m.db.CommentsForItem(itemID))
		m.itemCommentIdx = indexOfCommentRow(rows, act.commentID)
		m.itemSideScroll = 0
	case outlineActivityWorklogRoot:
		m.itemFocus = itemFocusWorklog
		m.itemWorklogIdx = 0
		m.itemSideScroll = 0
	case outlineActivityWorklogEntry:
		m.itemFocus = itemFocusWorklog
		m.itemWorklogIdx = indexOfWorklogEntry(m.db.WorklogForItem(itemID), act.worklogID)
		m.itemSideScroll = 0
	}
}
