package tui

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestToggleCollapseAll_CyclesThreeStates(t *testing.T) {
        now := time.Now().UTC()
        db := &store.DB{
                CurrentActorID: "act-test",
                Actors:         []model.Actor{{ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"}},
                Projects:       []model.Project{{ID: "proj-a", Name: "Project", CreatedBy: "act-test", CreatedAt: now}},
                Outlines:       []model.Outline{{ID: "out-a", ProjectID: "proj-a", StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: "act-test", CreatedAt: now}},
                Items: []model.Item{
                        {ID: "item-a", ProjectID: "proj-a", OutlineID: "out-a", Rank: "h", Title: "A", OwnerActorID: "act-test", CreatedBy: "act-test", CreatedAt: now, UpdatedAt: now},
                        {ID: "item-b", ProjectID: "proj-a", OutlineID: "out-a", ParentID: strPtr2("item-a"), Rank: "i", Title: "B", OwnerActorID: "act-test", CreatedBy: "act-test", CreatedAt: now, UpdatedAt: now},
                        {ID: "item-c", ProjectID: "proj-a", OutlineID: "out-a", ParentID: strPtr2("item-b"), Rank: "j", Title: "C", OwnerActorID: "act-test", CreatedBy: "act-test", CreatedAt: now, UpdatedAt: now},
                },
        }

        m := newAppModel(t.TempDir(), db)
        m.view = viewOutline
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.selectedOutline = &db.Outlines[0]
        m.collapsed = map[string]bool{}
        m.refreshItems(db.Outlines[0])

        // State 1: all collapsed (default).
        if !m.collapsed["item-a"] || !m.collapsed["item-b"] {
                t.Fatalf("expected defaults collapsed; got item-a=%v item-b=%v", m.collapsed["item-a"], m.collapsed["item-b"])
        }

        // State 2: open first layer.
        m.toggleCollapseAll()
        if m.collapsed["item-a"] || !m.collapsed["item-b"] {
                t.Fatalf("expected first-layer; got item-a=%v item-b=%v", m.collapsed["item-a"], m.collapsed["item-b"])
        }

        // State 3: open all layers.
        m.toggleCollapseAll()
        if m.collapsed["item-a"] || m.collapsed["item-b"] {
                t.Fatalf("expected all-expanded; got item-a=%v item-b=%v", m.collapsed["item-a"], m.collapsed["item-b"])
        }

        // Back to state 1.
        m.toggleCollapseAll()
        if !m.collapsed["item-a"] || !m.collapsed["item-b"] {
                t.Fatalf("expected all-collapsed; got item-a=%v item-b=%v", m.collapsed["item-a"], m.collapsed["item-b"])
        }
}

func TestToggleCollapseSelected_CyclesThreeStates(t *testing.T) {
        now := time.Now().UTC()
        db := &store.DB{
                CurrentActorID: "act-test",
                Actors:         []model.Actor{{ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"}},
                Projects:       []model.Project{{ID: "proj-a", Name: "Project", CreatedBy: "act-test", CreatedAt: now}},
                Outlines:       []model.Outline{{ID: "out-a", ProjectID: "proj-a", StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: "act-test", CreatedAt: now}},
                Items: []model.Item{
                        {ID: "item-a", ProjectID: "proj-a", OutlineID: "out-a", Rank: "h", Title: "A", OwnerActorID: "act-test", CreatedBy: "act-test", CreatedAt: now, UpdatedAt: now},
                        {ID: "item-b", ProjectID: "proj-a", OutlineID: "out-a", ParentID: strPtr2("item-a"), Rank: "i", Title: "B", OwnerActorID: "act-test", CreatedBy: "act-test", CreatedAt: now, UpdatedAt: now},
                        {ID: "item-c", ProjectID: "proj-a", OutlineID: "out-a", ParentID: strPtr2("item-b"), Rank: "j", Title: "C", OwnerActorID: "act-test", CreatedBy: "act-test", CreatedAt: now, UpdatedAt: now},
                },
        }

        m := newAppModel(t.TempDir(), db)
        m.view = viewOutline
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.selectedOutline = &db.Outlines[0]
        m.collapsed = map[string]bool{}
        m.refreshItems(db.Outlines[0])
        selectListItemByID(&m.itemsList, "item-a")

        // State 1: collapsed.
        if !m.collapsed["item-a"] || !m.collapsed["item-b"] {
                t.Fatalf("expected defaults collapsed; got item-a=%v item-b=%v", m.collapsed["item-a"], m.collapsed["item-b"])
        }

        // State 2: open first layer.
        m.toggleCollapseSelected()
        if m.collapsed["item-a"] || !m.collapsed["item-b"] {
                t.Fatalf("expected first-layer; got item-a=%v item-b=%v", m.collapsed["item-a"], m.collapsed["item-b"])
        }

        // State 3: open all layers.
        m.toggleCollapseSelected()
        if m.collapsed["item-a"] || m.collapsed["item-b"] {
                t.Fatalf("expected all-expanded; got item-a=%v item-b=%v", m.collapsed["item-a"], m.collapsed["item-b"])
        }

        // Back to state 1.
        m.toggleCollapseSelected()
        if !m.collapsed["item-a"] || !m.collapsed["item-b"] {
                t.Fatalf("expected all-collapsed; got item-a=%v item-b=%v", m.collapsed["item-a"], m.collapsed["item-b"])
        }
}

func strPtr2(s string) *string { return &s }
