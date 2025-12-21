package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"

        "github.com/charmbracelet/lipgloss"
        "github.com/muesli/termenv"
)

func TestOutlineDelegate_EndStateItem_IsGrayAndStruck(t *testing.T) {
        old := lipgloss.ColorProfile()
        lipgloss.SetColorProfile(termenv.ANSI256)
        t.Cleanup(func() { lipgloss.SetColorProfile(old) })

        // Make theme selection deterministic for tests.
        oldBg := lipgloss.HasDarkBackground()
        lipgloss.SetHasDarkBackground(true)
        t.Cleanup(func() { lipgloss.SetHasDarkBackground(oldBg) })

        outline := model.Outline{
                ID:        "out-a",
                ProjectID: "proj-a",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "TODO", IsEndState: false},
                        {ID: "done", Label: "DONE", IsEndState: true},
                },
        }

        it := outlineRowItem{
                outline: outline,
                row: outlineRow{
                        item: model.Item{
                                ID:        "item-a",
                                OutlineID: "out-a",
                                Title:     "Ship it",
                                StatusID:  "done",
                                CreatedAt: time.Now().UTC(),
                                UpdatedAt: time.Now().UTC(),
                        },
                },
        }

        d := newOutlineItemDelegate()
        got := d.renderOutlineRow(80, "", it, false)

        // Strikethrough is SGR 9. Lipgloss typically emits \x1b[9m or combines it with other SGRs.
        if !strings.Contains(got, ";9m") && !strings.Contains(got, "[9m") {
                t.Fatalf("expected strikethrough escape code in rendered row; got: %q", got)
        }
        // Gray-ish 256-color fg: 38;5;243
        if !strings.Contains(got, "38;5;243") {
                t.Fatalf("expected gray foreground (38;5;243) in rendered row; got: %q", got)
        }
}

func TestOutlineDelegate_EndStateItem_LightTheme_IsMutedAndStruck(t *testing.T) {
        old := lipgloss.ColorProfile()
        lipgloss.SetColorProfile(termenv.ANSI256)
        t.Cleanup(func() { lipgloss.SetColorProfile(old) })

        oldBg := lipgloss.HasDarkBackground()
        lipgloss.SetHasDarkBackground(false)
        t.Cleanup(func() { lipgloss.SetHasDarkBackground(oldBg) })

        outline := model.Outline{
                ID:        "out-a",
                ProjectID: "proj-a",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "TODO", IsEndState: false},
                        {ID: "done", Label: "DONE", IsEndState: true},
                },
        }

        it := outlineRowItem{
                outline: outline,
                row: outlineRow{
                        item: model.Item{
                                ID:        "item-a",
                                OutlineID: "out-a",
                                Title:     "Ship it",
                                StatusID:  "done",
                                CreatedAt: time.Now().UTC(),
                                UpdatedAt: time.Now().UTC(),
                        },
                },
        }

        d := newOutlineItemDelegate()
        got := d.renderOutlineRow(80, "", it, false)

        if !strings.Contains(got, ";9m") && !strings.Contains(got, "[9m") {
                t.Fatalf("expected strikethrough escape code in rendered row; got: %q", got)
        }
        // Muted 256-color fg on light terminals: 38;5;240
        if !strings.Contains(got, "38;5;240") {
                t.Fatalf("expected muted foreground (38;5;240) in rendered row; got: %q", got)
        }
}

func TestOutlineDelegate_NonEndStateItem_IsNotStruck(t *testing.T) {
        old := lipgloss.ColorProfile()
        lipgloss.SetColorProfile(termenv.ANSI256)
        t.Cleanup(func() { lipgloss.SetColorProfile(old) })

        oldBg := lipgloss.HasDarkBackground()
        lipgloss.SetHasDarkBackground(true)
        t.Cleanup(func() { lipgloss.SetHasDarkBackground(oldBg) })

        outline := model.Outline{
                ID:        "out-a",
                ProjectID: "proj-a",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "TODO", IsEndState: false},
                        {ID: "done", Label: "DONE", IsEndState: true},
                },
        }

        it := outlineRowItem{
                outline: outline,
                row: outlineRow{
                        item: model.Item{
                                ID:        "item-a",
                                OutlineID: "out-a",
                                Title:     "Do it",
                                StatusID:  "todo",
                                CreatedAt: time.Now().UTC(),
                                UpdatedAt: time.Now().UTC(),
                        },
                },
        }

        d := newOutlineItemDelegate()
        got := d.renderOutlineRow(80, "", it, false)

        if strings.Contains(got, ";9m") || strings.Contains(got, "[9m") {
                t.Fatalf("did not expect strikethrough escape code for non-end-state row; got: %q", got)
        }
}
