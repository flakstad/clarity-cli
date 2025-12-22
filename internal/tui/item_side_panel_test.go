package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"

        "github.com/charmbracelet/lipgloss"
)

func TestRenderAccordionWorklog_UsesFullHeightForExpandedBody(t *testing.T) {
        // This is a regression test: previously we always reserved vertical space for
        // optional "more" indicators + a spacer, which caused the expanded content to
        // be cut off even when there was no list context to show (single-entry view).

        bodyLines := make([]string, 0, 40)
        for i := 1; i <= 25; i++ {
                // Ordered list tends to preserve one row per item in glamour rendering.
                bodyLines = append(bodyLines, formatOrderedListLine(i))
        }

        worklog := []model.WorklogEntry{
                {
                        ID:        "wl-1",
                        ItemID:    "item-1",
                        AuthorID:  "act-1",
                        Body:      strings.Join(bodyLines, "\n"),
                        CreatedAt: time.Unix(0, 0).UTC(),
                },
        }

        height := 12
        lines := renderAccordionWorklog(nil, worklog, 0, 70, height, 0, lipgloss.NewStyle(), lipgloss.NewStyle())
        out := strings.Join(lines, "\n")

        // With height=12 and a single entry, we should be able to show at least 11 body rows.
        // Ensure a later token is present; older behavior typically cut off earlier.
        if !strings.Contains(out, "L11") {
                t.Fatalf("expected expanded body to include later lines (e.g. L11); got:\n%s", out)
        }
}

func formatOrderedListLine(i int) string {
        // Stable token we can assert on after markdown rendering.
        // Example: "11. L11"
        s := ""
        if i < 10 {
                s = "0"
        }
        return strings.TrimSpace(strings.Join([]string{
                intToString(i) + ".",
                "L" + s + intToString(i),
        }, " "))
}

func intToString(n int) string {
        if n == 0 {
                return "0"
        }
        neg := false
        if n < 0 {
                neg = true
                n = -n
        }
        var buf [32]byte
        i := len(buf)
        for n > 0 {
                i--
                buf[i] = byte('0' + (n % 10))
                n /= 10
        }
        if neg {
                i--
                buf[i] = '-'
        }
        return string(buf[i:])
}
