package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/lipgloss"
        xansi "github.com/charmbracelet/x/ansi"
        "github.com/muesli/termenv"
)

func TestViewItem_IsLeftAlignedWithOuterMargin(t *testing.T) {
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
                        Description:  "desc",
                        StatusID:     "todo",
                        OwnerActorID: "act-test",
                        CreatedBy:    "act-test",
                        CreatedAt:    time.Now().UTC(),
                        UpdatedAt:    time.Now().UTC(),
                }},
        }

        m := newAppModel(t.TempDir(), db)
        m.view = viewItem
        m.modal = modalNone
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.openItemID = "item-a"
        m.width = 120
        m.height = 30

        out := m.viewItem()
        lines := strings.Split(out, "\n")
        if len(lines) <= topPadLines {
                t.Fatalf("expected output to include top padding + content; got %d lines", len(lines))
        }

        headerLine := stripSGR(lines[topPadLines])
        idx := strings.Index(headerLine, m.breadcrumbText())
        if idx < 0 {
                t.Fatalf("expected breadcrumb row to contain breadcrumb; got: %q", headerLine)
        }
        if idx != splitOuterMargin {
                t.Fatalf("expected breadcrumb to start at column=%d (outer margin), got %d (line=%q)", splitOuterMargin, idx, headerLine)
        }
}

func TestRenderItemDetail_RespectsWidth(t *testing.T) {
        db := &store.DB{
                CurrentActorID: "act-test",
                Items: []model.Item{
                        {
                                ID:           "item-a",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "h",
                                Title:        "Title",
                                Description:  strings.Repeat("X", 500), // force long line
                                StatusID:     "todo",
                                OwnerActorID: "act-test",
                                CreatedBy:    "act-test",
                                CreatedAt:    time.Now().UTC(),
                                UpdatedAt:    time.Now().UTC(),
                        },
                },
                Actors: []model.Actor{
                        {ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"},
                },
        }

        outline := model.Outline{
                ID:        "out-a",
                ProjectID: "proj-a",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "TODO", IsEndState: false},
                        {ID: "done", Label: "DONE", IsEndState: true},
                },
        }

        width := 40
        height := 20

        s := renderItemDetail(db, outline, db.Items[0], width, height, true, nil)
        for _, ln := range strings.Split(s, "\n") {
                if w := xansi.StringWidth(ln); w > width {
                        t.Fatalf("expected no line wider than %d cols; got %d: %q", width, w, ln)
                }
        }
}

func TestRenderItemDetail_EndStateTitle_IsStruck(t *testing.T) {
        old := lipgloss.ColorProfile()
        lipgloss.SetColorProfile(termenv.ANSI256)
        t.Cleanup(func() { lipgloss.SetColorProfile(old) })

        db := &store.DB{
                CurrentActorID: "act-test",
                Items: []model.Item{
                        {
                                ID:           "item-a",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "h",
                                Title:        "Title",
                                Description:  "desc",
                                StatusID:     "done",
                                OwnerActorID: "act-test",
                                CreatedBy:    "act-test",
                                CreatedAt:    time.Now().UTC(),
                                UpdatedAt:    time.Now().UTC(),
                        },
                },
                Actors: []model.Actor{
                        {ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"},
                },
        }

        outline := model.Outline{
                ID:        "out-a",
                ProjectID: "proj-a",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "TODO", IsEndState: false},
                        {ID: "done", Label: "DONE", IsEndState: true},
                },
        }

        s := renderItemDetail(db, outline, db.Items[0], 60, 20, true, nil)
        if !strings.Contains(s, ";9m") && !strings.Contains(s, "[9m") {
                t.Fatalf("expected strikethrough escape code in end-state detail; got: %q", s)
        }
}

func TestRenderItemDetail_ShowsHistoryForItem(t *testing.T) {
        db := &store.DB{
                CurrentActorID: "act-a",
                Actors: []model.Actor{
                        {ID: "act-a", Kind: model.ActorKindHuman, Name: "Alice"},
                },
                Items: []model.Item{
                        {
                                ID:           "item-a",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "h",
                                Title:        "Title",
                                Description:  "desc",
                                StatusID:     "todo",
                                OwnerActorID: "act-a",
                                CreatedBy:    "act-a",
                                CreatedAt:    time.Now().UTC(),
                                UpdatedAt:    time.Now().UTC(),
                        },
                },
        }

        outline := model.Outline{
                ID:        "out-a",
                ProjectID: "proj-a",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "TODO", IsEndState: false},
                },
        }

        now := time.Now().UTC()
        events := []model.Event{
                {ID: "evt-1", TS: now.Add(-3 * time.Hour), ActorID: "act-a", Type: "item.create", EntityID: "item-a", Payload: map[string]any{"id": "item-a"}},
                {ID: "evt-2", TS: now.Add(-2 * time.Hour), ActorID: "act-a", Type: "item.set_title", EntityID: "item-a", Payload: map[string]any{"title": "New title"}},
                // Not for this item:
                {ID: "evt-3", TS: now.Add(-1 * time.Hour), ActorID: "act-a", Type: "item.set_status", EntityID: "item-b", Payload: map[string]any{"status": "done"}},
        }

        s := renderItemDetail(db, outline, db.Items[0], 80, 30, true, events)
        if !strings.Contains(s, "History") {
                t.Fatalf("expected history section; got: %q", s)
        }
        if !strings.Contains(s, "set title:") {
                t.Fatalf("expected title-change summary in history; got: %q", s)
        }
        if !strings.Contains(s, "Alice") {
                t.Fatalf("expected actor name to render in history; got: %q", s)
        }
}
