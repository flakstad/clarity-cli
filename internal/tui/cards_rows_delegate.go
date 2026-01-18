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

type cardsRowDelegate struct {
	kind    string // "project" | "outline"
	minimal bool

	normal   lipgloss.Style
	selected lipgloss.Style
	muted    lipgloss.Style
}

func newProjectRowsDelegate(minimal bool) cardsRowDelegate {
	return newCardsRowDelegate("project", minimal)
}

func newOutlineRowsDelegate(minimal bool) cardsRowDelegate {
	return newCardsRowDelegate("outline", minimal)
}

func newCardsRowDelegate(kind string, minimal bool) cardsRowDelegate {
	muted := lipgloss.NewStyle().Foreground(ac("240", "245"))
	if minimal {
		muted = lipgloss.NewStyle().Foreground(colorMuted)
	}
	return cardsRowDelegate{
		kind:    kind,
		minimal: minimal,
		normal:  lipgloss.NewStyle(),
		selected: lipgloss.NewStyle().
			Foreground(colorSelectedFg).
			Background(colorSelectedBg).
			Bold(true),
		muted: muted,
	}
}

func (d cardsRowDelegate) Height() int  { return 1 }
func (d cardsRowDelegate) Spacing() int { return 0 }
func (d cardsRowDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d cardsRowDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	contentW := m.Width()
	if contentW < 8 {
		fmt.Fprint(w, "")
		return
	}

	style := d.normal
	if index == m.Index() {
		style = d.selected
	}

	left := ""
	right := ""

	sep := " " + glyphBullet() + " "
	switch it := item.(type) {
	case projectItem:
		name := strings.TrimSpace(it.project.Name)
		if name == "" {
			name = "(unnamed project)"
		}
		if it.current {
			name += " " + glyphBullet()
		}
		left = name

		open := it.meta.itemsTotal - it.meta.itemsDone - it.meta.itemsOnHold
		if open < 0 {
			open = 0
		}
		if d.minimal {
			right = fmt.Sprintf("%d open%s%d done", open, sep, it.meta.itemsDone)
		} else {
			right = fmt.Sprintf("%d outlines%s%d open%s%d done", it.meta.outlinesTotal, sep, open, sep, it.meta.itemsDone)
		}
	case outlineItem:
		name := outlineDisplayName(it.outline)
		if it.current {
			name += " " + glyphBullet()
		}
		left = name

		open := it.meta.itemsTotal - it.meta.itemsDone - it.meta.itemsOnHold
		if open < 0 {
			open = 0
		}
		if d.minimal {
			right = fmt.Sprintf("%d open%s%d done", open, sep, it.meta.itemsDone)
		} else {
			right = fmt.Sprintf("%d top%s%d open%s%d done", it.meta.topLevel, sep, open, sep, it.meta.itemsDone)
		}
	default:
		left = fmt.Sprint(item)
	}

	if strings.TrimSpace(right) != "" && d.minimal && index != m.Index() {
		right = d.muted.Render(right)
	}

	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)

	// Layout: left content + right-aligned summary.
	gap := 2
	leftW := xansi.StringWidth(left)
	rightW := xansi.StringWidth(right)
	availLeft := contentW
	if rightW > 0 {
		availLeft = contentW - gap - rightW
		if availLeft < 1 {
			availLeft = 1
		}
	}
	if leftW > availLeft {
		left = xansi.Cut(left, 0, availLeft)
		leftW = xansi.StringWidth(left)
	}

	line := left
	if rightW > 0 {
		if leftW < availLeft {
			line += strings.Repeat(" ", availLeft-leftW)
		}
		line += strings.Repeat(" ", gap) + right
	}

	lineW := xansi.StringWidth(line)
	if lineW < contentW {
		line += strings.Repeat(" ", contentW-lineW)
	} else if lineW > contentW {
		line = xansi.Cut(line, 0, contentW)
	}

	fmt.Fprint(w, style.Render(line))
}
