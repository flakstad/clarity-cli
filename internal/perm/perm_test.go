package perm

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestCanEditItem_AssignmentLocksToOtherHuman(t *testing.T) {
        now := time.Now().UTC()

        h1 := "act-human-1"
        h2 := "act-human-2"
        a1 := "act-agent-1"
        a2 := "act-agent-2"

        db := &store.DB{
                Actors: []model.Actor{
                        {ID: h1, Kind: model.ActorKindHuman, Name: "h1"},
                        {ID: h2, Kind: model.ActorKindHuman, Name: "h2"},
                        {ID: a1, Kind: model.ActorKindAgent, Name: "a1", UserID: &h1},
                        {ID: a2, Kind: model.ActorKindAgent, Name: "a2", UserID: &h2},
                },
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj",
                        OutlineID:    "out",
                        Title:        "A",
                        OwnerActorID: h1, // owned by h1
                        CreatedBy:    h1,
                        CreatedAt:    now,
                        UpdatedAt:    now,
                }},
        }
        it := &db.Items[0]

        // Assigned to other human -> h1 should be blocked even though they're owner.
        it.AssignedActorID = &h2
        if CanEditItem(db, h1, it) {
                t.Fatalf("expected owner to be blocked when assigned to other human")
        }

        // Assigned to other human's agent -> h1 should also be blocked.
        it.AssignedActorID = &a2
        if CanEditItem(db, h1, it) {
                t.Fatalf("expected owner to be blocked when assigned to other human's agent")
        }

        // Assigned to own agent -> allowed.
        it.AssignedActorID = &a1
        if !CanEditItem(db, h1, it) {
                t.Fatalf("expected human to be able to edit when assigned to their own agent")
        }
}

func TestCanEditItem_HumanCanEditIfAssignedToThemEvenIfNotOwner(t *testing.T) {
        now := time.Now().UTC()

        h1 := "act-human-1"
        h2 := "act-human-2"

        db := &store.DB{
                Actors: []model.Actor{
                        {ID: h1, Kind: model.ActorKindHuman, Name: "h1"},
                        {ID: h2, Kind: model.ActorKindHuman, Name: "h2"},
                },
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj",
                        OutlineID:    "out",
                        Title:        "A",
                        OwnerActorID: h2, // owned by someone else
                        AssignedActorID: func() *string {
                                tmp := h1
                                return &tmp
                        }(),
                        CreatedBy: h2,
                        CreatedAt: now,
                        UpdatedAt: now,
                }},
        }
        it := &db.Items[0]

        if !CanEditItem(db, h1, it) {
                t.Fatalf("expected human to be able to edit item assigned to them even if not owner")
        }
}

func TestCanEditItem_HumanCanEditItemsOwnedByTheirAgents(t *testing.T) {
        now := time.Now().UTC()

        h1 := "act-human-1"
        a1 := "act-agent-1"

        db := &store.DB{
                Actors: []model.Actor{
                        {ID: h1, Kind: model.ActorKindHuman, Name: "h1"},
                        {ID: a1, Kind: model.ActorKindAgent, Name: "a1", UserID: &h1},
                },
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj",
                        OutlineID:    "out",
                        Title:        "A",
                        OwnerActorID: a1, // owned by agent
                        CreatedBy:    h1,
                        CreatedAt:    now,
                        UpdatedAt:    now,
                }},
        }
        it := &db.Items[0]

        if !CanEditItem(db, h1, it) {
                t.Fatalf("expected human to be able to edit item owned by their agent")
        }
}

func TestCanEditItem_HumanCanEditItemsAssignedToTheirAgentsEvenIfNotOwner(t *testing.T) {
        now := time.Now().UTC()

        h1 := "act-human-1"
        h2 := "act-human-2"
        a1 := "act-agent-1"

        db := &store.DB{
                Actors: []model.Actor{
                        {ID: h1, Kind: model.ActorKindHuman, Name: "h1"},
                        {ID: h2, Kind: model.ActorKindHuman, Name: "h2"},
                        {ID: a1, Kind: model.ActorKindAgent, Name: "a1", UserID: &h1},
                },
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj",
                        OutlineID:    "out",
                        Title:        "A",
                        OwnerActorID: h2, // owned by someone else
                        AssignedActorID: func() *string {
                                tmp := a1
                                return &tmp
                        }(),
                        CreatedBy: h2,
                        CreatedAt: now,
                        UpdatedAt: now,
                }},
        }
        it := &db.Items[0]

        if !CanEditItem(db, h1, it) {
                t.Fatalf("expected human to be able to edit item assigned to their agent even if not owner")
        }
}
