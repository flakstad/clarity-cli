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
        if focused {
                switch it := item.(type) {
                case outlineRowItem:
                        fmt.Fprint(w, d.renderFocusedOutlineRow(contentW, prefix, it))
                        return
                case addItemRow:
                        // Match the outline's twisty column (2 chars) so "+ Add item" aligns.
                        fmt.Fprint(w, d.renderFocusedRow(contentW, d.addRow, prefix+"  "+it.Title()))
                        return
                }
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

func (d outlineItemDelegate) renderFocusedOutlineRow(width int, prefix string, it outlineRowItem) string {
        bg := d.selected.GetBackground()

        base := lipgloss.NewStyle().
                Foreground(d.selected.GetForeground()).
                Background(bg).
                Bold(true)

        indent := strings.Repeat("  ", it.row.depth)
        twisty := " "
        if it.row.hasChildren {
                if it.row.collapsed {
                        twisty = "▸"
                } else {
                        twisty = "▾"
                }
        }

        lead := base.Render(prefix + indent + twisty + " ")
        title := it.row.item.Title

        // Status is rendered as its own styled segment so that its internal ANSI
        // reset doesn't wipe out the focused-row background for the rest of the row.
        statusID := strings.TrimSpace(it.row.item.StatusID)
        statusTxt := strings.ToUpper(strings.TrimSpace(statusLabel(it.outline, statusID)))
        var statusSeg string
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
                statusSeg = style.Copy().Background(bg).Render(statusTxt) + base.Render(" ")
        }

        main := base.Render(title)
        cookie := renderProgressCookie(it.row.doneChildren, it.row.totalChildren)
        out := lead + statusSeg + main + cookie

        // Fill to full width so the background highlight covers the whole row.
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
