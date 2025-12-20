package tui

import (
        "strings"

        xansi "github.com/charmbracelet/x/ansi"
)

// normalizePane forces s to be exactly width columns wide (ANSI-aware) and height
// lines tall. This makes split-pane rendering stable when using lipgloss.JoinHorizontal.
func normalizePane(s string, width, height int) string {
        if width < 0 {
                width = 0
        }
        if height < 0 {
                height = 0
        }

        lines := strings.Split(s, "\n")

        if height > 0 {
                if len(lines) > height {
                        lines = lines[:height]
                }
                for len(lines) < height {
                        lines = append(lines, "")
                }
        }

        for i := range lines {
                ln := lines[i]
                w := xansi.StringWidth(ln)

                if w > width {
                        if width <= 0 {
                                ln = ""
                        } else if width == 1 {
                                ln = xansi.Cut(ln, 0, 1)
                        } else {
                                ln = xansi.Cut(ln, 0, width-1) + "…"
                        }
                        w = xansi.StringWidth(ln)
                }
                if w < width {
                        ln = ln + strings.Repeat(" ", width-w)
                }
                lines[i] = ln
        }

        return strings.Join(lines, "\n")
}
