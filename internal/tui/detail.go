package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

func renderItemDetail(db *store.DB, outline model.Outline, it model.Item, width, height int, focused bool, events []model.Event) string {
	titleStyle := lipgloss.NewStyle().Bold(true)
	if isEndState(outline, it.StatusID) {
		titleStyle = faintIfDark(lipgloss.NewStyle()).
			Foreground(colorMuted).
			Strikethrough(true).
			Bold(true)
	}
	labelStyle := styleMuted()
	// NOTE: The returned string must be exactly `width` columns wide (ANSI-aware) so that
	// split-view rendering with lipgloss.JoinHorizontal stays stable.
	padX := 1
	innerW := width - (2 * padX)
	if innerW < 0 {
		innerW = 0
	}
	box := lipgloss.NewStyle().
		Width(innerW).
		Height(height).
		Padding(0, padX)

	status := renderStatus(outline, it.StatusID)
	createdBy := "-"
	if strings.TrimSpace(it.CreatedBy) != "" {
		createdBy = "@" + actorDetailLabel(db, it.CreatedBy)
	}
	assigned := "-"
	if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) != "" {
		assigned = "@" + actorDetailLabel(db, *it.AssignedActorID)
	}
	tags := "-"
	if len(it.Tags) > 0 {
		cleaned := make([]string, 0, len(it.Tags))
		for _, t := range it.Tags {
			t = normalizeTag(t)
			if t == "" {
				continue
			}
			cleaned = append(cleaned, t)
		}
		cleaned = uniqueSortedStrings(cleaned)
		if len(cleaned) > 0 {
			parts := make([]string, 0, len(cleaned))
			for _, t := range cleaned {
				parts = append(parts, "#"+t)
			}
			tags = strings.Join(parts, " ")
		}
	}

	// Direct parent (shown when viewing a child).
	var parent *model.Item
	if it.ParentID != nil && strings.TrimSpace(*it.ParentID) != "" {
		if p, ok := db.FindItem(strings.TrimSpace(*it.ParentID)); ok {
			parent = p
		}
	}

	// Direct children (shown to support outline-style nesting).
	children := db.ChildrenOf(it.ID)
	sort.Slice(children, func(i, j int) bool { return compareOutlineItems(children[i], children[j]) < 0 })

	comments := db.CommentsForItem(it.ID)
	commentsCount := len(comments)
	lastComment := "-"
	if commentsCount > 0 {
		lastComment = fmtTS(comments[0].CreatedAt)
	}

	attRows := buildAttachmentPanelRows(db, it)
	lastAttachment := "-"
	if len(attRows) > 0 {
		latest := attRows[0].Attachment.CreatedAt
		for i := 1; i < len(attRows); i++ {
			if attRows[i].Attachment.CreatedAt.After(latest) {
				latest = attRows[i].Attachment.CreatedAt
			}
		}
		lastAttachment = fmtTS(latest)
	}

	worklog := db.WorklogForItem(it.ID)
	// WorklogForItem is sorted by CreatedAt desc; keep stable ordering in case of ties.
	sort.SliceStable(worklog, func(i, j int) bool { return worklog[i].CreatedAt.After(worklog[j].CreatedAt) })
	lastWorklog := "-"
	if len(worklog) > 0 {
		lastWorklog = fmtTS(worklog[0].CreatedAt)
	}

	history := filterEventsForItem(db, events, it.ID)
	lastHistory := "-"
	if len(history) > 0 {
		lastHistory = fmtTS(history[0].TS)
	}

	desc := "(no description)"
	if strings.TrimSpace(it.Description) != "" {
		rendered := strings.TrimSpace(renderMarkdown(it.Description, innerW))
		if rendered == "" {
			rendered = strings.TrimSpace(it.Description)
		}
		maxDescLines := height / 2
		if maxDescLines < 6 {
			maxDescLines = 6
		}
		if maxDescLines > 24 {
			maxDescLines = 24
		}
		desc = truncateLines(rendered, maxDescLines)
	}

	lines := []string{
		titleStyle.Render(it.Title),
		"",
		labelStyle.Render("ID: ") + it.ID,
		labelStyle.Render("Created by: ") + createdBy,
		labelStyle.Render("Assigned: ") + assigned,
		labelStyle.Render("Tags: ") + tags,
		labelStyle.Render("Priority: ") + fmt.Sprintf("%v", it.Priority),
		labelStyle.Render("On hold: ") + fmt.Sprintf("%v", it.OnHold),
		"",
		labelStyle.Render("Description"),
		desc,
		"",
	}
	if commentsCount > 0 {
		if preview := commentThreadPreviewLines(db, comments, innerW, 1, 6); len(preview) > 0 {
			lines = append(lines,
				labelStyle.Render("Comments"),
				strings.Join(preview, "\n"),
				"",
			)
		}
	}
	if parent != nil {
		lines = append(lines,
			labelStyle.Render("Parent"),
			renderParentOutline(db, outline, *parent, innerW, false),
			"",
		)
	}
	lines = append(lines,
		labelStyle.Render("Children"),
		renderChildrenOutline(db, outline, children, innerW, false, 0, 0, 8),
		"",
		labelStyle.Render("Related"),
		fmt.Sprintf("Attachments: %d (last %s)", len(attRows), lastAttachment),
		fmt.Sprintf("Comments: %d (last %s)", commentsCount, lastComment),
		fmt.Sprintf("My worklog: %d (last %s)", len(worklog), lastWorklog),
		fmt.Sprintf("History:   %d (last %s)", len(history), lastHistory),
	)

	if strings.TrimSpace(status) != "" {
		// Insert status after ID line.
		lines = append(lines[:4], append([]string{labelStyle.Render("Status: ") + status}, lines[4:]...)...)
	}

	// Normalize to guarantee stable split-pane rendering even with unbroken long tokens.
	return normalizePane(box.Render(strings.Join(lines, "\n")), width, height)
}

func renderItemDetailInteractive(db *store.DB, outline model.Outline, it model.Item, width, height int, focus itemPageFocus, events []model.Event, childIdx int, childOff int, attachmentIdx int, commentIdx int, worklogIdx int, historyIdx int, scroll int) string {
	titleStyle := lipgloss.NewStyle().Bold(true)
	if isEndState(outline, it.StatusID) {
		titleStyle = faintIfDark(lipgloss.NewStyle()).
			Foreground(colorMuted).
			Strikethrough(true).
			Bold(true)
	}
	labelStyle := styleMuted()
	// NOTE: The returned string must be exactly `width` columns wide (ANSI-aware) so that
	// split-view rendering with lipgloss.JoinHorizontal stays stable.
	padX := 1
	innerW := width - (2 * padX)
	if innerW < 0 {
		innerW = 0
	}
	box := lipgloss.NewStyle().
		Width(innerW).
		Height(height).
		Padding(0, padX)

	btnBase := lipgloss.NewStyle().
		Padding(0, 1).
		Foreground(colorSurfaceFg).
		Background(colorControlBg).
		MaxWidth(innerW)
	btnActive := btnBase.
		Foreground(colorSelectedFg).
		Background(colorSelectedBg).
		Bold(true)
	btn := func(active bool) lipgloss.Style {
		if active {
			return btnActive
		}
		return btnBase
	}

	status := renderStatus(outline, it.StatusID)
	createdBy := "-"
	if strings.TrimSpace(it.CreatedBy) != "" {
		createdBy = "@" + actorDetailLabel(db, it.CreatedBy)
	}
	assigned := "-"
	if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) != "" {
		assigned = "@" + actorDetailLabel(db, *it.AssignedActorID)
	}
	tags := "-"
	if len(it.Tags) > 0 {
		cleaned := make([]string, 0, len(it.Tags))
		for _, t := range it.Tags {
			t = normalizeTag(t)
			if t == "" {
				continue
			}
			cleaned = append(cleaned, t)
		}
		cleaned = uniqueSortedStrings(cleaned)
		if len(cleaned) > 0 {
			parts := make([]string, 0, len(cleaned))
			for _, t := range cleaned {
				parts = append(parts, "#"+t)
			}
			tags = strings.Join(parts, " ")
		}
	}

	// Direct parent (shown when viewing a child).
	var parent *model.Item
	if it.ParentID != nil && strings.TrimSpace(*it.ParentID) != "" {
		if p, ok := db.FindItem(strings.TrimSpace(*it.ParentID)); ok {
			parent = p
		}
	}

	// Direct children (shown to support outline-style nesting).
	children := db.ChildrenOf(it.ID)
	sort.Slice(children, func(i, j int) bool { return compareOutlineItems(children[i], children[j]) < 0 })

	comments := db.CommentsForItem(it.ID)
	commentsCount := len(comments)
	commentRows := buildCommentThreadRows(comments)
	if commentIdx < 0 {
		commentIdx = 0
	}
	if commentIdx >= len(commentRows) && len(commentRows) > 0 {
		commentIdx = len(commentRows) - 1
	}

	attRows := buildAttachmentPanelRows(db, it)
	if attachmentIdx < 0 {
		attachmentIdx = 0
	}
	if attachmentIdx >= len(attRows) && len(attRows) > 0 {
		attachmentIdx = len(attRows) - 1
	}

	worklog := db.WorklogForItem(it.ID)
	// WorklogForItem is sorted by CreatedAt desc; keep stable ordering in case of ties.
	sort.SliceStable(worklog, func(i, j int) bool { return worklog[i].CreatedAt.After(worklog[j].CreatedAt) })
	if worklogIdx < 0 {
		worklogIdx = 0
	}
	if worklogIdx >= len(worklog) && len(worklog) > 0 {
		worklogIdx = len(worklog) - 1
	}

	history := filterEventsForItem(db, events, it.ID)
	if historyIdx < 0 {
		historyIdx = 0
	}
	if historyIdx >= len(history) && len(history) > 0 {
		historyIdx = len(history) - 1
	}

	titleBtn := btn(focus == itemFocusTitle).Render(titleStyle.Render(it.Title))
	parentBtn := btn(focus == itemFocusParent).Render("Parent")
	childrenBtn := btn(focus == itemFocusChildren).Render("Children")
	attachmentsBtn := btn(focus == itemFocusAttachments).Render("Attachments")
	worklogBtn := btn(focus == itemFocusWorklog).Render("My worklog")
	historyBtn := btn(focus == itemFocusHistory).Render("History")

	headerLines := []string{
		titleBtn,
		"",
		labelStyle.Render("ID: ") + it.ID,
		labelStyle.Render("Created by: ") + createdBy,
		labelStyle.Render("Assigned: ") + btn(focus == itemFocusAssigned).Render(assigned),
		labelStyle.Render("Tags: ") + btn(focus == itemFocusTags).Render(tags),
		labelStyle.Render("Priority: ") + btn(focus == itemFocusPriority).Render(fmt.Sprintf("%v", it.Priority)),
		labelStyle.Render("On hold: ") + fmt.Sprintf("%v", it.OnHold),
		"",
	}

	if strings.TrimSpace(status) != "" {
		// Insert status after ID line.
		statusLine := labelStyle.Render("Status: ") + btn(focus == itemFocusStatus).Render(status)
		headerLines = append(headerLines[:4], append([]string{statusLine}, headerLines[4:]...)...)
	}

	descLines := []string{"(no description)"}
	if strings.TrimSpace(it.Description) != "" {
		rendered := strings.TrimSpace(renderMarkdown(it.Description, innerW))
		if rendered == "" {
			rendered = strings.TrimSpace(it.Description)
		}
		descLines = strings.Split(rendered, "\n")
		if len(descLines) == 0 {
			descLines = []string{"(no description)"}
		}
		// Guardrails: avoid pathological render sizes.
		if len(descLines) > 800 {
			descLines = append(descLines[:799], "…")
		}
	}

	focusRowStyle := lipgloss.NewStyle().
		Foreground(colorSelectedFg).
		Background(colorSelectedBg).
		Bold(true)

	renderCommentsInline := func() []string {
		if len(commentRows) == 0 {
			return []string{"(no comments)"}
		}
		out := make([]string, 0, len(commentRows)*8)

		authorStyle := lipgloss.NewStyle().Bold(true).Foreground(colorSurfaceFg)
		tsStyle := styleMuted()
		attStyle := styleMuted()
		focusBg := lipgloss.NewStyle().Background(colorSelectedBg)

		// Local helper: keep lines tight, never pad; cut if needed.
		cutLine := func(s string, w int) string {
			s = strings.TrimRight(s, " ")
			if w <= 0 {
				return ""
			}
			if xansi.StringWidth(s) > w {
				return xansi.Cut(s, 0, w) + "\x1b[0m"
			}
			return s
		}

		for i, r := range commentRows {
			depth := r.Depth
			if depth < 0 {
				depth = 0
			}
			if depth > 12 {
				depth = 12
			}
			indentCols := 2 * depth
			indent := strings.Repeat(" ", indentCols)
			contentW := maxInt(10, innerW-indentCols)

			author := authorStyle.Render(actorLabel(db, r.Comment.AuthorID))
			ts := tsStyle.Render(fmtTS(r.Comment.CreatedAt))
			metaPrefix := ""
			if depth > 0 {
				// Reply indicator: down-right arrow.
				metaPrefix = "↳ "
			}
			metaLine := metaPrefix + author + "  " + ts

			attLines := []string(nil)
			if db != nil {
				as := db.AttachmentsForComment(strings.TrimSpace(r.Comment.ID))
				if len(as) > 0 {
					attLines = make([]string, 0, len(as))
					for _, a := range as {
						name := strings.TrimSpace(a.Title)
						if name == "" {
							name = strings.TrimSpace(a.OriginalName)
						}
						if name == "" {
							name = strings.TrimSpace(a.ID)
						}
						alt := strings.TrimSpace(a.Alt)
						if alt != "" {
							name = name + " — " + alt
						}
						attLines = append(attLines, fmt.Sprintf("- %s  (%d bytes, %s)", name, a.SizeBytes, strings.TrimSpace(a.ID)))
					}
				}
			}

			selected := focus == itemFocusComments && i == commentIdx

			block := make([]string, 0, 6)
			block = append(block, metaLine)

			bodyMD := strings.TrimSpace(r.Comment.Body)
			if bodyMD != "" {
				bodyRendered := strings.TrimRight(renderMarkdownCompact(bodyMD, contentW), "\n")
				if strings.TrimSpace(bodyRendered) == "" {
					bodyRendered = bodyMD
				}
				bodyLines := strings.Split(bodyRendered, "\n")
				// Keep dense: collapse consecutive blank lines and trim ends.
				collapsed := make([]string, 0, len(bodyLines))
				blank := false
				for _, ln := range bodyLines {
					if strings.TrimSpace(xansi.Strip(ln)) == "" {
						if blank {
							continue
						}
						blank = true
						collapsed = append(collapsed, "")
						continue
					}
					blank = false
					collapsed = append(collapsed, ln)
				}
				for len(collapsed) > 0 && strings.TrimSpace(xansi.Strip(collapsed[0])) == "" {
					collapsed = collapsed[1:]
				}
				for len(collapsed) > 0 && strings.TrimSpace(xansi.Strip(collapsed[len(collapsed)-1])) == "" {
					collapsed = collapsed[:len(collapsed)-1]
				}
				for _, ln := range collapsed {
					block = append(block, ln)
				}
			}

			if len(attLines) > 0 {
				block = append(block, attStyle.Render(fmt.Sprintf("Attachments (%d)", len(attLines))))
				for _, ln := range attLines {
					block = append(block, attStyle.Render(truncateText(strings.TrimSpace(ln), contentW)))
				}
			}

			for _, ln := range block {
				ln = cutLine(ln, contentW)
				ln = indent + ln
				ln = cutLine(ln, innerW)
				if selected {
					ln = focusBg.Render(ln)
				}
				out = append(out, ln)
			}
			// Minimal vertical spacing between comments.
			if i < len(commentRows)-1 {
				out = append(out, "")
			}
		}
		return out
	}

	bodyLines := []string{
		parentBtn,
	}
	if parent != nil {
		bodyLines = append(bodyLines, strings.Split(renderParentOutline(db, outline, *parent, innerW, focus == itemFocusParent), "\n")...)
	} else {
		bodyLines = append(bodyLines, styleMuted().Render("(no parent)"))
	}
	bodyLines = append(bodyLines, "")
	bodyLines = append(bodyLines, childrenBtn)
	bodyLines = append(bodyLines, strings.Split(renderChildrenOutline(db, outline, children, innerW, focus == itemFocusChildren, childIdx, childOff, 8), "\n")...)
	bodyLines = append(bodyLines, "")

	bodyLines = append(bodyLines, btn(focus == itemFocusDescription).Render("Description (edit)"))
	bodyLines = append(bodyLines, descLines...)
	bodyLines = append(bodyLines, "")

	bodyLines = append(bodyLines, attachmentsBtn)
	if len(attRows) == 0 {
		bodyLines = append(bodyLines, "(no attachments)")
	} else {
		for i := range attRows {
			a := attRows[i].Attachment
			name := strings.TrimSpace(a.Title)
			if name == "" {
				name = strings.TrimSpace(a.OriginalName)
			}
			if name == "" {
				name = strings.TrimSpace(a.ID)
			}
			alt := truncateInline(strings.TrimSpace(a.Alt), maxInt(0, innerW-30))
			if alt != "" {
				name = name + " — " + alt
			}
			txt := fmt.Sprintf("%s  (%d bytes, %s)", name, a.SizeBytes, strings.TrimSpace(a.ID))
			if i == attachmentIdx && focus == itemFocusAttachments {
				txt = focusRowStyle.Render(txt)
			}
			bodyLines = append(bodyLines, fixedWidthLine(txt, innerW))
		}
	}
	bodyLines = append(bodyLines, "")

	bodyLines = append(bodyLines, btn(focus == itemFocusComments).Render(fmt.Sprintf("Comments (%d)  C: new  R: reply", commentsCount)))
	bodyLines = append(bodyLines, renderCommentsInline()...)
	bodyLines = append(bodyLines, "")

	bodyLines = append(bodyLines, worklogBtn)
	if len(worklog) == 0 {
		bodyLines = append(bodyLines, "(no worklog)")
	} else {
		for i := range worklog {
			w := worklog[i]
			metaPlain := fmt.Sprintf("%s  %s", fmtTS(w.CreatedAt), actorLabel(db, w.AuthorID))
			txt := fmt.Sprintf("%s  %s", metaPlain, truncateInline(w.Body, maxInt(20, innerW-26)))
			st := styleMuted()
			if focus == itemFocusWorklog && i == worklogIdx {
				st = st.Copy().Background(colorSelectedBg)
			}
			bodyLines = append(bodyLines, fixedWidthLine(st.Render(txt), innerW))
		}
	}
	bodyLines = append(bodyLines, "")

	bodyLines = append(bodyLines, historyBtn)
	if len(history) == 0 {
		bodyLines = append(bodyLines, "(no history)")
	} else {
		for i := range history {
			ev := history[i]
			txt := fmt.Sprintf("%s  %s  %s", fmtTS(ev.TS), actorLabel(db, ev.ActorID), eventSummary(ev))
			st := styleMuted()
			if i == historyIdx && focus == itemFocusHistory {
				st = st.Copy().Background(colorSelectedBg)
			}
			bodyLines = append(bodyLines, fixedWidthLine(st.Render(txt), innerW))
		}
	}
	bodyLines = append(bodyLines, "")

	// Window the body to the available height so small terminals don't silently cut off content.
	moreStyle := styleMuted()
	lines := windowWithIndicators(headerLines, bodyLines, height, scroll, moreStyle)

	// Normalize to guarantee stable split-pane rendering even with unbroken long tokens.
	return normalizePane(box.Render(strings.Join(lines, "\n")), width, height)
}

func windowWithIndicators(header, body []string, height int, scroll int, moreStyle lipgloss.Style) []string {
	if height <= 0 {
		return []string{}
	}
	if scroll < 0 {
		scroll = 0
	}
	// If the header itself doesn't fit, fall back to a single scrollable window over
	// the combined content so we still show "more" indicators on small terminals.
	if len(header) > height {
		all := append([]string{}, header...)
		all = append(all, body...)
		return windowLinesWithIndicators(all, height, scroll, moreStyle)
	}
	avail := height - len(header)
	if avail <= 0 {
		// No room for body: fall back to a single scrollable window over the combined content
		// so we still expose body content and "more" indicators.
		if len(body) > 0 {
			all := append([]string{}, header...)
			all = append(all, body...)
			return windowLinesWithIndicators(all, height, scroll, moreStyle)
		}
		out := append([]string{}, header...)
		for len(out) < height {
			out = append(out, "")
		}
		return out
	}

	windowBody := windowLinesWithIndicators(body, avail, scroll, moreStyle)
	out := append([]string{}, header...)
	out = append(out, windowBody...)
	for len(out) < height {
		out = append(out, "")
	}
	if len(out) > height {
		out = out[:height]
	}
	return out
}

func windowLinesWithIndicators(lines []string, height int, scroll int, moreStyle lipgloss.Style) []string {
	if height <= 0 {
		return []string{}
	}
	if scroll < 0 {
		scroll = 0
	}
	if len(lines) == 0 {
		out := []string{""}
		for len(out) < height {
			out = append(out, "")
		}
		return out[:height]
	}

	// Converge on a stable window that accounts for indicator rows.
	for iter := 0; iter < 6; iter++ {
		needTop := scroll > 0
		avail := height
		if needTop {
			avail--
		}
		if avail < 0 {
			avail = 0
		}
		needBottom := scroll+avail < len(lines)
		if needBottom {
			avail--
		}
		if avail < 0 {
			avail = 0
		}
		// Clamp scroll to what can actually be shown.
		maxScroll := len(lines) - avail
		if maxScroll < 0 {
			maxScroll = 0
		}
		if scroll > maxScroll {
			scroll = maxScroll
			continue
		}
		// Re-evaluate needBottom after clamping and indicator reservation.
		needBottom2 := scroll+avail < len(lines)
		if needBottom2 && !needBottom {
			// Need to reserve space for bottom indicator too.
			continue
		}
		break
	}

	needTop := scroll > 0
	avail := height
	if needTop {
		avail--
	}
	if avail < 0 {
		avail = 0
	}
	needBottom := scroll+avail < len(lines)
	if needBottom {
		avail--
	}
	if avail < 0 {
		avail = 0
	}

	// Final clamp.
	maxScroll := len(lines) - avail
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}

	start := scroll
	end := start + avail
	if end > len(lines) {
		end = len(lines)
	}

	out := []string{}
	if needTop {
		out = append(out, moreStyle.Render(fmt.Sprintf("↑ %d more", start)))
	}
	out = append(out, lines[start:end]...)
	if needBottom {
		out = append(out, moreStyle.Render(fmt.Sprintf("↓ %d more", len(lines)-end)))
	}
	for len(out) < height {
		out = append(out, "")
	}
	if len(out) > height {
		out = out[:height]
	}
	return out
}

func truncateLines(s string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	return strings.Join(lines[:maxLines], "\n") + "\n…"
}

func renderChildrenOutline(db *store.DB, outline model.Outline, children []model.Item, width int, focused bool, selIdx int, off int, maxRows int) string {
	if len(children) == 0 {
		return "(no children)"
	}
	if maxRows <= 0 {
		maxRows = 1
	}
	// Some terminals/fonts treat a few glyphs as "ambiguous width" and may wrap lines
	// even when our width calculations say they fit (especially when the whole pane is
	// re-wrapped by lipgloss). Leave a small safety margin.
	if width > 0 {
		width -= 2
		if width < 0 {
			width = 0
		}
	}
	if selIdx < 0 {
		selIdx = 0
	}
	if selIdx >= len(children) {
		selIdx = len(children) - 1
	}
	if off < 0 {
		off = 0
	}
	if off > selIdx {
		off = selIdx
	}
	maxOff := len(children) - maxRows
	if maxOff < 0 {
		maxOff = 0
	}
	if off > maxOff {
		off = maxOff
	}
	end := off + maxRows
	if end > len(children) {
		end = len(children)
	}

	// Compute direct-child progress for each child (so progress cookies match outline behavior).
	childChildren := map[string][]model.Item{}
	for _, ch := range children {
		id := strings.TrimSpace(ch.ID)
		if id == "" {
			continue
		}
		childChildren[id] = db.ChildrenOf(id)
	}
	progress := computeChildProgress(outline, childChildren)

	d := newOutlineItemDelegate()
	lines := make([]string, 0, (end-off)+1)
	for i := off; i < end; i++ {
		ch := children[i]
		id := strings.TrimSpace(ch.ID)
		kids := childChildren[id]
		hasKids := len(kids) > 0
		doneChildren, totalChildren := 0, 0
		if p, ok := progress[id]; ok {
			doneChildren, totalChildren = p[0], p[1]
		}
		row := outlineRow{
			item:          ch,
			depth:         0,
			hasChildren:   hasKids,
			collapsed:     hasKids, // show ▸ to indicate subtree exists
			doneChildren:  doneChildren,
			totalChildren: totalChildren,
		}
		item := outlineRowItem{row: row, outline: outline}
		lines = append(lines, d.renderOutlineRow(width, "", item, focused && i == selIdx))
	}
	if end < len(children) {
		more := fmt.Sprintf("… and %d more", len(children)-end)
		lines = append(lines, styleMuted().Render(truncateText(more, width)))
	}
	return strings.Join(lines, "\n")
}

func renderParentOutline(db *store.DB, outline model.Outline, parent model.Item, width int, focused bool) string {
	parentID := strings.TrimSpace(parent.ID)
	if parentID == "" {
		return "(no parent)"
	}
	// Match the children renderer's width safety margin.
	if width > 0 {
		width -= 2
		if width < 0 {
			width = 0
		}
	}

	kids := db.ChildrenOf(parentID)
	hasKids := len(kids) > 0
	progress := computeChildProgress(outline, map[string][]model.Item{parentID: kids})
	doneChildren, totalChildren := 0, 0
	if p, ok := progress[parentID]; ok {
		doneChildren, totalChildren = p[0], p[1]
	}

	row := outlineRow{
		item:          parent,
		depth:         0,
		hasChildren:   hasKids,
		collapsed:     hasKids, // show ▸ to indicate subtree exists
		doneChildren:  doneChildren,
		totalChildren: totalChildren,
	}
	d := newOutlineItemDelegate()
	item := outlineRowItem{row: row, outline: outline}
	return d.renderOutlineRow(width, "", item, focused)
}

func fmtTS(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func eventIsForItem(ev model.Event, itemID string) bool {
	id := strings.TrimSpace(itemID)
	if id == "" {
		return false
	}
	if strings.TrimSpace(ev.EntityID) == id {
		return true
	}
	m, ok := ev.Payload.(map[string]any)
	if !ok {
		return false
	}
	switch strings.TrimSpace(ev.Type) {
	case "comment.add", "worklog.add":
		if v, ok := m["itemId"].(string); ok && strings.TrimSpace(v) == id {
			return true
		}
	case "dep.add":
		if v, ok := m["fromItemId"].(string); ok && strings.TrimSpace(v) == id {
			return true
		}
		if v, ok := m["toItemId"].(string); ok && strings.TrimSpace(v) == id {
			return true
		}
	}
	return false
}

func eventSummary(ev model.Event) string {
	typ := strings.TrimSpace(ev.Type)
	if typ == "" {
		typ = "(unknown)"
	}
	m, ok := ev.Payload.(map[string]any)
	if !ok {
		return typ
	}

	switch typ {
	case "item.set_title":
		if v, ok := m["title"].(string); ok && strings.TrimSpace(v) != "" {
			return "set title: " + truncateInline(v, 60)
		}
	case "item.set_status":
		// Prefer explicit transitions if available.
		from, _ := m["from"].(string)
		to, _ := m["to"].(string)
		note, _ := m["note"].(string)
		from = strings.TrimSpace(from)
		to = strings.TrimSpace(to)
		note = strings.TrimSpace(note)
		if from != "" || to != "" {
			if from == "" {
				from = "none"
			}
			if to == "" {
				to = "none"
			}
			if from == to {
				out := "set status: " + to
				if note != "" {
					out += " (note: " + truncateInline(note, 48) + ")"
				}
				return out
			}
			out := "set status: " + from + " -> " + to
			if note != "" {
				out += " (note: " + truncateInline(note, 48) + ")"
			}
			return out
		}
		if v, ok := m["status"].(string); ok {
			if strings.TrimSpace(v) == "" {
				out := "set status: none"
				if note != "" {
					out += " (note: " + truncateInline(note, 48) + ")"
				}
				return out
			}
			out := "set status: " + v
			if note != "" {
				out += " (note: " + truncateInline(note, 48) + ")"
			}
			return out
		}
	case "item.set_description":
		return "updated description"
	case "item.set_priority":
		if v, ok := m["priority"].(bool); ok {
			return fmt.Sprintf("set priority: %v", v)
		}
	case "item.set_on_hold":
		if v, ok := m["onHold"].(bool); ok {
			return fmt.Sprintf("set on hold: %v", v)
		}
	case "item.set_due":
		if v, ok := m["due"]; ok {
			if v == nil {
				return "due: cleared"
			}
			return "due: set"
		}
	case "item.set_schedule":
		if v, ok := m["schedule"]; ok {
			if v == nil {
				return "schedule: cleared"
			}
			return "schedule: set"
		}
	case "item.set_assign":
		if v, ok := m["assignedActorId"]; ok {
			if v == nil {
				return "unassigned"
			}
			if s, ok := v.(string); ok {
				return "assigned: " + s
			}
		}
	case "item.tags_add":
		if v, ok := m["tag"].(string); ok && strings.TrimSpace(v) != "" {
			return "tag added: #" + normalizeTag(v)
		}
	case "item.tags_remove":
		if v, ok := m["tag"].(string); ok && strings.TrimSpace(v) != "" {
			return "tag removed: #" + normalizeTag(v)
		}
	case "item.tags_set":
		if v, ok := m["tags"]; ok {
			if v == nil {
				return "tags: cleared"
			}
			return "tags: updated"
		}
	case "comment.add":
		if v, ok := m["body"].(string); ok && strings.TrimSpace(v) != "" {
			return "comment: " + truncateInline(v, 60)
		}
		return "comment added"
	case "worklog.add":
		if v, ok := m["body"].(string); ok && strings.TrimSpace(v) != "" {
			return "worklog: " + truncateInline(v, 60)
		}
		return "worklog added"
	case "dep.add":
		from, _ := m["fromItemId"].(string)
		to, _ := m["toItemId"].(string)
		return "dep: " + strings.TrimSpace(from) + " -> " + strings.TrimSpace(to)
	}

	return typ
}

func truncateInline(s string, max int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
