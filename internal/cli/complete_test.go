package cli

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestHasIncompleteChildren_IgnoresNoStatusChildren(t *testing.T) {
        t.Parallel()

        now := time.Date(2025, 12, 21, 0, 0, 0, 0, time.UTC)
        actorID := "act-a"
        projectID := "proj-a"
        outlineID := "out-a"
        parentID := "item-parent"
        childID := "item-child"

        db := &store.DB{
                Version:          1,
                CurrentActorID:   actorID,
                CurrentProjectID: projectID,
                NextIDs:          map[string]int{},
                Actors:           []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "A"}},
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
        }

        if got := hasIncompleteChildren(db, parentID); got {
                t.Fatalf("expected hasIncompleteChildren=false when only child has no status")
        }
}
