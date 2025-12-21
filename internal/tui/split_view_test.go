package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/bubbles/list"
        tea "github.com/charmbracelet/bubbletea"
        xansi "github.com/charmbracelet/x/ansi"
)

func TestViewOutline_SplitPreview_PanesStayFixedWidthPerLine(t *testing.T) {
        db := &store.DB{
                CurrentActorID: "act-test",
                Actors:         []model.Actor{{ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"}},
                Projects: []model.Project{{
                        ID:        "proj-a",
                        Name:      "Project A",
                        CreatedBy: "act-test",
                        CreatedAt: time.Now().UTC(),
                }},
                Outlines: []model.Outline{{
                        ID:         "out-a",
                        ProjectID:  "proj-a",
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  "act-test",
                        CreatedAt:  time.Now().UTC(),
                }},
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj-a",
                        OutlineID:    "out-a",
                        Rank:         "h",
                        Title:        "Title",
                        Description:  strings.Repeat("X", 500), // ensure detail view has lots of wrapped content
                        StatusID:     "todo",
                        Priority:     false,
                        OnHold:       false,
                        Archived:     false,
                        OwnerActorID: "act-test",
                        CreatedBy:    "act-test",
                        CreatedAt:    time.Now().UTC(),
                        UpdatedAt:    time.Now().UTC(),
                }},
        }

        m := newAppModel(t.TempDir(), db)
        m.view = viewOutline
        m.showPreview = true
        m.modal = modalNone
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        // Use a wide terminal so split preview is actually visible.
        m.width = 100
        m.height = 30

        m.itemsList.SetItems([]list.Item{
                outlineRowItem{
                        row: outlineRow{
                                item:          db.Items[0],
                                depth:         0,
                                hasChildren:   false,
                                collapsed:     false,
                                doneChildren:  0,
                                totalChildren: 0,
                        },
                        outline: db.Outlines[0],
                },
        })

        out := m.viewOutline()
        lines := strings.Split(out, "\n")

        frameH := m.height - 6
        if frameH < 8 {
                frameH = 8
        }
        bodyHeight := frameH - (topPadLines + breadcrumbGap + 2)
        if bodyHeight < 6 {
                bodyHeight = 6
        }

        // The join block starts after:
        // - topPadLines leading blank lines
        // - 1 breadcrumb line
        // - breadcrumbGap blank lines
        start := topPadLines + 1 + breadcrumbGap

        if len(lines) < start+bodyHeight {
                t.Fatalf("unexpected outline view height: got %d lines, need at least %d", len(lines), start+bodyHeight)
        }

        for i := start; i < start+bodyHeight; i++ {
                if w := xansi.StringWidth(lines[i]); w != m.width {
                        t.Fatalf("expected split-view line %d to be exactly %d cols wide; got %d: %q", i, m.width, w, lines[i])
                }
        }
}

func TestViewOutline_NarrowTerminal_AutoCollapsesPreview(t *testing.T) {
        db := &store.DB{
                CurrentActorID: "act-test",
                Actors:         []model.Actor{{ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"}},
                Projects: []model.Project{{
                        ID:        "proj-a",
                        Name:      "Project A",
                        CreatedBy: "act-test",
                        CreatedAt: time.Now().UTC(),
                }},
                Outlines: []model.Outline{{
                        ID:         "out-a",
                        ProjectID:  "proj-a",
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  "act-test",
                        CreatedAt:  time.Now().UTC(),
                }},
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj-a",
                        OutlineID:    "out-a",
                        Rank:         "h",
                        Title:        "Title",
                        Description:  strings.Repeat("X", 200),
                        StatusID:     "todo",
                        Priority:     false,
                        OnHold:       false,
                        Archived:     false,
                        OwnerActorID: "act-test",
                        CreatedBy:    "act-test",
                        CreatedAt:    time.Now().UTC(),
                        UpdatedAt:    time.Now().UTC(),
                }},
        }

        m := newAppModel(t.TempDir(), db)
        m.view = viewOutline
        m.showPreview = true
        m.modal = modalNone
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.width = 40 // narrower than minSplitPreviewW
        m.height = 20

        m.itemsList.SetItems([]list.Item{
                outlineRowItem{
                        row: outlineRow{
                                item:          db.Items[0],
                                depth:         0,
                                hasChildren:   false,
                                collapsed:     false,
                                doneChildren:  0,
                                totalChildren: 0,
                        },
                        outline: db.Outlines[0],
                },
        })

        _ = m.viewOutline()

        // Even if the user toggled preview on, narrow terminals should auto-collapse it.
        if strings.Contains(m.footerText(), "tab: toggle focus") {
                t.Fatalf("expected no tab focus hint on narrow terminals when preview is not visible; got footer: %q", m.footerText())
        }
        if got := m.itemsList.Width(); got != m.width {
                t.Fatalf("expected outline list to use full width on narrow terminals; got %d, want %d", got, m.width)
        }
}

func TestView_ShowsResizingOverlay(t *testing.T) {
        db := &store.DB{
                CurrentActorID: "act-test",
                Actors:         []model.Actor{{ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"}},
        }
        m := newAppModel(t.TempDir(), db)
        m.view = viewProjects
        m.width = 60
        m.height = 20
        m.resizing = true

        out := m.View()
        if !strings.Contains(out, "Resizing") {
                t.Fatalf("expected resizing overlay to contain 'Resizing'; got: %q", out)
        }
        // Ensure we render a full-height block (stable during resize).
        lines := strings.Split(out, "\n")
        if len(lines) != m.height {
                t.Fatalf("expected resizing overlay to be exactly %d lines tall; got %d", m.height, len(lines))
        }
}

func TestUpdate_WindowSizeMsg_DebouncesResizingFlag(t *testing.T) {
        db := &store.DB{
                CurrentActorID: "act-test",
                Actors:         []model.Actor{{ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"}},
        }
        m := newAppModel(t.TempDir(), db)

        // First WindowSizeMsg is treated as initial sizing (no resizing overlay).
        mm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
        m = mm.(appModel)
        if !m.resizing {
                // ok
        } else {
                t.Fatalf("expected resizing=false after initial WindowSizeMsg")
        }
        if m.resizeSeq != 0 {
                t.Fatalf("expected resizeSeq=0 after initial WindowSizeMsg, got %d", m.resizeSeq)
        }

        // Second WindowSizeMsg -> enter resizing mode, seq=1
        mm, _ = m.Update(tea.WindowSizeMsg{Width: 81, Height: 25})
        m = mm.(appModel)
        if !m.resizing {
                t.Fatalf("expected resizing=true after subsequent WindowSizeMsg")
        }
        if m.resizeSeq != 1 {
                t.Fatalf("expected resizeSeq=1 after subsequent WindowSizeMsg, got %d", m.resizeSeq)
        }

        // Stale done message should NOT clear.
        mm, _ = m.Update(resizeDoneMsg{seq: 0})
        m = mm.(appModel)
        if !m.resizing {
                t.Fatalf("expected resizing to remain true for stale resizeDoneMsg")
        }

        // Current done message clears.
        mm, _ = m.Update(resizeDoneMsg{seq: 1})
        m = mm.(appModel)
        if m.resizing {
                t.Fatalf("expected resizing=false after latest resizeDoneMsg")
        }
}
