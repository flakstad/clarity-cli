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
        selectedStyle := lipgloss.NewStyle().Foreground(colorSelectedFg).Background(colorAccent).Bold(true)
        moreStyle := styleMuted()

        lines := []string{}
        switch kind {
        case itemSideComments:
                comments := db.CommentsForItem(it.ID)
                lines = append(lines, headerStyle.Render(fmt.Sprintf("Comments (%d)", len(comments))))
                lines = append(lines, headerStyle.Render("up/down: select  home/end: jump  pgup/pgdown: scroll"))
                lines = append(lines, "")
                lines = append(lines, renderAccordionComments(comments, commentIdx, innerW, height-3, scroll, selectedStyle, moreStyle)...)
        case itemSideWorklog:
                worklog := db.WorklogForItem(it.ID)
                lines = append(lines, headerStyle.Render(fmt.Sprintf("Worklog (%d)", len(worklog))))
                lines = append(lines, headerStyle.Render("up/down: select  home/end: jump  pgup/pgdown: scroll"))
                lines = append(lines, "")
                lines = append(lines, renderAccordionWorklog(worklog, worklogIdx, innerW, height-3, scroll, selectedStyle, moreStyle)...)
        case itemSideHistory:
                evs := filterEventsForItem(db, events, it.ID)
                lines = append(lines, headerStyle.Render(fmt.Sprintf("History (%d)", len(evs))))
                lines = append(lines, headerStyle.Render("up/down: select  home/end: jump  pgup/pgdown: scroll"))
                lines = append(lines, "")
                lines = append(lines, renderAccordionHistory(db, events, it.ID, historyIdx, innerW, height-3, scroll, selectedStyle, moreStyle)...)
        }

        return normalizePane(box.Render(strings.Join(lines, "\n")), width, height)
}

func renderAccordionComments(comments []model.Comment, selected int, width, height, scroll int, selectedStyle lipgloss.Style, moreStyle lipgloss.Style) []string {
        if height < 1 {
                return []string{}
        }
        if len(comments) == 0 {
                return []string{"(no comments)"}
        }

        // Comments are rendered oldest-first for reading flow.
        ordered := make([]model.Comment, 0, len(comments))
        for i := len(comments) - 1; i >= 0; i-- {
                ordered = append(ordered, comments[i])
        }

        if selected < 0 {
                selected = 0
        }
        if selected >= len(ordered) {
                selected = len(ordered) - 1
        }
        // Clamp scroll.
        if scroll < 0 {
                scroll = 0
        }

        // Reserve space: selected block uses some rows; rest are collapsed lines.
        maxBlock := height
        // Leave at least 2 collapsed lines if possible.
        if maxBlock > 6 {
                maxBlock = height - 2
        }
        if maxBlock < 3 {
                maxBlock = 3
        }

        title := fmt.Sprintf("%s  %s", fmtTS(ordered[selected].CreatedAt), strings.TrimSpace(ordered[selected].AuthorID))
        mdLines := strings.Split(renderMarkdown(strings.TrimSpace(ordered[selected].Body), maxInt(10, width-2)), "\n")
        if scroll > len(mdLines) {
                scroll = len(mdLines)
        }
        mdH := maxBlock - 2 // title + at least 1 md line
        if mdH < 1 {
                mdH = 1
        }
        if scroll > 0 && scroll > len(mdLines)-mdH {
                scroll = maxInt(0, len(mdLines)-mdH)
        }
        mdWin := mdLines
        if len(mdWin) > mdH {
                end := scroll + mdH
                if end > len(mdLines) {
                        end = len(mdLines)
                }
                mdWin = mdLines[scroll:end]
        }

        selectedBlock := []string{selectedStyle.Render("▸ " + title)}
        for _, ln := range mdWin {
                selectedBlock = append(selectedBlock, "  "+ln)
        }

        remain := height - len(selectedBlock)
        if remain < 0 {
                remain = 0
        }
        beforeN := remain / 2
        afterN := remain - beforeN

        start := selected - beforeN
        if start < 0 {
                start = 0
        }
        end := selected + 1 + afterN
        if end > len(ordered) {
                end = len(ordered)
        }
        // Rebalance if we hit edges.
        for end-start < 1 && start > 0 {
                start--
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
                snippet := truncateInline(ordered[i].Body, maxInt(20, width-26))
                out = append(out, fmt.Sprintf("- %s  %s  %s", fmtTS(ordered[i].CreatedAt), strings.TrimSpace(ordered[i].AuthorID), snippet))
        }
        if end < len(ordered) {
                out = append(out, moreStyle.Render(fmt.Sprintf("↓ %d more", len(ordered)-end)))
        }
        return out
}

func renderAccordionWorklog(worklog []model.WorklogEntry, selected int, width, height, scroll int, selectedStyle lipgloss.Style, moreStyle lipgloss.Style) []string {
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

        maxBlock := height
        if maxBlock > 6 {
                maxBlock = height - 2
        }
        if maxBlock < 3 {
                maxBlock = 3
        }

        title := fmt.Sprintf("%s  %s", fmtTS(worklog[selected].CreatedAt), strings.TrimSpace(worklog[selected].AuthorID))
        mdLines := strings.Split(renderMarkdown(strings.TrimSpace(worklog[selected].Body), maxInt(10, width-2)), "\n")
        if scroll > len(mdLines) {
                scroll = len(mdLines)
        }
        mdH := maxBlock - 2
        if mdH < 1 {
                mdH = 1
        }
        if scroll > 0 && scroll > len(mdLines)-mdH {
                scroll = maxInt(0, len(mdLines)-mdH)
        }
        mdWin := mdLines
        if len(mdWin) > mdH {
                end := scroll + mdH
                if end > len(mdLines) {
                        end = len(mdLines)
                }
                mdWin = mdLines[scroll:end]
        }

        selectedBlock := []string{selectedStyle.Render("▸ " + title)}
        for _, ln := range mdWin {
                selectedBlock = append(selectedBlock, "  "+ln)
        }

        remain := height - len(selectedBlock)
        if remain < 0 {
                remain = 0
        }
        beforeN := remain / 2
        afterN := remain - beforeN

        start := selected - beforeN
        if start < 0 {
                start = 0
        }
        end := selected + 1 + afterN
        if end > len(worklog) {
                end = len(worklog)
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
                snippet := truncateInline(worklog[i].Body, maxInt(20, width-26))
                out = append(out, fmt.Sprintf("- %s  %s  %s", fmtTS(worklog[i].CreatedAt), strings.TrimSpace(worklog[i].AuthorID), snippet))
        }
        if end < len(worklog) {
                out = append(out, moreStyle.Render(fmt.Sprintf("↓ %d more", len(worklog)-end)))
        }
        return out
}

func renderAccordionHistory(db *store.DB, events []model.Event, itemID string, selected int, width, height, scroll int, selectedStyle lipgloss.Style, moreStyle lipgloss.Style) []string {
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

        maxBlock := height
        if maxBlock > 6 {
                maxBlock = height - 2
        }
        if maxBlock < 3 {
                maxBlock = 3
        }

        ev := evs[selected]
        actor := strings.TrimSpace(ev.ActorID)
        if db != nil {
                if a, ok := db.FindActor(actor); ok && strings.TrimSpace(a.Name) != "" {
                        actor = a.Name
                }
        }

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
        mdH := maxBlock - 2
        if mdH < 1 {
                mdH = 1
        }
        if scroll > 0 && scroll > len(mdLines)-mdH {
                scroll = maxInt(0, len(mdLines)-mdH)
        }
        mdWin := mdLines
        if len(mdWin) > mdH {
                end := scroll + mdH
                if end > len(mdLines) {
                        end = len(mdLines)
                }
                mdWin = mdLines[scroll:end]
        }

        selectedBlock := []string{selectedStyle.Render("▸ " + truncateInline(title, maxInt(20, width-2)))}
        for _, ln := range mdWin {
                selectedBlock = append(selectedBlock, "  "+ln)
        }

        remain := height - len(selectedBlock)
        if remain < 0 {
                remain = 0
        }
        beforeN := remain / 2
        afterN := remain - beforeN

        start := selected - beforeN
        if start < 0 {
                start = 0
        }
        end := selected + 1 + afterN
        if end > len(evs) {
                end = len(evs)
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
                actor := strings.TrimSpace(ev.ActorID)
                if db != nil {
                        if a, ok := db.FindActor(actor); ok && strings.TrimSpace(a.Name) != "" {
                                actor = a.Name
                        }
                }
                out = append(out, fmt.Sprintf("- %s  %s  %s", fmtTS(ev.TS), actor, truncateInline(eventSummary(ev), maxInt(20, width-26))))
        }
        if end < len(evs) {
                out = append(out, moreStyle.Render(fmt.Sprintf("↓ %d more", len(evs)-end)))
        }
        return out
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
