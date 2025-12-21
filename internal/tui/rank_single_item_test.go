package tui

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestReorderItem_DuplicateRanks_OnlyUpdatesMovedItem(t *testing.T) {
        dir := t.TempDir()
        now := time.Now().UTC()

        db := &store.DB{
                CurrentActorID: "act-a",
                Actors:         []model.Actor{{ID: "act-a", Kind: model.ActorKindHuman, Name: "tester"}},
                Projects:       []model.Project{{ID: "proj-a", Name: "P", CreatedBy: "act-a", CreatedAt: now}},
                Outlines:       []model.Outline{{ID: "out-a", ProjectID: "proj-a", StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: "act-a", CreatedAt: now}},
                Items: []model.Item{
                        {ID: "a", ProjectID: "proj-a", OutlineID: "out-a", Rank: "h", Title: "A", OwnerActorID: "act-a", CreatedBy: "act-a", CreatedAt: now, UpdatedAt: now},
                        {ID: "b", ProjectID: "proj-a", OutlineID: "out-a", Rank: "h", Title: "B", OwnerActorID: "act-a", CreatedBy: "act-a", CreatedAt: now, UpdatedAt: now},
                        {ID: "c", ProjectID: "proj-a", OutlineID: "out-a", Rank: "t", Title: "C", OwnerActorID: "act-a", CreatedBy: "act-a", CreatedAt: now, UpdatedAt: now},
                },
        }
        s := store.Store{Dir: dir}
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.db = db

        a0, _ := m.db.FindItem("a")
        b0, _ := m.db.FindItem("b")
        c0, _ := m.db.FindItem("c")
        aRank0, bRank0, cRank0 := a0.Rank, b0.Rank, c0.Rank

        // Move B down (after A): this used to sometimes hit RankBetween requires a < b when ranks collide.
        if err := m.reorderItem(b0, "a", ""); err != nil {
                t.Fatalf("expected reorder to succeed; got err: %v", err)
        }

        a1, _ := m.db.FindItem("a")
        b1, _ := m.db.FindItem("b")
        c1, _ := m.db.FindItem("c")

        if a1.Rank != aRank0 {
                t.Fatalf("expected non-moved sibling rank unchanged; a: %q -> %q", aRank0, a1.Rank)
        }
        if c1.Rank != cRank0 {
                t.Fatalf("expected non-moved sibling rank unchanged; c: %q -> %q", cRank0, c1.Rank)
        }
        if b1.Rank == bRank0 {
                t.Fatalf("expected moved item rank to change; b still %q", b1.Rank)
        }
}

func TestOutdentSelected_DuplicateDestRanks_OnlyUpdatesMovedItem(t *testing.T) {
        dir := t.TempDir()
        now := time.Now().UTC()

        act := "act-a"
        proj := "proj-a"
        out := "out-a"

        parentID := "p"
        sibID := "s"
        childID := "c"

        db := &store.DB{
                CurrentActorID: act,
                Actors:         []model.Actor{{ID: act, Kind: model.ActorKindHuman, Name: "tester"}},
                Projects:       []model.Project{{ID: proj, Name: "P", CreatedBy: act, CreatedAt: now}},
                Outlines:       []model.Outline{{ID: out, ProjectID: proj, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: act, CreatedAt: now}},
                Items: []model.Item{
                        {ID: parentID, ProjectID: proj, OutlineID: out, Rank: "h", Title: "P", OwnerActorID: act, CreatedBy: act, CreatedAt: now, UpdatedAt: now},
                        {ID: sibID, ProjectID: proj, OutlineID: out, Rank: "h", Title: "S", OwnerActorID: act, CreatedBy: act, CreatedAt: now, UpdatedAt: now},
                        {ID: childID, ProjectID: proj, OutlineID: out, ParentID: &parentID, Rank: "h", Title: "C", OwnerActorID: act, CreatedBy: act, CreatedAt: now, UpdatedAt: now},
                },
        }
        s := store.Store{Dir: dir}
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.view = viewOutline
        m.selectedProjectID = proj
        m.selectedOutlineID = out
        m.selectedOutline = &db.Outlines[0]
        m.collapsed = map[string]bool{parentID: false}
        m.collapseInitialized = true
        m.refreshItems(db.Outlines[0])
        selectListItemByID(&m.itemsList, childID)

        // Snapshot non-moved sibling ranks.
        p0, _ := db.FindItem(parentID)
        s0, _ := db.FindItem(sibID)
        pRank0, sRank0 := p0.Rank, s0.Rank

        if err := m.outdentSelected(); err != nil {
                t.Fatalf("expected outdent to succeed; got err: %v", err)
        }

        db2, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        p1, _ := db2.FindItem(parentID)
        s1, _ := db2.FindItem(sibID)
        c1, _ := db2.FindItem(childID)

        if p1.Rank != pRank0 {
                t.Fatalf("expected non-moved sibling rank unchanged; parent: %q -> %q", pRank0, p1.Rank)
        }
        if s1.Rank != sRank0 {
                t.Fatalf("expected non-moved sibling rank unchanged; sibling: %q -> %q", sRank0, s1.Rank)
        }
        if c1.ParentID != nil && *c1.ParentID != "" {
                t.Fatalf("expected child to be outdented to root; got parentID=%v", c1.ParentID)
        }
}
