package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

type itemSidePanelKind int

const (
	itemSideNone itemSidePanelKind = iota
	itemSideAttachments
	itemSideComments
	itemSideWorklog
	itemSideHistory
)

func sidePanelKindForFocus(f itemPageFocus) itemSidePanelKind {
	switch f {
	case itemFocusAttachments:
		return itemSideAttachments
	case itemFocusComments:
		return itemSideComments
	case itemFocusWorklog:
		return itemSideWorklog
	case itemFocusHistory:
		return itemSideHistory
	default:
		return itemSideNone
	}
}

func renderItemSidePanelWithEvents(db *store.DB, it model.Item, width, height int, kind itemSidePanelKind, focused bool, attachmentIdx, commentIdx, worklogIdx, historyIdx int, scroll int, events []model.Event) string {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	if db == nil || kind == itemSideNone {
		return normalizePane(lipgloss.NewStyle().Width(width).Height(height).Render(""), width, height)
	}

	padX := 1
	innerW := width - (2 * padX)
	if innerW < 0 {
		innerW = 0
	}
	box := lipgloss.NewStyle().Width(innerW).Height(height).Padding(0, padX)

	headerStyle := styleMuted()
	// Match list selection highlight (outline + other lists): consistent fg/bg and a bit of weight.
	// When the panel is unfocused, avoid a second "active" highlight.
	focusRowStyle := lipgloss.NewStyle()
	if focused {
		focusRowStyle = lipgloss.NewStyle().
			Foreground(colorSelectedFg).
			Background(colorSelectedBg).
			Bold(true)
	}
	moreStyle := styleMuted()

	lines := []string{}
	switch kind {
	case itemSideAttachments:
		rows := buildAttachmentPanelRows(db, it)
		lines = append(lines, headerStyle.Render(fmt.Sprintf("Attachments (%d)", len(rows))))
		lines = append(lines, "")
		lines = append(lines, renderAttachmentPanelRows(rows, attachmentIdx, innerW, height-2, scroll, focusRowStyle, moreStyle)...)
	case itemSideComments:
		comments := db.CommentsForItem(it.ID)
		lines = append(lines, renderThreadedComments(db, comments, commentIdx, innerW, height, scroll, focusRowStyle, moreStyle)...)
	case itemSideWorklog:
		worklog := db.WorklogForItem(it.ID)
		lines = append(lines, renderAccordionWorklog(db, worklog, worklogIdx, innerW, height, scroll, focusRowStyle, moreStyle)...)
	case itemSideHistory:
		lines = append(lines, renderAccordionHistory(db, events, it.ID, historyIdx, innerW, height, scroll, focusRowStyle, moreStyle)...)
	}

	return normalizePane(box.Render(strings.Join(lines, "\n")), width, height)
}

func renderItemActivityColumns(db *store.DB, it model.Item, width, height int, focus itemPageFocus, commentIdx, worklogIdx, historyIdx int, scroll int, events []model.Event) string {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	gap := 1
	if width < 10 {
		gap = 0
	}
	avail := width - 2*gap
	if avail < 0 {
		avail = 0
	}
	w1 := avail / 3
	w2 := avail / 3
	w3 := avail - w1 - w2
	if w3 < 0 {
		w3 = 0
	}

	// Default focus to Comments so the right side is useful immediately.
	k := sidePanelKindForFocus(focus)
	if k != itemSideComments && k != itemSideWorklog && k != itemSideHistory {
		focus = itemFocusComments
	}

	comments := renderItemSidePanelWithEvents(db, it, w1, height, itemSideComments, focus == itemFocusComments, 0, commentIdx, 0, 0, scroll, events)
	worklog := renderItemSidePanelWithEvents(db, it, w2, height, itemSideWorklog, focus == itemFocusWorklog, 0, 0, worklogIdx, 0, scroll, events)
	history := renderItemSidePanelWithEvents(db, it, w3, height, itemSideHistory, focus == itemFocusHistory, 0, 0, 0, historyIdx, scroll, events)

	// Join horizontally with stable spacing.
	if gap > 0 {
		sep := strings.Repeat(" ", gap)
		return lipgloss.JoinHorizontal(lipgloss.Top, comments, sep, worklog, sep, history)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, comments, worklog, history)
}

func renderItemActivityTabbed(db *store.DB, it model.Item, width, height int, focus itemPageFocus, focused bool, commentIdx, worklogIdx int, scroll int) string {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	if db == nil {
		return normalizePane(lipgloss.NewStyle().Width(width).Height(height).Render(""), width, height)
	}

	// Default to Comments.
	k := sidePanelKindForFocus(focus)
	if k != itemSideComments && k != itemSideWorklog {
		k = itemSideComments
	}

	tab := func(label string, active bool) string {
		st := styleMuted()
		if active {
			if focused {
				st = lipgloss.NewStyle().Foreground(colorSelectedFg).Background(colorSelectedBg).Bold(true)
			} else {
				st = styleMuted().Bold(true)
			}
		}
		return st.Render(" " + label + " ")
	}

	header := lipgloss.JoinHorizontal(lipgloss.Left,
		tab("Comments", k == itemSideComments),
		" ",
		tab("My worklog", k == itemSideWorklog),
	)
	if xansi.StringWidth(header) > width {
		header = xansi.Cut(header, 0, width)
	}

	bodyH := height - 1
	if bodyH < 0 {
		bodyH = 0
	}
	var body string
	switch k {
	case itemSideWorklog:
		body = renderItemSidePanelWithEvents(db, it, width, bodyH, itemSideWorklog, focused && focus == itemFocusWorklog, 0, 0, worklogIdx, 0, scroll, nil)
	default:
		body = renderItemSidePanelWithEvents(db, it, width, bodyH, itemSideComments, focused && focus == itemFocusComments, 0, commentIdx, 0, 0, scroll, nil)
	}

	return normalizePane(header+"\n"+body, width, height)
}

type attachmentPanelRow struct {
	Attachment model.Attachment
	Comment    *model.Comment
}

func buildAttachmentPanelRows(db *store.DB, it model.Item) []attachmentPanelRow {
	if db == nil {
		return nil
	}
	var out []attachmentPanelRow

	for _, a := range db.AttachmentsForItem(it.ID) {
		out = append(out, attachmentPanelRow{Attachment: a})
	}
	return out
}

func renderAttachmentPanelRows(rows []attachmentPanelRow, selected int, width, height, scroll int, focusRowStyle lipgloss.Style, moreStyle lipgloss.Style) []string {
	if height < 1 {
		return []string{}
	}
	if len(rows) == 0 {
		return []string{"(no attachments)"}
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= len(rows) {
		selected = len(rows) - 1
	}
	if scroll < 0 {
		scroll = 0
	}

	lineFor := func(r attachmentPanelRow) string {
		a := r.Attachment
		name := strings.TrimSpace(a.Title)
		if name == "" {
			name = strings.TrimSpace(a.OriginalName)
		}
		if name == "" {
			name = a.ID
		}
		meta := fmt.Sprintf("%d bytes, %s", a.SizeBytes, strings.TrimSpace(a.ID))
		alt := truncateInline(strings.TrimSpace(a.Alt), maxInt(0, width-30))
		if alt != "" {
			name = name + " — " + alt
		}
		prefix := ""
		if r.Comment != nil {
			prefix = "cmt " + fmtTS(r.Comment.CreatedAt) + "  "
		}
		txt := prefix + name + "  (" + meta + ")"
		if xansi.StringWidth(txt) > width {
			txt = xansi.Cut(txt, 0, maxInt(0, width))
		}
		return txt
	}

	// Simple scrolling list (no expanded view yet).
	if scroll > len(rows)-height {
		scroll = maxInt(0, len(rows)-height)
	}
	win := rows
	if len(win) > height {
		end := scroll + height
		if end > len(rows) {
			end = len(rows)
		}
		win = rows[scroll:end]
	}

	out := make([]string, 0, len(win)+2)
	if scroll > 0 {
		out = append(out, moreStyle.Render(fmt.Sprintf("↑ %d more", scroll)))
	}
	for i := 0; i < len(win); i++ {
		idx := scroll + i
		line := lineFor(win[i])
		if idx == selected {
			line = focusRowStyle.Render(line)
		}
		out = append(out, line)
	}
	if scroll+len(win) < len(rows) {
		out = append(out, moreStyle.Render(fmt.Sprintf("↓ %d more", len(rows)-(scroll+len(win)))))
	}
	// Trim to height.
	if len(out) > height {
		out = out[:height]
	}
	return out
}

func renderThreadedComments(db *store.DB, comments []model.Comment, selected int, width, height, scroll int, focusRowStyle lipgloss.Style, moreStyle lipgloss.Style) []string {
	if height < 1 {
		return []string{}
	}
	if len(comments) == 0 {
		return []string{"(no comments)"}
	}

	rows := buildCommentThreadRows(comments)
	if len(rows) == 0 {
		return []string{"(no comments)"}
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= len(rows) {
		selected = len(rows) - 1
	}
	if scroll < 0 {
		scroll = 0
	}

	attachmentsCountByCommentID := map[string]int{}
	if db != nil {
		for i := range rows {
			cid := strings.TrimSpace(rows[i].Comment.ID)
			if cid == "" {
				continue
			}
			attachmentsCountByCommentID[cid] = len(db.AttachmentsForComment(cid))
		}
	}

	headerLineFor := func(r commentThreadRow) string {
		indent := strings.Repeat("  ", r.Depth)
		prefix := ""
		if r.Depth > 0 {
			prefix = "↳ "
		}
		actor := actorLabel(db, r.Comment.AuthorID)
		sfx := ""
		if n := attachmentsCountByCommentID[strings.TrimSpace(r.Comment.ID)]; n > 0 {
			sfx = fmt.Sprintf("  (%d att)", n)
		}
		return fmt.Sprintf("%s%s%s  %s%s", indent, prefix, fmtTS(r.Comment.CreatedAt), actor, sfx)
	}

	// Render all comments expanded (no "collapsed preview" list).
	all := make([]string, 0, maxInt(16, len(rows)*4))
	startLineByRow := make([]int, len(rows))

	for i, r := range rows {
		startLineByRow[i] = len(all)

		hdr := headerLineFor(r)
		if i == selected {
			hdr = focusRowStyle.Render(hdr)
		}
		all = append(all, hdr)

		c := r.Comment
		// Note: replies are stored as plain bodies; we keep the reply relationship via
		// ReplyToCommentID, but we do not inline quoted parent text in the rendering.
		md := strings.TrimSpace(c.Body)

		mdLines := strings.Split(renderMarkdownComment(md, maxInt(10, width-2)), "\n")
		filtered := make([]string, 0, len(mdLines))
		for _, ln := range mdLines {
			if strings.TrimSpace(stripANSIEscapes(ln)) == "" {
				continue
			}
			filtered = append(filtered, ln)
		}
		mdLines = filtered

		if db != nil {
			as := db.AttachmentsForComment(strings.TrimSpace(c.ID))
			if len(as) > 0 {
				mdLines = append(mdLines, "")
				mdLines = append(mdLines, "Attachments:")
				for _, a := range as {
					name := strings.TrimSpace(a.Title)
					if name == "" {
						name = strings.TrimSpace(a.OriginalName)
					}
					if name == "" {
						name = strings.TrimSpace(a.ID)
					}
					line := "- " + name + " (" + strings.TrimSpace(a.ID) + ")"
					if xansi.StringWidth(line) > width-2 {
						line = xansi.Cut(line, 0, maxInt(0, width-2))
					}
					mdLines = append(mdLines, line)
				}
			}
		}

		indent := strings.Repeat("  ", r.Depth)
		prefix := ""
		if r.Depth > 0 {
			prefix = "↳ "
		}
		// Align body with the header's date/author line by replacing the reply arrow with
		// spaces on body lines (so the body doesn't show the arrow, but keeps alignment).
		bodyPrefix := indent + strings.Repeat(" ", xansi.StringWidth(prefix))
		for _, ln := range mdLines {
			all = append(all, bodyPrefix+ln)
		}
		// Spacer between comments.
		if i < len(rows)-1 {
			all = append(all, "")
		}
	}

	if len(all) == 0 {
		return []string{"(no comments)"}
	}

	// Keep the selected comment header visible.
	selLine := startLineByRow[selected]
	if selLine < scroll {
		scroll = selLine
	}
	if selLine >= scroll+height {
		scroll = selLine - height + 1
	}
	if scroll < 0 {
		scroll = 0
	}
	maxScroll := len(all) - height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}

	win := all[scroll:]
	if len(win) > height {
		win = win[:height]
	}
	_ = moreStyle // kept for signature compatibility; comments now scroll as a full list
	return win
}

func findCommentByID(comments []model.Comment, id string) (model.Comment, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.Comment{}, false
	}
	for _, c := range comments {
		if strings.TrimSpace(c.ID) == id {
			return c, true
		}
	}
	return model.Comment{}, false
}

func renderAccordionWorklog(db *store.DB, worklog []model.WorklogEntry, selected int, width, height, scroll int, focusRowStyle lipgloss.Style, moreStyle lipgloss.Style) []string {
	if height < 1 {
		return []string{}
	}
	if len(worklog) == 0 {
		return []string{"(no worklog)"}
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= len(worklog) {
		selected = len(worklog) - 1
	}
	if scroll < 0 {
		scroll = 0
	}

	lineFor := func(w model.WorklogEntry) string {
		actor := actorLabel(db, w.AuthorID)
		snippet := truncateInline(w.Body, maxInt(20, width-26))
		return fmt.Sprintf("%s  %s  %s", fmtTS(w.CreatedAt), actor, snippet)
	}

	mdLines := strings.Split(renderMarkdownComment(strings.TrimSpace(worklog[selected].Body), maxInt(10, width-2)), "\n")
	// Glamour often emits "visually empty" spacer lines that contain only ANSI styling codes.
	// Those consume vertical space without adding information; drop them so expanded bodies
	// use available height for real content.
	filtered := make([]string, 0, len(mdLines))
	for _, ln := range mdLines {
		if strings.TrimSpace(stripANSIEscapes(ln)) == "" {
			continue
		}
		filtered = append(filtered, ln)
	}
	mdLines = filtered
	if scroll > len(mdLines) {
		scroll = len(mdLines)
	}

	mdH := height - 1
	if mdH < 1 {
		mdH = 1
	}
	var (
		start int
		end   int
		mdWin []string
	)
	for iter := 0; iter < 6; iter++ {
		if scroll > 0 && scroll > len(mdLines)-mdH {
			scroll = maxInt(0, len(mdLines)-mdH)
		}
		mdWin = mdLines
		if len(mdWin) > mdH {
			mdEnd := scroll + mdH
			if mdEnd > len(mdLines) {
				mdEnd = len(mdLines)
			}
			mdWin = mdLines[scroll:mdEnd]
		}

		selectedBlockLen := 1 + len(mdWin)
		remain := height - selectedBlockLen
		if remain < 0 {
			remain = 0
		}
		beforeN := remain / 2
		afterN := remain - beforeN

		start = selected - beforeN
		if start < 0 {
			start = 0
		}
		end = selected + 1 + afterN
		if end > len(worklog) {
			end = len(worklog)
		}

		outLen := (end - start - 1) + selectedBlockLen
		if start > 0 {
			outLen++
		}
		if end < len(worklog) {
			outLen++
		}
		overflow := outLen - height
		if overflow <= 0 {
			break
		}
		mdH -= overflow
		if mdH < 1 {
			mdH = 1
			break
		}
	}

	selectedBlock := []string{focusRowStyle.Render(lineFor(worklog[selected]))}
	for _, ln := range mdWin {
		selectedBlock = append(selectedBlock, ln)
	}

	out := []string{}
	if start > 0 {
		out = append(out, moreStyle.Render(fmt.Sprintf("↑ %d more", start)))
	}
	for i := start; i < end; i++ {
		if i == selected {
			out = append(out, selectedBlock...)
			continue
		}
		out = append(out, lineFor(worklog[i]))
	}
	if end < len(worklog) {
		out = append(out, moreStyle.Render(fmt.Sprintf("↓ %d more", len(worklog)-end)))
	}
	return out
}

func renderAccordionHistory(db *store.DB, events []model.Event, itemID string, selected int, width, height, scroll int, focusRowStyle lipgloss.Style, moreStyle lipgloss.Style) []string {
	if height < 1 {
		return []string{}
	}
	evs := filterEventsForItem(db, events, itemID)
	if len(evs) == 0 {
		return []string{"(no events)"}
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= len(evs) {
		selected = len(evs) - 1
	}
	if scroll < 0 {
		scroll = 0
	}

	ev := evs[selected]
	actor := actorLabel(db, ev.ActorID)

	payloadJSON := ""
	if m, err := json.MarshalIndent(ev.Payload, "", "  "); err == nil {
		payloadJSON = string(m)
	}
	md := strings.TrimSpace(fmt.Sprintf("**Type**: `%s`\n\n**When**: %s\n\n**Actor**: %s\n\n**Summary**: %s\n\n```json\n%s\n```",
		strings.TrimSpace(ev.Type),
		fmtTS(ev.TS),
		actor,
		eventSummary(ev),
		payloadJSON,
	))

	title := fmt.Sprintf("%s  %s  %s", fmtTS(ev.TS), actor, eventSummary(ev))
	mdLines := strings.Split(renderMarkdownComment(md, maxInt(10, width-2)), "\n")
	if scroll > len(mdLines) {
		scroll = len(mdLines)
	}

	mdH := height - 1
	if mdH < 1 {
		mdH = 1
	}
	var (
		start int
		end   int
		mdWin []string
	)
	for iter := 0; iter < 6; iter++ {
		if scroll > 0 && scroll > len(mdLines)-mdH {
			scroll = maxInt(0, len(mdLines)-mdH)
		}
		mdWin = mdLines
		if len(mdWin) > mdH {
			mdEnd := scroll + mdH
			if mdEnd > len(mdLines) {
				mdEnd = len(mdLines)
			}
			mdWin = mdLines[scroll:mdEnd]
		}

		selectedBlockLen := 1 + len(mdWin)
		remain := height - selectedBlockLen
		if remain < 0 {
			remain = 0
		}
		beforeN := remain / 2
		afterN := remain - beforeN

		start = selected - beforeN
		if start < 0 {
			start = 0
		}
		end = selected + 1 + afterN
		if end > len(evs) {
			end = len(evs)
		}

		outLen := (end - start - 1) + selectedBlockLen
		if start > 0 {
			outLen++
		}
		if end < len(evs) {
			outLen++
		}
		overflow := outLen - height
		if overflow <= 0 {
			break
		}
		mdH -= overflow
		if mdH < 1 {
			mdH = 1
			break
		}
	}

	selectedBlock := []string{focusRowStyle.Render(truncateInline(title, maxInt(20, width-2)))}
	for _, ln := range mdWin {
		selectedBlock = append(selectedBlock, ln)
	}

	out := []string{}
	if start > 0 {
		out = append(out, moreStyle.Render(fmt.Sprintf("↑ %d more", start)))
	}
	for i := start; i < end; i++ {
		if i == selected {
			out = append(out, selectedBlock...)
			continue
		}
		ev := evs[i]
		actor := actorLabel(db, ev.ActorID)
		out = append(out, fmt.Sprintf("%s  %s  %s", fmtTS(ev.TS), actor, truncateInline(eventSummary(ev), maxInt(20, width-26))))
	}
	if end < len(evs) {
		out = append(out, moreStyle.Render(fmt.Sprintf("↓ %d more", len(evs)-end)))
	}
	return out
}

func actorLabel(db *store.DB, actorID string) string {
	id := strings.TrimSpace(actorID)
	if id == "" || db == nil {
		return id
	}
	if a, ok := db.FindActor(id); ok {
		// Important: if an agent wrote it, show the agent identity (do NOT collapse to the owning human).
		if strings.TrimSpace(a.Name) != "" {
			return strings.TrimSpace(a.Name)
		}
	}
	return id
}

func filterEventsForItem(db *store.DB, events []model.Event, itemID string) []model.Event {
	id := strings.TrimSpace(itemID)
	if id == "" || len(events) == 0 {
		return nil
	}
	matches := make([]model.Event, 0, 32)
	for _, ev := range events {
		if eventIsForItem(ev, id) {
			matches = append(matches, ev)
		}
	}
	// Render newest-first like detail.go.
	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}
	return matches
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
