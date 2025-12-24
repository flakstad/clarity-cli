package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestRenderItemDetailInteractive_SmallHeight_ShowsScrollIndicators(t *testing.T) {
        db := &store.DB{
                Actors: []model.Actor{
                        {ID: "act-1", Kind: model.ActorKindHuman, Name: "A"},
                },
                Items: []model.Item{},
        }
        outline := model.Outline{
                ID:        "out-1",
                ProjectID: "proj-1",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "Todo", IsEndState: false},
                },
                CreatedBy: "act-1",
                CreatedAt: time.Now(),
        }

        longDesc := strings.Repeat("line\n", 60)
        it := model.Item{
                ID:           "item-1",
                ProjectID:    "proj-1",
                OutlineID:    "out-1",
                Title:        "Title",
                Description:  longDesc,
                StatusID:     "todo",
                OwnerActorID: "act-1",
                CreatedBy:    "act-1",
                CreatedAt:    time.Now(),
                UpdatedAt:    time.Now(),
        }

        // Very small height: should not just cut content silently; should show a "↓ more" indicator.
        out0 := renderItemDetailInteractive(db, outline, it, 42, 10, itemFocusTitle, nil, 0, 0, 0)
        plain0 := stripANSIEscapes(out0)
        if !strings.Contains(plain0, "↓") {
                t.Fatalf("expected bottom scroll indicator, got:\n%s", plain0)
        }

        // After scrolling down, we should see a top indicator too.
        out1 := renderItemDetailInteractive(db, outline, it, 42, 10, itemFocusTitle, nil, 0, 0, 10)
        plain1 := stripANSIEscapes(out1)
        if !strings.Contains(plain1, "↑") {
                t.Fatalf("expected top scroll indicator after scrolling, got:\n%s", plain1)
        }
}
