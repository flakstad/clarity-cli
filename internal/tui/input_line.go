package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

func renderInputLine(bodyW int, inputView string) string {
	if bodyW < 10 {
		bodyW = 10
	}

	// Text inputs should always render as a single visual line inside modals.
	// If the view ever contains newlines (or overflows due to ANSI/cursor styling),
	// it can trigger wrapping behavior that looks like "newline insertion" while typing.
	inputView = strings.ReplaceAll(inputView, "\n", " ")
	inputView = strings.ReplaceAll(inputView, "\r", " ")

	line := lipgloss.PlaceHorizontal(
		bodyW,
		lipgloss.Left,
		" "+inputView+" ",
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceBackground(colorInputBg),
	)
	if xansi.StringWidth(line) > bodyW {
		// Ensure we never exceed the modal body width; terminate ANSI styling to prevent bleed.
		line = xansi.Cut(line, 0, bodyW) + "\x1b[0m"
	}
	return line
}
