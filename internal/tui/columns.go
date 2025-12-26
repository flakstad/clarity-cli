package tui

import (
        "fmt"
        "sort"
        "strings"

        "clarity-cli/internal/model"

        "github.com/charmbracelet/lipgloss"
        xansi "github.com/charmbracelet/x/ansi"
)

type outlineColumnsSelection struct {
        Col  int
        Item int
        // ItemID is the stable selected item id (preferred over Item index for tracking focus
        // across re-sorts and status changes).
        ItemID string
}

type outlineColumnsCol struct {
        statusID string
        label    string
        items    []outlineColumnsItem
}

type outlineColumnsBoard struct {
        cols []outlineColumnsCol
}

type outlineColumnsItem struct {
        Item          model.Item
        DoneChildren  int
        TotalChildren int
        HasChildren   bool
}

func buildOutlineColumnsBoard(outline model.Outline, items []model.Item) outlineColumnsBoard {
        // Column order: (no status) then outline-defined statuses (in order).
        cols := make([]outlineColumnsCol, 0, len(outline.StatusDefs)+1)
        cols = append(cols, outlineColumnsCol{statusID: "", label: "(no status)"})
        for _, def := range outline.StatusDefs {
                lbl := strings.TrimSpace(def.Label)
                if lbl == "" {
                        lbl = strings.TrimSpace(def.ID)
                }
                if lbl == "" {
                        lbl = "(status)"
                }
                cols = append(cols, outlineColumnsCol{statusID: def.ID, label: lbl})
        }

        // Build parent -> children map so we can compute progress cookies (done/total
        // direct children).
        children := map[string][]model.Item{}
        present := map[string]bool{}
        for _, it := range items {
                present[it.ID] = true
        }
        for _, it := range items {
                if it.ParentID == nil || strings.TrimSpace(*it.ParentID) == "" {
                        continue
                }
                // If a parent is missing (e.g. archived), treat this as a root-like node for
                // progress purposes (i.e. don't attribute it to a non-present parent).
                if !present[*it.ParentID] {
                        continue
                }
                children[*it.ParentID] = append(children[*it.ParentID], it)
        }
        for pid := range children {
                sibs := children[pid]
                sort.Slice(sibs, func(i, j int) bool { return compareOutlineItems(sibs[i], sibs[j]) < 0 })
                children[pid] = sibs
        }
        progress := computeChildProgress(outline, children)

        // Columns mode shows only top-level outline items (no nesting).
        // Nested items remain accessible via the outline list view.
        topLevel := make([]model.Item, 0, len(items))
        for _, it := range items {
                if it.ParentID == nil || strings.TrimSpace(*it.ParentID) == "" {
                        topLevel = append(topLevel, it)
                }
        }

        // Assign items to columns.
        for _, it := range topLevel {
                doneChildren := 0
                totalChildren := 0
                if p, ok := progress[it.ID]; ok {
                        doneChildren, totalChildren = p[0], p[1]
                }
                wrapped := outlineColumnsItem{
                        Item:          it,
                        DoneChildren:  doneChildren,
                        TotalChildren: totalChildren,
                        HasChildren:   len(children[it.ID]) > 0,
                }

                sid := strings.TrimSpace(it.StatusID)
                if sid == "" {
                        cols[0].items = append(cols[0].items, wrapped)
                        continue
                }
                added := false
                for i := 1; i < len(cols); i++ {
                        if cols[i].statusID == sid {
                                cols[i].items = append(cols[i].items, wrapped)
                                added = true
                                break
                        }
                }
                if !added {
                        // Unknown status ID: put into the "(no status)" column for now.
                        cols[0].items = append(cols[0].items, wrapped)
                }
        }

        // Stable ordering inside columns.
        for i := range cols {
                sort.Slice(cols[i].items, func(a, b int) bool {
                        return compareOutlineItems(cols[i].items[a].Item, cols[i].items[b].Item) < 0
                })
        }

        return outlineColumnsBoard{cols: cols}
}

func (b outlineColumnsBoard) indexOfItemID(itemID string) (int, int, bool) {
        itemID = strings.TrimSpace(itemID)
        if itemID == "" {
                return 0, 0, false
        }
        for ci := range b.cols {
                for ii := range b.cols[ci].items {
                        if strings.TrimSpace(b.cols[ci].items[ii].Item.ID) == itemID {
                                return ci, ii, true
                        }
                }
        }
        return 0, 0, false
}

func (b outlineColumnsBoard) clamp(sel outlineColumnsSelection) outlineColumnsSelection {
        if len(b.cols) == 0 {
                return outlineColumnsSelection{Col: 0, Item: -1}
        }

        // Prefer stable selection by ID when present.
        if ci, ii, ok := b.indexOfItemID(sel.ItemID); ok {
                sel.Col = ci
                sel.Item = ii
        } else {
                sel.ItemID = ""
        }

        if sel.Col < 0 {
                sel.Col = 0
        }
        if sel.Col >= len(b.cols) {
                sel.Col = len(b.cols) - 1
        }

        nItems := len(b.cols[sel.Col].items)
        if nItems == 0 {
                sel.Item = -1
                return sel
        }
        if sel.Item < 0 {
                sel.Item = 0
        }
        if sel.Item >= nItems {
                sel.Item = nItems - 1
        }
        if sel.Item >= 0 && sel.Item < nItems {
                sel.ItemID = strings.TrimSpace(b.cols[sel.Col].items[sel.Item].Item.ID)
        }
        return sel
}

func (b outlineColumnsBoard) selectedItem(sel outlineColumnsSelection) (outlineColumnsItem, bool) {
        sel = b.clamp(sel)
        if len(b.cols) == 0 {
                return outlineColumnsItem{}, false
        }
        if sel.Col < 0 || sel.Col >= len(b.cols) {
                return outlineColumnsItem{}, false
        }
        if sel.Item < 0 || sel.Item >= len(b.cols[sel.Col].items) {
                return outlineColumnsItem{}, false
        }
        return b.cols[sel.Col].items[sel.Item], true
}

func renderOutlineColumns(outline model.Outline, board outlineColumnsBoard, sel outlineColumnsSelection, width, height int) string {
        if width < 0 {
                width = 0
        }
        if height < 0 {
                height = 0
        }

        // If we have more columns than we can reasonably show, still render all; widths will shrink.
        n := len(board.cols)
        if n <= 0 {
                return normalizePane("", width, height)
        }
        sel = board.clamp(sel)

        gap := 2
        avail := width - gap*(n-1)
        if avail < n {
                avail = n
        }
        colW := avail / n
        if colW < 10 {
                colW = 10
        }

        headerStyle := lipgloss.NewStyle().Bold(true).Foreground(colorSurfaceFg).Background(colorControlBg)
        headerSelectedStyle := lipgloss.NewStyle().Bold(true).Foreground(colorSelectedFg).Background(colorSelectedBg)
        muted := styleMuted()

        // In columns mode, keep item rendering minimal: whitespace defines the "card",
        // not borders (which can read like a continuous list when stacked).
        itemStyle := lipgloss.NewStyle().Width(colW).Padding(0, 1)
        itemSelectedStyle := itemStyle.Copy().Foreground(colorSelectedFg).Background(colorSelectedBg).Bold(true)
        itemInnerW := colW - 2 // left+right padding
        if itemInnerW < 0 {
                itemInnerW = 0
        }

        wrapPlainTextWithPrefix := func(s string, maxW int, firstPrefix, contPrefix string) []string {
                if maxW <= 0 {
                        return []string{""}
                }
                s = strings.TrimSpace(s)
                if s == "" {
                        return []string{firstPrefix}
                }
                firstPW := xansi.StringWidth(firstPrefix)
                contPW := xansi.StringWidth(contPrefix)
                firstAvail := maxW - firstPW
                contAvail := maxW - contPW
                if firstAvail < 1 {
                        firstAvail = 1
                }
                if contAvail < 1 {
                        contAvail = 1
                }

                words := strings.Fields(s)
                lines := make([]string, 0, 4)
                linePrefix := firstPrefix
                avail := firstAvail

                cur := ""
                curW := 0
                flush := func() {
                        lines = append(lines, linePrefix+cur)
                        linePrefix = contPrefix
                        avail = contAvail
                        cur = ""
                        curW = 0
                }

                for _, w := range words {
                        wordW := xansi.StringWidth(w)
                        if cur == "" {
                                if wordW <= avail {
                                        cur = w
                                        curW = wordW
                                        continue
                                }
                                rest := w
                                for xansi.StringWidth(rest) > avail {
                                        lines = append(lines, linePrefix+xansi.Cut(rest, 0, avail))
                                        rest = xansi.Cut(rest, avail, xansi.StringWidth(rest))
                                        linePrefix = contPrefix
                                        avail = contAvail
                                }
                                cur = rest
                                curW = xansi.StringWidth(rest)
                                continue
                        }

                        if curW+1+wordW <= avail {
                                cur = cur + " " + w
                                curW = curW + 1 + wordW
                                continue
                        }
                        flush()

                        if wordW <= avail {
                                cur = w
                                curW = wordW
                                continue
                        }
                        rest := w
                        for xansi.StringWidth(rest) > avail {
                                lines = append(lines, linePrefix+xansi.Cut(rest, 0, avail))
                                rest = xansi.Cut(rest, avail, xansi.StringWidth(rest))
                                linePrefix = contPrefix
                                avail = contAvail
                        }
                        cur = rest
                        curW = xansi.StringWidth(rest)
                }

                if cur != "" || len(lines) == 0 {
                        lines = append(lines, linePrefix+cur)
                }
                return lines
        }

        type token struct {
                s string
                w int
        }

        wrapTokens := func(tokens []token, maxW int) []string {
                if maxW <= 0 {
                        return []string{""}
                }
                if len(tokens) == 0 {
                        return nil
                }
                lines := make([]string, 0, 2)
                cur := make([]string, 0, 4)
                used := 0
                flush := func() {
                        lines = append(lines, strings.Join(cur, " "))
                        cur = nil
                        used = 0
                }
                for _, tok := range tokens {
                        next := tok.w
                        if used > 0 {
                                next++ // space separator
                        }
                        if used+next <= maxW {
                                cur = append(cur, tok.s)
                                used += next
                                continue
                        }
                        if len(cur) > 0 {
                                flush()
                        }
                        // If a single token is wider than maxW, hard-cut it (no ellipsis).
                        if tok.w > maxW {
                                lines = append(lines, xansi.Cut(tok.s, 0, maxW))
                                continue
                        }
                        cur = append(cur, tok.s)
                        used = tok.w
                }
                if len(cur) > 0 {
                        flush()
                }
                return lines
        }

        renderMetaLines := func(it outlineColumnsItem, selected bool, maxW int, indent string) []string {
                indentW := xansi.StringWidth(indent)
                availW := maxW - indentW
                if availW < 1 {
                        availW = 1
                }

                tokens := make([]token, 0, 5)

                if it.Item.Priority {
                        st := metaPriorityStyle
                        if selected {
                                st = st.Copy().Background(colorSelectedBg)
                        }
                        seg := st.Render("priority")
                        tokens = append(tokens, token{s: seg, w: xansi.StringWidth(seg)})
                }
                if it.Item.OnHold {
                        st := metaOnHoldStyle
                        if selected {
                                st = st.Copy().Background(colorSelectedBg)
                        }
                        seg := st.Render("on hold")
                        tokens = append(tokens, token{s: seg, w: xansi.StringWidth(seg)})
                }
                if s := strings.TrimSpace(formatScheduleLabel(it.Item.Schedule)); s != "" {
                        st := metaScheduleStyle
                        if selected {
                                st = st.Copy().Background(colorSelectedBg)
                        }
                        seg := st.Render(s)
                        tokens = append(tokens, token{s: seg, w: xansi.StringWidth(seg)})
                }
                if s := strings.TrimSpace(formatDueLabel(it.Item.Due)); s != "" {
                        st := metaDueStyle
                        if selected {
                                st = st.Copy().Background(colorSelectedBg)
                        }
                        seg := st.Render(s)
                        tokens = append(tokens, token{s: seg, w: xansi.StringWidth(seg)})
                }

                lines := wrapTokens(tokens, availW)
                for i := range lines {
                        lines[i] = indent + lines[i]
                }
                return lines
        }

        renderItemCard := func(colStatusID string, it outlineColumnsItem, selected bool) string {
                title := strings.TrimSpace(it.Item.Title)
                if title == "" {
                        title = "(untitled)"
                }
                prefix := "  "
                if it.HasChildren {
                        prefix = "▸ "
                }
                titleLines := wrapPlainTextWithPrefix(title, itemInnerW, prefix, "  ")

                titleStyle := lipgloss.NewStyle().Bold(true)
                if selected {
                        titleStyle = titleStyle.Copy().Foreground(colorSelectedFg).Background(colorSelectedBg)
                } else if isEndState(outline, colStatusID) {
                        titleStyle = faintIfDark(lipgloss.NewStyle()).
                                Foreground(colorMuted).
                                Strikethrough(true)
                }

                // Indent meta to align with the wrapped title lines (i.e. no chevron).
                metaLines := renderMetaLines(it, selected, itemInnerW, "  ")

                content := make([]string, 0, len(titleLines)+len(metaLines))
                for _, ln := range titleLines {
                        content = append(content, titleStyle.Render(ln))
                }
                content = append(content, metaLines...)

                inner := normalizePane(strings.Join(content, "\n"), itemInnerW, 0)
                if selected {
                        return itemSelectedStyle.Render(inner)
                }
                return itemStyle.Render(inner)
        }

        renderCol := func(colIdx int, c outlineColumnsCol) string {
                head := fmt.Sprintf("%s (%d)", c.label, len(c.items))
                head = truncateText(head, colW)
                lines := make([]string, 0, max(2, height))
                hs := headerStyle
                if colIdx == sel.Col {
                        hs = headerSelectedStyle
                }
                lines = append(lines, hs.Width(colW).Render(head))

                if len(c.items) == 0 {
                        lines = append(lines, muted.Render("(empty)"))
                        return normalizePane(strings.Join(lines, "\n"), colW, height)
                }

                // Padding above the first item.
                lines = append(lines, "")

                for i, it := range c.items {
                        card := renderItemCard(c.statusID, it, colIdx == sel.Col && i == sel.Item)
                        lines = append(lines, strings.Split(card, "\n")...)

                        // Separator between items.
                        if i < len(c.items)-1 {
                                sepW := colW - 2 // align with item padding
                                if sepW < 0 {
                                        sepW = 0
                                }
                                sep := " " + strings.Repeat("─", sepW) + " "
                                lines = append(lines, styleMuted().Render(sep))
                        }
                }
                return normalizePane(strings.Join(lines, "\n"), colW, height)
        }

        rendered := make([]string, 0, n)
        for i, c := range board.cols {
                rendered = append(rendered, renderCol(i, c))
        }

        out := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
        // Insert gaps manually because JoinHorizontal doesn't provide inter-column spacing.
        if gap > 0 && len(rendered) > 1 {
                out = rendered[0]
                sep := strings.Repeat(" ", gap)
                for i := 1; i < len(rendered); i++ {
                        out = lipgloss.JoinHorizontal(lipgloss.Top, out, sep, rendered[i])
                }
        }

        return normalizePane(out, width, height)
}
