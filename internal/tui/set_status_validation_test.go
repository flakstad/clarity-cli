package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestSetStatusForItem_BlocksCompletionWhenHasIncompleteChildren(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 21, 0, 0, 0, 0, time.UTC)

        actorID := "act-test"
        projectID := "proj-test"
        outlineID := "out-test"
        parentID := "item-parent"
        childID := "item-child"

        db := &store.DB{
                Version:          1,
                CurrentActorID:   actorID,
                CurrentProjectID: projectID,
                NextIDs:          map[string]int{},
                Actors:           []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "Test"}},
                Projects:         []model.Project{{ID: projectID, Name: "P", CreatedBy: actorID, CreatedAt: now}},
                Outlines:         []model.Outline{{ID: outlineID, ProjectID: projectID, Name: nil, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now}},
                Items: []model.Item{
                        {
                                ID:           parentID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                ParentID:     nil,
                                Rank:         "h",
                                Title:        "Parent",
                                StatusID:     "todo",
                                Archived:     false,
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                        {
                                ID:           childID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                ParentID:     ptr(parentID),
                                Rank:         "h0",
                                Title:        "Child",
                                StatusID:     "todo",
                                Archived:     false,
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                },
                Deps:     []model.Dependency{},
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        s := store.Store{Dir: dir}
        if err := s.Save(db); err != nil {
                t.Fatalf("save seed db: %v", err)
        }

        m := newAppModel(dir, db)
        if err := m.setStatusForItem(parentID, "done"); err == nil {
                t.Fatalf("expected error, got nil")
        } else if !strings.Contains(err.Error(), "incomplete children") {
                t.Fatalf("expected error to mention incomplete children; got %q", err.Error())
        }

        db2, err := s.Load()
        if err != nil {
                t.Fatalf("reload db: %v", err)
        }
        p2, ok := db2.FindItem(parentID)
        if !ok {
                t.Fatalf("expected parent item to exist after reload")
        }
        if got := strings.TrimSpace(p2.StatusID); got != "todo" {
                t.Fatalf("expected parent status to remain todo; got %q", got)
        }
}

func TestSetStatusForItem_DoesNotBlockCompletionWhenChildrenHaveNoStatus(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 21, 0, 0, 0, 0, time.UTC)

        actorID := "act-test"
        projectID := "proj-test"
        outlineID := "out-test"
        parentID := "item-parent"
        childID := "item-child"

        db := &store.DB{
                Version:          1,
                CurrentActorID:   actorID,
                CurrentProjectID: projectID,
                NextIDs:          map[string]int{},
                Actors:           []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "Test"}},
                Projects:         []model.Project{{ID: projectID, Name: "P", CreatedBy: actorID, CreatedAt: now}},
                Outlines:         []model.Outline{{ID: outlineID, ProjectID: projectID, Name: nil, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now}},
                Items: []model.Item{
                        {
                                ID:           parentID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                ParentID:     nil,
                                Rank:         "h",
                                Title:        "Parent",
                                StatusID:     "todo",
                                Archived:     false,
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                        {
                                ID:           childID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                ParentID:     ptr(parentID),
                                Rank:         "h0",
                                Title:        "Child (no status)",
                                StatusID:     "",
                                Archived:     false,
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                },
                Deps:     []model.Dependency{},
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        s := store.Store{Dir: dir}
        if err := s.Save(db); err != nil {
                t.Fatalf("save seed db: %v", err)
        }

        m := newAppModel(dir, db)
        if err := m.setStatusForItem(parentID, "done"); err != nil {
                t.Fatalf("expected no error, got %v", err)
        }

        db2, err := s.Load()
        if err != nil {
                t.Fatalf("reload db: %v", err)
        }
        p2, ok := db2.FindItem(parentID)
        if !ok {
                t.Fatalf("expected parent item to exist after reload")
        }
        if got := strings.TrimSpace(p2.StatusID); got != "done" {
                t.Fatalf("expected parent status to be done; got %q", got)
        }
}

func TestSetStatusForItem_BlocksCompletionWhenBlockedByUndoneDeps(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 21, 0, 0, 0, 0, time.UTC)

        actorID := "act-test"
        projectID := "proj-test"
        outlineID := "out-test"
        itemID := "item-a"
        depID := "item-b"

        db := &store.DB{
                Version:          1,
                CurrentActorID:   actorID,
                CurrentProjectID: projectID,
                NextIDs:          map[string]int{},
                Actors:           []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "Test"}},
                Projects:         []model.Project{{ID: projectID, Name: "P", CreatedBy: actorID, CreatedAt: now}},
                Outlines:         []model.Outline{{ID: outlineID, ProjectID: projectID, Name: nil, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now}},
                Items: []model.Item{
                        {
                                ID:           itemID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                Rank:         "h",
                                Title:        "A",
                                StatusID:     "todo",
                                Archived:     false,
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                        {
                                ID:           depID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                Rank:         "h0",
                                Title:        "B",
                                StatusID:     "todo",
                                Archived:     false,
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                },
                Deps: []model.Dependency{
                        {FromItemID: itemID, ToItemID: depID, Type: model.DependencyBlocks},
                },
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        s := store.Store{Dir: dir}
        if err := s.Save(db); err != nil {
                t.Fatalf("save seed db: %v", err)
        }

        m := newAppModel(dir, db)
        if err := m.setStatusForItem(itemID, "done"); err == nil {
                t.Fatalf("expected error, got nil")
        } else if !strings.Contains(err.Error(), "dependencies") {
                t.Fatalf("expected error to mention dependencies; got %q", err.Error())
        }
}
