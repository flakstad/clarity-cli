package tui

import (
        "fmt"
        "sort"
        "strings"

        "clarity-cli/internal/model"

        "github.com/charmbracelet/lipgloss"
)

func renderOutlineColumns(outline model.Outline, items []model.Item, width, height int) string {
        if width < 0 {
                width = 0
        }
        if height < 0 {
                height = 0
        }

        // Column order: (no status) then outline-defined statuses (in order).
        type col struct {
                statusID string
                label    string
                items    []model.Item
        }

        cols := make([]col, 0, len(outline.StatusDefs)+1)
        cols = append(cols, col{statusID: "", label: "(no status)"})
        for _, def := range outline.StatusDefs {
                lbl := strings.TrimSpace(def.Label)
                if lbl == "" {
                        lbl = strings.TrimSpace(def.ID)
                }
                if lbl == "" {
                        lbl = "(status)"
                }
                cols = append(cols, col{statusID: def.ID, label: lbl})
        }

        // Assign items to columns.
        for _, it := range items {
                sid := strings.TrimSpace(it.StatusID)
                if sid == "" {
                        cols[0].items = append(cols[0].items, it)
                        continue
                }
                added := false
                for i := 1; i < len(cols); i++ {
                        if cols[i].statusID == sid {
                                cols[i].items = append(cols[i].items, it)
                                added = true
                                break
                        }
                }
                if !added {
                        // Unknown status ID: put into the "(no status)" column for now.
                        cols[0].items = append(cols[0].items, it)
                }
        }

        // Stable ordering inside columns.
        for i := range cols {
                sort.Slice(cols[i].items, func(a, b int) bool {
                        return compareOutlineItems(cols[i].items[a], cols[i].items[b]) < 0
                })
        }

        // If we have more columns than we can reasonably show, still render all; widths will shrink.
        n := len(cols)
        if n <= 0 {
                return normalizePane("", width, height)
        }

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
        cardStyle := lipgloss.NewStyle()
        muted := styleMuted()

        renderCol := func(c col) string {
                head := fmt.Sprintf("%s (%d)", c.label, len(c.items))
                head = truncateText(head, colW)
                lines := make([]string, 0, max(2, height))
                lines = append(lines, headerStyle.Width(colW).Render(head))

                if len(c.items) == 0 {
                        lines = append(lines, muted.Render("(empty)"))
                        return normalizePane(strings.Join(lines, "\n"), colW, height)
                }

                for _, it := range c.items {
                        title := strings.TrimSpace(it.Title)
                        if title == "" {
                                title = "(untitled)"
                        }
                        prefix := "- "
                        if it.Priority {
                                prefix = "! "
                        } else if it.OnHold {
                                prefix = "~ "
                        }
                        line := prefix + title
                        line = truncateText(line, colW)
                        lines = append(lines, cardStyle.Render(line))
                }
                return normalizePane(strings.Join(lines, "\n"), colW, height)
        }

        rendered := make([]string, 0, n)
        for _, c := range cols {
                rendered = append(rendered, renderCol(c))
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
