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

        s := renderItemDetail(db, outline, db.Items[0], width, height, true)
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

        s := renderItemDetail(db, outline, db.Items[0], 60, 20, true)
        if !strings.Contains(s, ";9m") && !strings.Contains(s, "[9m") {
                t.Fatalf("expected strikethrough escape code in end-state detail; got: %q", s)
        }
}
