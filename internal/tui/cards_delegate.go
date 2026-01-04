package tui

import (
        "fmt"
        "io"
        "strings"
        "time"

        "github.com/charmbracelet/bubbles/list"
        tea "github.com/charmbracelet/bubbletea"
        "github.com/charmbracelet/lipgloss"
        xansi "github.com/charmbracelet/x/ansi"
)

type cardDelegate struct {
        kind string // "project" | "outline"

        normalCard   lipgloss.Style
        selectedCard lipgloss.Style

        titleStyle         lipgloss.Style
        titleSelectedStyle lipgloss.Style

        metaStyle lipgloss.Style
}

func newProjectCardDelegate() cardDelegate {
        return newCardDelegate("project")
}

func newOutlineCardDelegate() cardDelegate {
        return newCardDelegate("outline")
}

func newCardDelegate(kind string) cardDelegate {
        base := lipgloss.NewStyle().
                Width(0). // Set per-render.
                Padding(0, 1, 0, 2).
                Border(lipgloss.RoundedBorder()).
                BorderForeground(colorMuted).
                Foreground(colorSurfaceFg)

        selected := lipgloss.NewStyle().
                Width(0).
                Padding(0, 1, 0, 2).
                Border(lipgloss.RoundedBorder()).
                BorderForeground(colorAccent).
                Foreground(colorSurfaceFg)

        return cardDelegate{
                kind:               kind,
                normalCard:         base,
                selectedCard:       selected,
                titleStyle:         lipgloss.NewStyle().Bold(true).Foreground(colorSurfaceFg),
                titleSelectedStyle: lipgloss.NewStyle().Bold(true).Foreground(colorSurfaceFg),
                metaStyle:          lipgloss.NewStyle().Foreground(ac("238", "250")),
        }
}

func (d cardDelegate) Height() int  { return 5 } // 3 inner lines + border top/bottom
func (d cardDelegate) Spacing() int { return 1 }
func (d cardDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
        return nil
}

func (d cardDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
        totalW := m.Width()
        if totalW < 12 {
                fmt.Fprint(w, "")
                return
        }

        selected := index == m.Index()
        card := d.normalCard
        titleSt := d.titleStyle
        if selected {
                card = d.selectedCard
                titleSt = d.titleSelectedStyle
        }
        frameW := card.GetHorizontalFrameSize()
        innerW := totalW - frameW
        if innerW < 1 {
                innerW = 1
        }
        card = card.Width(innerW)

        lines := make([]string, 0, 3)
        switch it := item.(type) {
        case projectItem:
                title := strings.TrimSpace(it.project.Name)
                if title == "" {
                        title = "(unnamed project)"
                }
                if it.current {
                        title = "• " + title
                }

                created := fmtDate(it.project.CreatedAt)
                updated := created
                if it.meta.hasUpdated {
                        updated = fmtDate(it.meta.updatedAt)
                }

                outlinesLine := fmt.Sprintf("%d outlines", it.meta.outlinesTotal)
                if it.meta.outlinesArchived > 0 {
                        outlinesLine += fmt.Sprintf(" (+%d archived)", it.meta.outlinesArchived)
                }

                open := it.meta.itemsTotal - it.meta.itemsDone - it.meta.itemsOnHold
                if open < 0 {
                        open = 0
                }
                itemsLine := fmt.Sprintf(
                        "%d items • %d open • %d done",
                        it.meta.itemsTotal,
                        open,
                        it.meta.itemsDone,
                )
                if it.meta.itemsOnHold > 0 {
                        itemsLine += fmt.Sprintf(" • %d on hold", it.meta.itemsOnHold)
                }
                if it.meta.itemsNoStatus > 0 {
                        itemsLine += fmt.Sprintf(" • %d no status", it.meta.itemsNoStatus)
                }

                lines = append(lines,
                        titleSt.Render(title),
                        d.metaStyle.Render(outlinesLine+"  |  "+itemsLine),
                        d.metaStyle.Render("created "+created+"  |  updated "+updated),
                )
        case outlineItem:
                title := outlineDisplayName(it.outline)
                lines = append(lines, titleSt.Render(title))

                desc := strings.TrimSpace(it.outline.Description)
                if desc == "" {
                        desc = "(no description)"
                }
                lines = append(lines, d.metaStyle.Render(truncateToWidth(desc, innerW)))

                created := fmtDate(it.outline.CreatedAt)
                updated := created
                if it.meta.hasUpdated {
                        updated = fmtDate(it.meta.updatedAt)
                }

                itemsLine := fmt.Sprintf("%d items", it.meta.itemsTotal)
                if it.meta.itemsWithStatus > 0 {
                        itemsLine += fmt.Sprintf(" (%d with status)", it.meta.itemsWithStatus)
                }
                if it.meta.itemsDone > 0 {
                        itemsLine += fmt.Sprintf(" • %d done", it.meta.itemsDone)
                }
                if it.meta.itemsOnHold > 0 {
                        itemsLine += fmt.Sprintf(" • %d on hold", it.meta.itemsOnHold)
                }
                if it.meta.topLevel > 0 {
                        itemsLine += fmt.Sprintf(" • %d top-level", it.meta.topLevel)
                }
                if it.meta.itemsNoStatus > 0 {
                        itemsLine += fmt.Sprintf(" • %d no status", it.meta.itemsNoStatus)
                }

                lines = append(lines, d.metaStyle.Render(itemsLine+"  |  updated "+updated+"  |  created "+created))
        default:
                txt := fmt.Sprint(item)
                lines = append(lines,
                        titleSt.Render(truncateToWidth(txt, innerW)),
                        d.metaStyle.Render(""),
                        d.metaStyle.Render(""),
                )
        }

        for i := 0; i < len(lines); i++ {
                lines[i] = padOrCutANSI(lines[i], innerW)
        }
        for len(lines) < 3 {
                lines = append(lines, strings.Repeat(" ", innerW))
        }
        body := strings.Join(lines, "\n")
        fmt.Fprint(w, card.Render(body))
}

func fmtDate(t time.Time) string {
        if t.IsZero() {
                return "—"
        }
        return t.UTC().Format("2006-01-02")
}

func truncateToWidth(s string, w int) string {
        s = strings.ReplaceAll(s, "\n", " ")
        s = strings.TrimSpace(s)
        if w <= 0 {
                return ""
        }
        if xansi.StringWidth(s) <= w {
                return s
        }
        if w <= 1 {
                return "…"
        }
        return xansi.Cut(s, 0, w-1) + "…"
}

func padOrCutANSI(s string, w int) string {
        cur := xansi.StringWidth(s)
        switch {
        case cur < w:
                return s + strings.Repeat(" ", w-cur)
        case cur > w:
                return xansi.Cut(s, 0, w) + "\x1b[0m"
        default:
                return s
        }
}
