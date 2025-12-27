package mutate

import (
        "testing"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestSetItemStatus_ValidatesAgainstOutline(t *testing.T) {
        db := &store.DB{
                Actors: []model.Actor{
                        {ID: "act-human", Kind: model.ActorKindHuman, Name: "Human", UserID: strPtr("act-human")},
                },
                Outlines: []model.Outline{
                        {
                                ID:        "out-1",
                                ProjectID: "proj-1",
                                StatusDefs: []model.OutlineStatusDef{
                                        {ID: "todo"},
                                        {ID: "done"},
                                },
                        },
                },
                Items: []model.Item{
                        {
                                ID:           "item-1",
                                ProjectID:    "proj-1",
                                OutlineID:    "out-1",
                                StatusID:     "todo",
                                OwnerActorID: "act-human",
                        },
                },
        }

        if _, err := SetItemStatus(db, "act-human", "item-1", "nope"); err == nil {
                t.Fatalf("expected error")
        } else if err != ErrInvalidStatus {
                t.Fatalf("expected ErrInvalidStatus; got %v", err)
        }

        res, err := SetItemStatus(db, "act-human", "item-1", "done")
        if err != nil {
                t.Fatalf("SetItemStatus error: %v", err)
        }
        if !res.Changed {
                t.Fatalf("expected changed=true")
        }
        if got := res.Item.StatusID; got != "done" {
                t.Fatalf("expected status done; got %q", got)
        }

        // Allow clearing status.
        res2, err := SetItemStatus(db, "act-human", "item-1", "")
        if err != nil {
                t.Fatalf("SetItemStatus clear error: %v", err)
        }
        if !res2.Changed {
                t.Fatalf("expected changed=true for clear")
        }
        if got := res2.Item.StatusID; got != "" {
                t.Fatalf("expected status cleared; got %q", got)
        }
}
