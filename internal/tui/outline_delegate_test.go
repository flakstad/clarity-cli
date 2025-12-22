package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"

        "github.com/charmbracelet/lipgloss"
        "github.com/muesli/termenv"
)

func TestOutlineDelegate_DoesNotHardCapTitleWidth(t *testing.T) {
        d := newOutlineItemDelegate()

        outline := model.Outline{
                ID:        "out-a",
                ProjectID: "proj-a",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "TODO", IsEndState: false},
                },
                CreatedBy: "act-human",
                CreatedAt: time.Now().UTC(),
        }

        title := strings.Repeat("X", 80) // longer than old 50-char cap
        it := outlineRowItem{
                outline: outline,
                row: outlineRow{
                        item: model.Item{
                                ID:           "item-a",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "h",
                                Title:        title,
                                StatusID:     "todo",
                                OwnerActorID: "act-human",
                                CreatedBy:    "act-human",
                                CreatedAt:    time.Now().UTC(),
                                UpdatedAt:    time.Now().UTC(),
                        },
                        depth:       0,
                        hasChildren: false,
                        collapsed:   false,
                },
        }

        // Wide enough that we should not truncate at all.
        out := d.renderOutlineRow(140, "", it, false)
        if strings.Contains(out, "…") {
                t.Fatalf("expected no ellipsis in wide render; got %q", out)
        }
        if !strings.Contains(out, title) {
                t.Fatalf("expected full title to be present (len=%d)", len(title))
        }
}

func TestOutlineDelegate_MetaTakesPrecedenceOverTitle(t *testing.T) {
        d := newOutlineItemDelegate()

        outline := model.Outline{
                ID:        "out-a",
                ProjectID: "proj-a",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "TODO", IsEndState: false},
                },
                CreatedBy: "act-human",
                CreatedAt: time.Now().UTC(),
        }

        title := strings.Repeat("A", 200)
        it := outlineRowItem{
                outline: outline,
                row: outlineRow{
                        item: model.Item{
                                ID:           "item-a",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "h",
                                Title:        title,
                                StatusID:     "todo",
                                Priority:     true,
                                OwnerActorID: "act-human",
                                CreatedBy:    "act-human",
                                CreatedAt:    time.Now().UTC(),
                                UpdatedAt:    time.Now().UTC(),
                        },
                        depth:         0,
                        hasChildren:   true,
                        collapsed:     false,
                        doneChildren:  1,
                        totalChildren: 2,
                },
        }

        // Narrow enough that title must truncate; meta should still be present.
        out := d.renderOutlineRow(60, "", it, false)
        if !strings.Contains(out, "…") {
                t.Fatalf("expected title truncation (ellipsis) in narrow render; got %q", out)
        }
        if !strings.Contains(out, "priority") {
                t.Fatalf("expected meta 'priority' to be present; got %q", out)
        }
        if !strings.Contains(out, "1/2") {
                t.Fatalf("expected progress cookie to include 1/2; got %q", out)
        }
}

func TestOutlineDelegate_ProgressCookieFollowsTitle(t *testing.T) {
        d := newOutlineItemDelegate()

        outline := model.Outline{
                ID:        "out-a",
                ProjectID: "proj-a",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "TODO", IsEndState: false},
                },
                CreatedBy: "act-human",
                CreatedAt: time.Now().UTC(),
        }

        title := "MyTitle"
        it := outlineRowItem{
                outline: outline,
                row: outlineRow{
                        item: model.Item{
                                ID:           "item-a",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "h",
                                Title:        title,
                                StatusID:     "todo",
                                Priority:     true, // keep some meta present
                                OwnerActorID: "act-human",
                                CreatedBy:    "act-human",
                                CreatedAt:    time.Now().UTC(),
                                UpdatedAt:    time.Now().UTC(),
                        },
                        depth:         0,
                        hasChildren:   true,
                        collapsed:     false,
                        doneChildren:  1,
                        totalChildren: 2,
                },
        }

        out := d.renderOutlineRow(80, "", it, false)
        plain := stripANSIEscapes(out)

        titleIdx := strings.Index(plain, title)
        if titleIdx < 0 {
                t.Fatalf("expected rendered row to include title %q; got %q", title, plain)
        }
        cookieIdx := strings.Index(plain, "1/2")
        if cookieIdx < 0 {
                t.Fatalf("expected rendered row to include progress cookie 1/2; got %q", plain)
        }

        // The cookie includes a leading space plus a small fixed-width bar; it should appear
        // close to the title, not separated by a large right-alignment spacer.
        gap := cookieIdx - (titleIdx + len(title))
        if gap > 20 {
                t.Fatalf("expected progress cookie to follow title closely; gap=%d; got %q", gap, plain)
        }
}

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
