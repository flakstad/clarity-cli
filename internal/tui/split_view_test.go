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

func TestViewOutline_SplitPreview_RendersDetailPaneAndUsesOneThirdWidth(t *testing.T) {
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
        m.width = 100
        m.height = 30

        frameH := m.height - 6
        if frameH < 8 {
                frameH = 8
        }
        bodyH := frameH - (topPadLines + breadcrumbGap + 2)
        if bodyH < 6 {
                bodyH = 6
        }
        contentW := m.width - 2*splitOuterMargin
        if contentW < 10 {
                contentW = 10
        }
        _, rightW := splitPaneWidths(contentW)

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

        // Pre-seed the cache so viewOutline renders the full detail pane.
        m.previewCacheForID = "item-a"
        m.previewCacheW = rightW
        m.previewCacheH = bodyH
        m.previewCache = renderItemDetail(db, db.Outlines[0], db.Items[0], rightW, bodyH, false)

        out := m.viewOutline()
        if strings.Contains(out, "Preview") {
                t.Fatalf("did not expect preview modal overlay; got: %q", out)
        }
        if !strings.Contains(out, "Description") || !strings.Contains(out, "Owner: ") {
                t.Fatalf("expected detail pane content to render; got: %q", out)
        }

        leftW, _ := splitPaneWidths(contentW)
        if got := m.itemsList.Width(); got != leftW {
                t.Fatalf("expected left pane width=%d, got %d", leftW, got)
        }

        // Ensure stable full-width lines (important for split rendering).
        lines := strings.Split(out, "\n")
        for i := 0; i < len(lines) && i < 20; i++ {
                if w := xansi.StringWidth(lines[i]); w != m.width {
                        t.Fatalf("expected line %d width=%d, got %d (line=%q)", i, m.width, w, lines[i])
                }
        }
}

func TestUpdate_PreviewComputeMsg_IsDebouncedBySeqAndSelection(t *testing.T) {
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
                Items: []model.Item{
                        {
                                ID:           "item-a",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "h",
                                Title:        "A",
                                StatusID:     "todo",
                                OwnerActorID: "act-test",
                                CreatedBy:    "act-test",
                                CreatedAt:    time.Now().UTC(),
                                UpdatedAt:    time.Now().UTC(),
                        },
                        {
                                ID:           "item-b",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "i",
                                Title:        "B",
                                StatusID:     "todo",
                                OwnerActorID: "act-test",
                                CreatedBy:    "act-test",
                                CreatedAt:    time.Now().UTC(),
                                UpdatedAt:    time.Now().UTC(),
                        },
                },
        }

        m := newAppModel(t.TempDir(), db)
        m.view = viewOutline
        m.showPreview = true
        m.modal = modalNone
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.width = 100
        m.height = 30

        m.itemsList.SetItems([]list.Item{
                outlineRowItem{row: outlineRow{item: db.Items[0]}, outline: db.Outlines[0]},
                outlineRowItem{row: outlineRow{item: db.Items[1]}, outline: db.Outlines[0]},
        })
        m.itemsList.Select(1) // select item-b

        frameH, bodyH, contentW := m.outlineLayout()
        _ = frameH
        _, rightW := splitPaneWidths(contentW)

        // Set the current sequence to 2 (meaning seq=1 is stale).
        m.previewSeq = 2
        m.previewCacheForID = ""
        m.previewCache = ""

        // Stale message for different item should be ignored.
        mm, _ := m.Update(previewComputeMsg{seq: 1, itemID: "item-a", w: rightW, h: bodyH})
        m = mm.(appModel)
        if m.previewCacheForID != "" {
                t.Fatalf("expected no cache update for stale previewComputeMsg; got cacheForID=%q", m.previewCacheForID)
        }

        // Current message for selected item should update cache.
        mm, _ = m.Update(previewComputeMsg{seq: 2, itemID: "item-b", w: rightW, h: bodyH})
        m = mm.(appModel)
        if m.previewCacheForID != "item-b" || strings.TrimSpace(m.previewCache) == "" {
                t.Fatalf("expected cache update for current previewComputeMsg; got cacheForID=%q cacheLen=%d", m.previewCacheForID, len(m.previewCache))
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
        // Ensure we render "Resizing" only once (centered), not repeated on every row.
        if n := strings.Count(out, "Resizing"); n != 1 {
                t.Fatalf("expected resizing overlay to contain 'Resizing' exactly once; got %d occurrences", n)
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
