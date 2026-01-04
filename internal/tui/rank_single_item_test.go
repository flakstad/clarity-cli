package tui

import (
        "sort"
        "strings"
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
                        {ID: "c", ProjectID: "proj-a", OutlineID: "out-a", Rank: "h", Title: "C", OwnerActorID: "act-a", CreatedBy: "act-a", CreatedAt: now, UpdatedAt: now},
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

        // Move C up one slot (before B) inside a duplicate-rank block. This must always be possible
        // and must swap exactly one position in the current rendered order.
        if err := m.reorderItem(c0, "", "b"); err != nil {
                t.Fatalf("expected reorder to succeed; got err: %v", err)
        }

        db2, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        a1, _ := db2.FindItem("a")
        b1, _ := db2.FindItem("b")
        c1, _ := db2.FindItem("c")

        // A should remain at the top of the block, and C/B should swap.
        sibs := []model.Item{*a1, *b1, *c1}
        sort.Slice(sibs, func(i, j int) bool { return compareOutlineItems(sibs[i], sibs[j]) < 0 })
        got := []string{sibs[0].ID, sibs[1].ID, sibs[2].ID}
        want := []string{"a", "c", "b"}
        if strings.Join(got, ",") != strings.Join(want, ",") {
                t.Fatalf("expected sibling order %v; got %v (ranks: a=%q b=%q c=%q)", want, got, a1.Rank, b1.Rank, c1.Rank)
        }

        // A is outside the minimal rebalance window, so its rank should remain unchanged.
        if a1.Rank != aRank0 {
                t.Fatalf("expected a rank unchanged; a: %q -> %q", aRank0, a1.Rank)
        }
        // We must change at least the moved item's rank; inside a duplicate block we typically
        // also need to adjust the swapped neighbor.
        if c1.Rank == cRank0 {
                t.Fatalf("expected moved item rank to change; c still %q", c1.Rank)
        }
        if b1.Rank == bRank0 {
                t.Fatalf("expected swapped neighbor rank to change in fallback; b still %q", b1.Rank)
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
