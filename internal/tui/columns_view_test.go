package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestViewOutline_ColumnView_RendersStatusColumns(t *testing.T) {
        now := time.Now().UTC()
        db := &store.DB{
                CurrentActorID: "act-test",
                Actors:         []model.Actor{{ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"}},
                Projects:       []model.Project{{ID: "proj-a", Name: "Project", CreatedBy: "act-test", CreatedAt: now}},
                Outlines: []model.Outline{{
                        ID:        "out-a",
                        ProjectID: "proj-a",
                        StatusDefs: []model.OutlineStatusDef{
                                {ID: "todo", Label: "TODO"},
                                {ID: "doing", Label: "DOING"},
                                {ID: "done", Label: "DONE", IsEndState: true},
                        },
                        CreatedBy: "act-test",
                        CreatedAt: now,
                }},
                Items: []model.Item{
                        {ID: "item-a", ProjectID: "proj-a", OutlineID: "out-a", Rank: "h", Title: "A", StatusID: "todo", OwnerActorID: "act-test", CreatedBy: "act-test", CreatedAt: now, UpdatedAt: now},
                        {ID: "item-b", ProjectID: "proj-a", OutlineID: "out-a", Rank: "i", Title: "B", StatusID: "doing", OwnerActorID: "act-test", CreatedBy: "act-test", CreatedAt: now, UpdatedAt: now},
                        {ID: "item-c", ProjectID: "proj-a", OutlineID: "out-a", Rank: "j", Title: "C", StatusID: "", OwnerActorID: "act-test", CreatedBy: "act-test", CreatedAt: now, UpdatedAt: now},
                },
        }

        m := newAppModel(t.TempDir(), db)
        m.view = viewOutline
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.selectedOutline = &db.Outlines[0]
        m.width = 120
        m.height = 30
        m.outlineViewMode = map[string]outlineViewMode{"out-a": outlineViewModeColumns}

        out := m.viewOutline()
        if !strings.Contains(out, "(no status)") {
                t.Fatalf("expected '(no status)' column header, got: %q", out)
        }
        if !strings.Contains(out, "TODO") || !strings.Contains(out, "DOING") || !strings.Contains(out, "DONE") {
                t.Fatalf("expected status columns to render, got: %q", out)
        }
        if !strings.Contains(out, "A") || !strings.Contains(out, "B") || !strings.Contains(out, "C") {
                t.Fatalf("expected item titles to render, got: %q", out)
        }
}
