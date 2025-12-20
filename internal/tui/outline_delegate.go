package tui

import (
        "fmt"
        "io"
        "strings"

        "github.com/charmbracelet/bubbles/list"
        tea "github.com/charmbracelet/bubbletea"
        "github.com/charmbracelet/lipgloss"
        xansi "github.com/charmbracelet/x/ansi"
)

type outlineItemDelegate struct {
        normal   lipgloss.Style
        selected lipgloss.Style
        addRow   lipgloss.Style
}

func newOutlineItemDelegate() outlineItemDelegate {
        return outlineItemDelegate{
                normal: lipgloss.NewStyle(),
                selected: lipgloss.NewStyle().
                        Foreground(lipgloss.Color("255")).
                        Background(lipgloss.Color("236")).
                        Bold(true),
                addRow: lipgloss.NewStyle().
                        Foreground(lipgloss.Color("245")).
                        Italic(true),
        }
}

func (d outlineItemDelegate) Height() int  { return 1 }
func (d outlineItemDelegate) Spacing() int { return 0 }
func (d outlineItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
        return nil
}

func (d outlineItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
        contentW := m.Width()
        if contentW < 4 {
                fmt.Fprint(w, "")
                return
        }

        focused := index == m.Index()

        // Keep the left edge stable (no selector "bar"); use a full-row background
        // highlight for the focused row instead.
        prefix := ""
        switch it := item.(type) {
        case outlineRowItem:
                fmt.Fprint(w, d.renderOutlineRow(contentW, prefix, it, focused))
                return
        case addItemRow:
                // Match the outline's twisty column (2 chars) so "+ Add item" aligns.
                line := prefix + "  " + it.Title()
                if focused {
                        fmt.Fprint(w, d.renderFocusedRow(contentW, d.addRow, line))
                        return
                }
                fmt.Fprint(w, d.renderRow(contentW, d.addRow, line))
                return
        }

        txt := ""
        if t, ok := item.(interface{ Title() string }); ok {
                txt = t.Title()
        } else {
                txt = fmt.Sprint(item)
        }

        line := prefix + txt

        base := d.normal
        if _, ok := item.(addItemRow); ok {
                base = d.addRow
                // Match the outline's twisty column (2 chars) so "+ Add item" aligns.
                line = prefix + "  " + txt
        }
        if focused {
                fmt.Fprint(w, d.renderFocusedRow(contentW, base, line))
                return
        }
        fmt.Fprint(w, d.renderRow(contentW, base, line))
}

func (d outlineItemDelegate) renderOutlineRow(width int, prefix string, it outlineRowItem, focused bool) string {
        bg := d.selected.GetBackground()

        base := d.normal
        if focused {
                base = lipgloss.NewStyle().
                        Foreground(d.selected.GetForeground()).
                        Background(bg).
                        Bold(true)
        }

        indent := strings.Repeat("  ", it.row.depth)
        twisty := " "
        if it.row.hasChildren {
                if it.row.collapsed {
                        twisty = "▸"
                } else {
                        twisty = "▾"
                }
        }

        leadRaw := prefix + indent + twisty + " "
        leadSeg := base.Render(leadRaw)

        statusID := strings.TrimSpace(it.row.item.StatusID)
        statusTxt := strings.ToUpper(strings.TrimSpace(statusLabel(it.outline, statusID)))
        statusRaw := ""
        statusSeg := ""
        if statusTxt != "" {
                style := statusOtherStyle
                for _, def := range it.outline.StatusDefs {
                        if def.ID == statusID && def.IsEndState {
                                style = statusDoneStyle
                                break
                        }
                }
                switch strings.ToLower(statusID) {
                case "todo":
                        style = statusTodoStyle
                case "doing":
                        style = statusDoingStyle
                case "done":
                        style = statusDoneStyle
                }
                if focused {
                        style = style.Copy().Background(bg)
                }
                statusSeg = style.Render(statusTxt) + base.Render(" ")
                statusRaw = statusTxt + " "
        }

        metaParts := make([]string, 0, 3)

        if it.row.item.Priority {
                st := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
                if focused {
                        st = st.Background(bg)
                }
                metaParts = append(metaParts, st.Render("priority"))
        }
        if it.row.item.OnHold {
                st := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
                if focused {
                        st = st.Background(bg)
                }
                metaParts = append(metaParts, st.Render("on hold"))
        }
        if it.row.totalChildren > 0 {
                // Keep the "progress cookie" visual from earlier versions (like outline.js).
                metaParts = append(metaParts, renderProgressCookie(it.row.doneChildren, it.row.totalChildren))
        }

        rightSeg := strings.Join(metaParts, base.Render(" "))
        rightW := xansi.StringWidth(rightSeg)

        title := strings.TrimSpace(it.row.item.Title)
        if title == "" {
                title = "(untitled)"
        }

        // Reserve room for metadata on the right; the title has a hard cap.
        maxTitleW := 50
        availTitle := width - xansi.StringWidth(leadRaw) - xansi.StringWidth(statusRaw)
        if rightSeg != "" {
                availTitle -= (1 + rightW) // space + right side
        }
        if availTitle < 0 {
                availTitle = 0
        }
        if availTitle < maxTitleW {
                maxTitleW = availTitle
        }

        titleTrunc := truncateText(title, maxTitleW)
        titleSeg := base.Render(titleTrunc)

        spacerSeg := ""
        if rightSeg != "" {
                leftRaw := leadRaw + statusRaw + titleTrunc
                spacerW := width - xansi.StringWidth(leftRaw) - 1 - rightW
                if spacerW < 0 {
                        maxTitleW = max(0, maxTitleW+spacerW)
                        titleTrunc = truncateText(title, maxTitleW)
                        titleSeg = base.Render(titleTrunc)
                        leftRaw = leadRaw + statusRaw + titleTrunc
                        spacerW = width - xansi.StringWidth(leftRaw) - 1 - rightW
                        if spacerW < 0 {
                                spacerW = 0
                        }
                }
                spacerSeg = base.Render(" " + strings.Repeat(" ", spacerW))
        }

        out := leadSeg + statusSeg + titleSeg + spacerSeg + rightSeg
        curW := xansi.StringWidth(out)
        if curW < width {
                out += base.Render(strings.Repeat(" ", width-curW))
        } else if curW > width {
                out = xansi.Cut(out, 0, width)
        }
        return out
}

func (d outlineItemDelegate) renderFocusedRow(width int, base lipgloss.Style, line string) string {
        style := base.Copy().
                Foreground(d.selected.GetForeground()).
                Background(d.selected.GetBackground()).
                Bold(true)
        return d.renderRow(width, style, line)
}

func (d outlineItemDelegate) renderRow(width int, style lipgloss.Style, line string) string {
        plainW := xansi.StringWidth(line)
        if plainW < width {
                line += strings.Repeat(" ", width-plainW)
        } else if plainW > width {
                line = xansi.Cut(line, 0, width)
        }
        return style.Render(line)
}

func truncateText(s string, maxW int) string {
        if maxW <= 0 {
                return ""
        }
        if xansi.StringWidth(s) <= maxW {
                return s
        }
        if maxW == 1 {
                return "…"
        }
        return xansi.Cut(s, 0, maxW-1) + "…"
}

func max(a, b int) int {
        if a > b {
                return a
        }
        return b
}
