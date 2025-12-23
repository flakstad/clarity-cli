package cli

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestItemsMove_DuplicateRanks_AllowsExactPlacement(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)

        actorID := "act-testhuman"
        projectID := "proj-test"
        outlineID := "out-test"

        db := &store.DB{
                Version:        1,
                CurrentActorID: actorID,
                NextIDs:        map[string]int{},
                Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "Test Human"}},
                Projects:       []model.Project{{ID: projectID, Name: "Test Project", CreatedBy: actorID, CreatedAt: now}},
                Outlines:       []model.Outline{{ID: outlineID, ProjectID: projectID, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now}},
                Items: []model.Item{
                        {ID: "a", ProjectID: projectID, OutlineID: outlineID, Rank: "h", Title: "A", StatusID: "todo", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                        {ID: "b", ProjectID: projectID, OutlineID: outlineID, Rank: "h", Title: "B", StatusID: "todo", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                        {ID: "c", ProjectID: projectID, OutlineID: outlineID, Rank: "h", Title: "C", StatusID: "todo", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                },
                Deps:     []model.Dependency{},
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        if err := (store.Store{Dir: dir}).Save(db); err != nil {
                t.Fatalf("seed store: %v", err)
        }

        _, stderr, err := runCLI(t, []string{"--dir", dir, "--actor", actorID, "items", "move", "c", "--before", "b"})
        if err != nil {
                t.Fatalf("items move error: %v\nstderr:\n%s", err, string(stderr))
        }

        db2, err := (store.Store{Dir: dir}).Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }

        // Verify final sibling order is a,c,b (exact placement, no jump).
        var sibs []*model.Item
        for i := range db2.Items {
                it := &db2.Items[i]
                if it.OutlineID == outlineID && it.ParentID == nil && !it.Archived {
                        sibs = append(sibs, it)
                }
        }
        store.SortItemsByRankOrder(sibs)

        got := []string{sibs[0].ID, sibs[1].ID, sibs[2].ID}
        want := []string{"a", "c", "b"}
        for i := range want {
                if got[i] != want[i] {
                        t.Fatalf("expected sibling order %v; got %v", want, got)
                }
        }
}

func TestItemsSetParent_DuplicateRanks_AllowsExactPlacement(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)

        actorID := "act-testhuman"
        projectID := "proj-test"
        outlineID := "out-test"

        parentID := "p"

        db := &store.DB{
                Version:        1,
                CurrentActorID: actorID,
                NextIDs:        map[string]int{},
                Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "Test Human"}},
                Projects:       []model.Project{{ID: projectID, Name: "Test Project", CreatedBy: actorID, CreatedAt: now}},
                Outlines:       []model.Outline{{ID: outlineID, ProjectID: projectID, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now}},
                Items: []model.Item{
                        {ID: parentID, ProjectID: projectID, OutlineID: outlineID, Rank: "h", Title: "P", StatusID: "todo", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                        {ID: "x", ProjectID: projectID, OutlineID: outlineID, ParentID: &parentID, Rank: "h", Title: "X", StatusID: "todo", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                        {ID: "y", ProjectID: projectID, OutlineID: outlineID, ParentID: &parentID, Rank: "h", Title: "Y", StatusID: "todo", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                        {ID: "z", ProjectID: projectID, OutlineID: outlineID, Rank: "h", Title: "Z", StatusID: "todo", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                },
                Deps:     []model.Dependency{},
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        if err := (store.Store{Dir: dir}).Save(db); err != nil {
                t.Fatalf("seed store: %v", err)
        }

        _, stderr, err := runCLI(t, []string{"--dir", dir, "--actor", actorID, "items", "set-parent", "z", "--parent", parentID, "--before", "y"})
        if err != nil {
                t.Fatalf("items set-parent error: %v\nstderr:\n%s", err, string(stderr))
        }

        db2, err := (store.Store{Dir: dir}).Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }

        // Verify children under P are x,z,y in that order.
        var ch []*model.Item
        for i := range db2.Items {
                it := &db2.Items[i]
                if it.ParentID != nil && *it.ParentID == parentID && !it.Archived {
                        ch = append(ch, it)
                }
        }
        store.SortItemsByRankOrder(ch)

        got := []string{ch[0].ID, ch[1].ID, ch[2].ID}
        want := []string{"x", "z", "y"}
        for i := range want {
                if got[i] != want[i] {
                        t.Fatalf("expected child order %v; got %v", want, got)
                }
        }
}
