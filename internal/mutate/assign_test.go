package mutate

import (
        "strings"
        "testing"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func strPtr(s string) *string { return &s }

func TestSetAssignedActor_ClearsAssignment_RequiresCanEditItem(t *testing.T) {
        db := &store.DB{
                Actors: []model.Actor{
                        {ID: "act-owner", Kind: model.ActorKindHuman, Name: "Owner", UserID: strPtr("act-owner")},
                        {ID: "act-other", Kind: model.ActorKindHuman, Name: "Other", UserID: strPtr("act-other")},
                        {ID: "act-third", Kind: model.ActorKindHuman, Name: "Third", UserID: strPtr("act-third")},
                },
                Items: []model.Item{
                        {
                                ID:              "item-1",
                                OwnerActorID:    "act-owner",
                                AssignedActorID: strPtr("act-other"),
                        },
                },
        }

        // Not owner => permission error.
        if _, err := SetAssignedActor(db, "act-third", "item-1", nil, AssignOpts{}); err == nil {
                t.Fatalf("expected error")
        }

        // When assigned to another human, only the assignee can edit (owner is locked out).
        if _, err := SetAssignedActor(db, "act-owner", "item-1", nil, AssignOpts{}); err == nil {
                t.Fatalf("expected error")
        }

        res, err := SetAssignedActor(db, "act-other", "item-1", nil, AssignOpts{})
        if err != nil {
                t.Fatalf("SetAssignedActor error: %v", err)
        }
        if !res.Changed {
                t.Fatalf("expected changed=true")
        }
        if res.Item.AssignedActorID != nil {
                t.Fatalf("expected assigned cleared; got %v", *res.Item.AssignedActorID)
        }
}

func TestSetAssignedActor_SelfAssignAgent_CanClaimUnassignedSameHuman(t *testing.T) {
        db := &store.DB{
                Actors: []model.Actor{
                        {ID: "act-human", Kind: model.ActorKindHuman, Name: "Human", UserID: strPtr("act-human")},
                        {ID: "act-agent", Kind: model.ActorKindAgent, Name: "Agent", UserID: strPtr("act-human")},
                },
                Items: []model.Item{
                        {
                                ID:           "item-1",
                                OwnerActorID: "act-human",
                                // Unassigned
                        },
                },
        }

        res, err := SetAssignedActor(db, "act-agent", "item-1", strPtr("act-agent"), AssignOpts{TakeAssigned: true})
        if err != nil {
                t.Fatalf("SetAssignedActor error: %v", err)
        }
        if !res.Changed {
                t.Fatalf("expected changed=true")
        }
        if res.Item.AssignedActorID == nil || strings.TrimSpace(*res.Item.AssignedActorID) != "act-agent" {
                t.Fatalf("expected assigned to agent; got %+v", res.Item.AssignedActorID)
        }
        if strings.TrimSpace(res.Item.OwnerActorID) != "act-agent" {
                t.Fatalf("expected owner transferred to agent; got %q", res.Item.OwnerActorID)
        }
        if res.Item.OwnerDelegatedAt != nil || res.Item.OwnerDelegatedFrom != nil {
                t.Fatalf("expected delegation cleared when claiming; got from=%v at=%v", res.Item.OwnerDelegatedFrom, res.Item.OwnerDelegatedAt)
        }
}

func TestSetAssignedActor_TakeAssignedRequiredForClaim(t *testing.T) {
        db := &store.DB{
                Actors: []model.Actor{
                        {ID: "act-owner", Kind: model.ActorKindHuman, Name: "Owner", UserID: strPtr("act-owner")},
                        {ID: "act-agent1", Kind: model.ActorKindAgent, Name: "A1", UserID: strPtr("act-owner")},
                        {ID: "act-agent2", Kind: model.ActorKindAgent, Name: "A2", UserID: strPtr("act-owner")},
                },
                Items: []model.Item{
                        {
                                ID:              "item-1",
                                OwnerActorID:    "act-owner",
                                AssignedActorID: strPtr("act-agent1"),
                        },
                },
        }

        if _, err := SetAssignedActor(db, "act-agent2", "item-1", strPtr("act-agent2"), AssignOpts{TakeAssigned: false}); err == nil {
                t.Fatalf("expected error")
        } else if err != ErrTakeAssignedRequired {
                t.Fatalf("expected ErrTakeAssignedRequired; got %v", err)
        }
}
