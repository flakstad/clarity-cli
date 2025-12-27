package mutate

import (
        "testing"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestSetItemArchived(t *testing.T) {
        db := &store.DB{
                Actors: []model.Actor{
                        {ID: "act-owner", Kind: model.ActorKindHuman, Name: "Owner", UserID: strPtr("act-owner")},
                        {ID: "act-other", Kind: model.ActorKindHuman, Name: "Other", UserID: strPtr("act-other")},
                },
                Items: []model.Item{
                        {
                                ID:           "item-1",
                                OwnerActorID: "act-owner",
                                Archived:     false,
                        },
                },
        }

        if _, err := SetItemArchived(db, "act-other", "item-1", true); err == nil {
                t.Fatalf("expected error")
        }

        res, err := SetItemArchived(db, "act-owner", "item-1", true)
        if err != nil {
                t.Fatalf("SetItemArchived error: %v", err)
        }
        if !res.Changed {
                t.Fatalf("expected changed=true")
        }
        if !res.Item.Archived {
                t.Fatalf("expected archived=true")
        }

        // No-op
        res2, err := SetItemArchived(db, "act-owner", "item-1", true)
        if err != nil {
                t.Fatalf("SetItemArchived no-op error: %v", err)
        }
        if res2.Changed {
                t.Fatalf("expected changed=false")
        }
}
