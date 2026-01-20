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
			Foreground(colorSelectedFg).
			Background(colorSelectedBg).
			Bold(true),
		addRow: lipgloss.NewStyle().
			Foreground(ac("240", "245")).
			Italic(true),
	}
}

// focusAwareOutlineItemDelegate is like outlineItemDelegate, but only highlights the selected row
// when active is true. This prevents "double highlights" when multiple panes are visible.
type focusAwareOutlineItemDelegate struct {
	outlineItemDelegate
	active *bool
}

func newFocusAwareOutlineItemDelegate(active *bool) focusAwareOutlineItemDelegate {
	return focusAwareOutlineItemDelegate{
		outlineItemDelegate: newOutlineItemDelegate(),
		active:              active,
	}
}

func (d focusAwareOutlineItemDelegate) Height() int  { return 1 }
func (d focusAwareOutlineItemDelegate) Spacing() int { return 0 }
func (d focusAwareOutlineItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d focusAwareOutlineItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	contentW := m.Width()
	if contentW < 4 {
		fmt.Fprint(w, "")
		return
	}

	active := true
	if d.active != nil {
		active = *d.active
	}
	focused := index == m.Index() && active

	prefix := ""
	switch it := item.(type) {
	case outlineRowItem:
		fmt.Fprint(w, d.outlineItemDelegate.renderOutlineRow(contentW, prefix, it, focused))
		return
	case outlineActivityRowItem:
		fmt.Fprint(w, d.outlineItemDelegate.renderOutlineActivityRow(contentW, prefix, it, focused))
		return
	case outlineDescRowItem:
		fmt.Fprint(w, d.outlineItemDelegate.renderOutlineDescRow(contentW, prefix, it, focused))
		return
	case addItemRow:
		line := prefix + "  " + it.Title()
		if focused {
			fmt.Fprint(w, d.outlineItemDelegate.renderFocusedRow(contentW, d.addRow, line))
			return
		}
		fmt.Fprint(w, d.outlineItemDelegate.renderRow(contentW, d.addRow, line))
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
		line = prefix + "  " + txt
	}
	if focused {
		fmt.Fprint(w, d.outlineItemDelegate.renderFocusedRow(contentW, base, line))
		return
	}
	fmt.Fprint(w, d.outlineItemDelegate.renderRow(contentW, base, line))
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
	case outlineActivityRowItem:
		fmt.Fprint(w, d.renderOutlineActivityRow(contentW, prefix, it, focused))
		return
	case outlineDescRowItem:
		fmt.Fprint(w, d.renderOutlineDescRow(contentW, prefix, it, focused))
		return
	case addItemRow:
		// Match the outline's twisty column (2 chars) so "+ New" aligns.
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
		// Match the outline's twisty column (2 chars) so "+ New" aligns.
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

	// Short-lived error flash (e.g. permission denied).
	flashFg := colorSelectedFg
	flashBg := ac("196", "160") // red
	if it.flashKind == "error" {
		bg = flashBg
	}

	base := d.normal
	if it.flashKind == "error" {
		base = base.Copy().Foreground(flashFg).Background(bg).Bold(true)
	}
	if focused {
		base = lipgloss.NewStyle().
			Foreground(flashFg).
			Background(bg).
			Bold(true)
	}

	indent := strings.Repeat("  ", it.row.depth)
	twisty := " "
	if it.row.hasChildren || it.row.hasDescription {
		if it.row.collapsed {
			twisty = glyphTwistyCollapsed()
		} else {
			twisty = glyphTwistyExpanded()
		}
	}

	leadRaw := prefix + indent + twisty + " "
	leadSeg := base.Render(leadRaw)

	statusID := strings.TrimSpace(it.row.item.StatusID)
	statusTxt := strings.ToUpper(strings.TrimSpace(statusLabel(it.outline, statusID)))
	if it.row.checkbox {
		checked := isCheckboxChecked(it.outline, statusID)
		if glyphs() == glyphSetASCII {
			statusTxt = "[ ]"
			if checked {
				statusTxt = "[x]"
			}
		} else {
			statusTxt = "☐"
			if checked {
				statusTxt = "☑"
			}
		}
	}
	statusRaw := ""
	statusSeg := ""
	if statusTxt != "" {
		style := statusNonEndStyle
		if it.row.checkbox {
			if isCheckboxChecked(it.outline, statusID) {
				style = statusEndStyle
			}
		} else if isEndState(it.outline, statusID) {
			style = statusEndStyle
		}
		if focused || it.flashKind == "error" {
			style = style.Copy().Background(bg)
		}
		statusSeg = style.Render(statusTxt) + base.Render(" ")
		statusRaw = statusTxt + " "
	}

	metaParts := make([]string, 0, 10)

	// Progress cookie should follow immediately after the title (not float on the right).
	progressCookie := ""
	if it.row.totalChildren > 0 {
		// Keep the "progress cookie" visual from earlier versions (like outline.js),
		// but render it adjacent to the title for better scanability.
		progressCookie = renderProgressCookie(it.row.doneChildren, it.row.totalChildren)
	}
	progressW := xansi.StringWidth(progressCookie)

	if it.row.item.Priority {
		st := metaPriorityStyle
		if focused {
			st = st.Background(bg)
		}
		metaParts = append(metaParts, st.Render("priority"))
	}
	if it.row.item.OnHold {
		st := metaOnHoldStyle
		if focused {
			st = st.Background(bg)
		}
		metaParts = append(metaParts, st.Render("on hold"))
	}
	if s := strings.TrimSpace(formatScheduleLabel(it.row.item.Schedule)); s != "" {
		st := metaScheduleStyle
		if focused {
			st = st.Background(bg)
		}
		metaParts = append(metaParts, st.Render(s))
	}
	if s := strings.TrimSpace(formatDueLabel(it.row.item.Due)); s != "" {
		st := metaDueStyle
		if focused {
			st = st.Background(bg)
		}
		metaParts = append(metaParts, st.Render(s))
	}
	if lbl := strings.TrimSpace(it.row.assignedLabel); lbl != "" {
		st := metaAssignStyle
		if focused {
			st = st.Background(bg)
		}
		metaParts = append(metaParts, st.Render("@"+lbl))
	}
	if it.row.commentsCount > 0 {
		st := metaCommentStyle
		if focused {
			st = st.Background(bg)
		}
		metaParts = append(metaParts, st.Render(fmt.Sprintf("c:%d", it.row.commentsCount)))
	}
	for _, tag := range it.row.item.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		st := metaTagStyle
		if focused {
			st = st.Background(bg)
		}
		metaParts = append(metaParts, st.Render("#"+tag))
	}
	inlineMetaSeg := strings.Join(metaParts, base.Render(" "))
	inlineMetaW := xansi.StringWidth(inlineMetaSeg)

	title := strings.TrimSpace(it.row.item.Title)
	if title == "" {
		title = "(untitled)"
	}

	// Reserve room for progress + inline metadata; truncate the title ONLY when we're out of space.
	leadW := xansi.StringWidth(leadRaw)
	statusW := xansi.StringWidth(statusRaw)
	availTitle := width - leadW - statusW
	if progressCookie != "" {
		availTitle -= progressW
	}
	if inlineMetaSeg != "" {
		availTitle -= (1 + inlineMetaW) // space + inline metadata
	}
	if availTitle < 0 {
		availTitle = 0
	}

	titleTrunc := truncateText(title, availTitle)
	titleStyle := base
	if isEndState(it.outline, statusID) {
		titleStyle = faintIfDark(base.Copy()).
			Foreground(colorMuted).
			Strikethrough(true)
	}
	titleSeg := titleStyle.Render(titleTrunc)
	progressSeg := progressCookie
	metaSpacer := ""
	if inlineMetaSeg != "" {
		metaSpacer = base.Render(" ")
	}
	out := leadSeg + statusSeg + titleSeg + progressSeg + metaSpacer + inlineMetaSeg
	curW := xansi.StringWidth(out)
	if curW < width {
		out += base.Render(strings.Repeat(" ", width-curW))
	} else if curW > width {
		// IMPORTANT: when cutting ANSI-styled strings, ensure we always terminate styling.
		// Otherwise some terminals will "bleed" background/bold into the next line, which
		// can look like an extra blank highlighted row.
		out = xansi.Cut(out, 0, width) + "\x1b[0m"
	}
	return out
}

func (d outlineItemDelegate) renderOutlineActivityRow(width int, prefix string, it outlineActivityRowItem, focused bool) string {
	base := d.normal
	if focused {
		base = d.selected
	}

	indent := strings.Repeat("  ", it.depth)
	twisty := " "
	if it.hasChildren || it.hasDescription {
		if it.collapsed {
			twisty = glyphTwistyCollapsed()
		} else {
			twisty = glyphTwistyExpanded()
		}
	}
	line := prefix + indent + twisty + " " + strings.TrimSpace(it.label)
	return d.renderRow(width, base, line)
}

func (d outlineItemDelegate) renderOutlineDescRow(width int, prefix string, it outlineDescRowItem, focused bool) string {
	bg := d.selected.GetBackground()

	base := d.normal
	if focused {
		base = lipgloss.NewStyle().
			Foreground(d.selected.GetForeground()).
			Background(bg).
			Bold(true)
	}

	indent := strings.Repeat("  ", it.depth)
	leadRaw := prefix + indent + "  "
	leadSeg := base.Render(leadRaw)

	avail := width - xansi.StringWidth(leadRaw)
	if avail < 0 {
		avail = 0
	}
	line := strings.TrimRight(it.line, " \t")
	if focused {
		// For consistent selection highlighting, render focused rows as plain text.
		line = xansi.Strip(line)
	}
	if xansi.StringWidth(line) > avail {
		if focused {
			line = truncateText(line, avail)
		} else {
			line = truncateStyledText(line, avail)
		}
	}

	txtSeg := line
	if focused {
		txtSeg = base.Render(line)
	}

	out := leadSeg + txtSeg
	curW := xansi.StringWidth(out)
	if curW < width {
		out += base.Render(strings.Repeat(" ", width-curW))
	} else if curW > width {
		out = xansi.Cut(out, 0, width) + "\x1b[0m"
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

func truncateStyledText(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if xansi.StringWidth(s) <= maxW {
		return s
	}
	if maxW == 1 {
		return "…"
	}
	// Ensure any open ANSI styling is always terminated.
	return xansi.Cut(s, 0, maxW-1) + "…" + "\x1b[0m"
}
