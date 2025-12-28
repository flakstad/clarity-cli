package tui

import (
        "encoding/json"
        "fmt"
        "strings"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/lipgloss"
)

type itemSidePanelKind int

const (
        itemSideNone itemSidePanelKind = iota
        itemSideComments
        itemSideWorklog
        itemSideHistory
)

func sidePanelKindForFocus(f itemPageFocus) itemSidePanelKind {
        switch f {
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

func renderItemSidePanelWithEvents(db *store.DB, it model.Item, width, height int, kind itemSidePanelKind, commentIdx, worklogIdx, historyIdx int, scroll int, events []model.Event) string {
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
        focusRowStyle := lipgloss.NewStyle().
                Foreground(colorSelectedFg).
                Background(colorSelectedBg).
                Bold(true)
        moreStyle := styleMuted()

        lines := []string{}
        switch kind {
        case itemSideComments:
                comments := db.CommentsForItem(it.ID)
                lines = append(lines, headerStyle.Render(fmt.Sprintf("Comments (%d)", len(comments))))
                lines = append(lines, headerStyle.Render("up/down, j/k, ctrl+n/p: select  home/end: jump  pgup/pgdown: scroll  R: reply"))
                lines = append(lines, "")
                lines = append(lines, renderThreadedComments(db, comments, commentIdx, innerW, height-3, scroll, focusRowStyle, moreStyle)...)
        case itemSideWorklog:
                worklog := db.WorklogForItem(it.ID)
                lines = append(lines, headerStyle.Render(fmt.Sprintf("Worklog (%d)", len(worklog))))
                lines = append(lines, headerStyle.Render("up/down, j/k, ctrl+n/p: select  home/end: jump  pgup/pgdown: scroll"))
                lines = append(lines, "")
                lines = append(lines, renderAccordionWorklog(db, worklog, worklogIdx, innerW, height-3, scroll, focusRowStyle, moreStyle)...)
        case itemSideHistory:
                evs := filterEventsForItem(db, events, it.ID)
                lines = append(lines, headerStyle.Render(fmt.Sprintf("History (%d)", len(evs))))
                lines = append(lines, headerStyle.Render("up/down, j/k, ctrl+n/p: select  home/end: jump  pgup/pgdown: scroll"))
                lines = append(lines, "")
                lines = append(lines, renderAccordionHistory(db, events, it.ID, historyIdx, innerW, height-3, scroll, focusRowStyle, moreStyle)...)
        }

        return normalizePane(box.Render(strings.Join(lines, "\n")), width, height)
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

        // Collapsed line (always rendered, even when selected).
        lineFor := func(r commentThreadRow) string {
                indent := strings.Repeat("  ", r.Depth)
                prefix := ""
                if r.Depth > 0 {
                        prefix = "↳ "
                }
                actor := actorLabel(db, r.Comment.AuthorID)
                snippetMax := maxInt(20, width-26-(2*r.Depth))
                snippet := truncateInline(r.Comment.Body, snippetMax)
                return fmt.Sprintf("%s%s%s  %s  %s", indent, prefix, fmtTS(r.Comment.CreatedAt), actor, snippet)
        }

        // Expanded markdown for selected comment: include inline quote if it's a reply.
        c := rows[selected].Comment
        md := strings.TrimSpace(c.Body)
        if c.ReplyToCommentID != nil && strings.TrimSpace(*c.ReplyToCommentID) != "" {
                parentID := strings.TrimSpace(*c.ReplyToCommentID)
                if parent, ok := findCommentByID(comments, parentID); ok {
                        q := truncateInline(parent.Body, 280)
                        md = fmt.Sprintf("> %s  %s\n> %s\n\n%s",
                                fmtTS(parent.CreatedAt),
                                actorLabel(db, parent.AuthorID),
                                q,
                                strings.TrimSpace(c.Body),
                        )
                }
        }

        mdLines := strings.Split(renderMarkdown(md, maxInt(10, width-2)), "\n")
        if scroll > len(mdLines) {
                scroll = len(mdLines)
        }

        // Fit expanded markdown into the available height. We dynamically account for the optional
        // "more" indicator rows instead of permanently reserving space, so the expanded view uses
        // the full pane height when possible.
        mdH := height - 1 // 1 collapsed line + mdH lines
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
                if end > len(rows) {
                        end = len(rows)
                }

                outLen := (end - start - 1) + selectedBlockLen
                if start > 0 {
                        outLen++
                }
                if end < len(rows) {
                        outLen++
                }

                overflow := outLen - height
                if overflow <= 0 {
                        break
                }
                // Reduce markdown window to make room for indicators/neighbor rows.
                mdH -= overflow
                if mdH < 1 {
                        mdH = 1
                        break
                }
        }

        selectedLine := focusRowStyle.Render(lineFor(commentThreadRow{Comment: c, Depth: rows[selected].Depth}))
        selectedBlock := []string{selectedLine}
        for _, ln := range mdWin {
                selectedBlock = append(selectedBlock, "  "+ln)
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
                out = append(out, lineFor(rows[i]))
        }
        if end < len(rows) {
                out = append(out, moreStyle.Render(fmt.Sprintf("↓ %d more", len(rows)-end)))
        }
        return out
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

        mdLines := strings.Split(renderMarkdown(strings.TrimSpace(worklog[selected].Body), maxInt(10, width-2)), "\n")
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
                selectedBlock = append(selectedBlock, "  "+ln)
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
        mdLines := strings.Split(renderMarkdown(md, maxInt(10, width-2)), "\n")
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
                selectedBlock = append(selectedBlock, "  "+ln)
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
