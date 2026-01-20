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

type compactItemDelegate struct {
	normal   lipgloss.Style
	selected lipgloss.Style
}

func newCompactItemDelegate() compactItemDelegate {
	return compactItemDelegate{
		normal: lipgloss.NewStyle().
			Foreground(colorSurfaceFg),
		selected: lipgloss.NewStyle().
			Foreground(colorSelectedFg).
			Background(colorSelectedBg).
			Bold(true),
	}
}

func (d compactItemDelegate) Height() int  { return 1 }
func (d compactItemDelegate) Spacing() int { return 0 }
func (d compactItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d compactItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	contentW := m.Width()
	if contentW < 4 {
		fmt.Fprint(w, "")
		return
	}

	style := d.normal
	if index == m.Index() {
		style = d.selected
	}

	txt := ""
	if t, ok := item.(interface{ Title() string }); ok {
		txt = t.Title()
	} else {
		txt = fmt.Sprint(item)
	}

	line := txt
	lineW := xansi.StringWidth(line)
	if lineW < contentW {
		line += strings.Repeat(" ", contentW-lineW)
	} else if lineW > contentW {
		line = xansi.Cut(line, 0, contentW)
	}

	fmt.Fprint(w, style.Render(line))
}

// focusAwareCompactItemDelegate is like compactItemDelegate, but only highlights the selected row
// when the list is focused. This is useful when a modal has multiple focusable regions (e.g. tags:
// input + list) and we want a single active highlight at a time.
type focusAwareCompactItemDelegate struct {
	normal   lipgloss.Style
	selected lipgloss.Style
	active   *bool
}

func newFocusAwareCompactItemDelegate(active *bool) focusAwareCompactItemDelegate {
	return focusAwareCompactItemDelegate{
		normal: lipgloss.NewStyle().
			Foreground(colorSurfaceFg),
		selected: lipgloss.NewStyle().
			Foreground(colorSelectedFg).
			Background(colorSelectedBg).
			Bold(true),
		active: active,
	}
}

func (d focusAwareCompactItemDelegate) Height() int  { return 1 }
func (d focusAwareCompactItemDelegate) Spacing() int { return 0 }
func (d focusAwareCompactItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d focusAwareCompactItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	contentW := m.Width()
	if contentW < 4 {
		fmt.Fprint(w, "")
		return
	}

	style := d.normal
	active := true
	if d.active != nil {
		active = *d.active
	}
	if index == m.Index() && active {
		style = d.selected
	}

	txt := ""
	if t, ok := item.(interface{ Title() string }); ok {
		txt = t.Title()
	} else {
		txt = fmt.Sprint(item)
	}

	line := txt
	lineW := xansi.StringWidth(line)
	if lineW < contentW {
		line += strings.Repeat(" ", contentW-lineW)
	} else if lineW > contentW {
		line = xansi.Cut(line, 0, contentW)
	}

	fmt.Fprint(w, style.Render(line))
}
