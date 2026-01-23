package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func renderConfirmModal(width int, title string, body string, confirmLabel string, cancelLabel string, focus confirmModalFocus) string {
	// Avoid borders here: some terminals show background artifacts when nesting bordered
	// components inside a modal with a background color.
	btnBase := lipgloss.NewStyle().
		Padding(0, 1).
		Foreground(colorSurfaceFg).
		Background(colorControlBg)
	btnActive := btnBase.
		Foreground(colorSelectedFg).
		Background(colorSelectedBg).
		Bold(true)

	confirm := btnBase.Render(confirmLabel)
	cancel := btnBase.Render(cancelLabel)
	if focus == confirmFocusConfirm {
		confirm = btnActive.Render(confirmLabel)
	}
	if focus == confirmFocusCancel {
		cancel = btnActive.Render(cancelLabel)
	}

	sep := lipgloss.NewStyle().Background(colorControlBg).Render(" ")
	controls := lipgloss.JoinHorizontal(lipgloss.Top, confirm, sep, cancel)

	bodyW := modalBodyWidth(width)
	help := styleMuted().Width(bodyW).Render("tab: focus   enter: select   esc/ctrl+g: cancel")

	content := strings.Join([]string{
		body,
		"",
		controls,
		"",
		help,
	}, "\n")
	return renderModalBox(width, title, content)
}
