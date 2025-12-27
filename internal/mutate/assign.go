package mutate

import (
        "errors"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/perm"
        "clarity-cli/internal/store"
)

var ErrTakeAssignedRequired = errors.New("item is already assigned; pass --take-assigned to take it anyway")

type AssignOpts struct {
        // TakeAssigned is only relevant when assigning the item to the current actor (claim behavior).
        // If false, attempting to take an item already assigned to another actor returns ErrTakeAssignedRequired.
        TakeAssigned bool
}

type AssignResult struct {
        Item         *model.Item
        Changed      bool
        EventPayload map[string]any
}

// SetAssignedActor sets (or clears) the assigned actor for an item, enforcing permissions via internal/perm.
// It also applies Clarity ownership-transfer rules when assigning (but not when clearing).
//
// Callers are responsible for saving db and appending the item.set_assign event.
func SetAssignedActor(db *store.DB, actorID, itemID string, assignedActorID *string, opts AssignOpts) (AssignResult, error) {
        itemID = strings.TrimSpace(itemID)
        actorID = strings.TrimSpace(actorID)
        if db == nil || itemID == "" || actorID == "" {
                return AssignResult{}, nil
        }

        it, ok := db.FindItem(itemID)
        if !ok {
                return AssignResult{}, NotFoundError{Kind: "item", ID: itemID}
        }

        next := ""
        if assignedActorID != nil {
                next = strings.TrimSpace(*assignedActorID)
        }

        // Clear assignment (owner-only; no ownership transfer).
        if next == "" {
                if !perm.CanEditItem(db, actorID, it) {
                        return AssignResult{}, OwnerOnlyError{ActorID: actorID, OwnerActorID: it.OwnerActorID, ItemID: itemID}
                }
                if it.AssignedActorID == nil || strings.TrimSpace(*it.AssignedActorID) == "" {
                        return AssignResult{Item: it, Changed: false}, nil
                }
                it.AssignedActorID = nil
                return AssignResult{
                        Item:         it,
                        Changed:      true,
                        EventPayload: map[string]any{"assignedActorId": nil},
                }, nil
        }

        if _, ok := db.FindActor(next); !ok {
                return AssignResult{}, NotFoundError{Kind: "actor", ID: next}
        }

        // No-op: already assigned to target.
        if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) == next {
                return AssignResult{Item: it, Changed: false}, nil
        }

        // Claim coordination: if taking for ourselves, don't steal unless requested.
        if next == actorID && it.AssignedActorID != nil {
                curAssigned := strings.TrimSpace(*it.AssignedActorID)
                if curAssigned != "" && curAssigned != actorID && !opts.TakeAssigned {
                        return AssignResult{}, ErrTakeAssignedRequired
                }
        }

        // Special case: allow an agent to self-assign an unassigned item belonging to the same human user,
        // even if they aren't the current owner. This "claims" the item (transfers ownership to the agent).
        isUnassigned := it.AssignedActorID == nil
        isSelfAssign := next == actorID
        if isUnassigned && isSelfAssign && !perm.CanEditItem(db, actorID, it) {
                curHuman, ok1 := db.HumanUserIDForActor(actorID)
                ownerHuman, ok2 := db.HumanUserIDForActor(it.OwnerActorID)
                if ok1 && ok2 && strings.TrimSpace(curHuman) != "" && curHuman == ownerHuman {
                        tmp := actorID
                        it.AssignedActorID = &tmp
                        it.OwnerActorID = actorID
                        it.OwnerDelegatedFrom = nil
                        it.OwnerDelegatedAt = nil
                        return AssignResult{
                                Item:         it,
                                Changed:      true,
                                EventPayload: map[string]any{"assignedActorId": it.AssignedActorID},
                        }, nil
                }
                return AssignResult{}, OwnerOnlyError{ActorID: actorID, OwnerActorID: it.OwnerActorID, ItemID: itemID}
        }

        // Normal permission check.
        if !perm.CanEditItem(db, actorID, it) {
                return AssignResult{}, OwnerOnlyError{ActorID: actorID, OwnerActorID: it.OwnerActorID, ItemID: itemID}
        }

        // Transfer ownership when assigning to someone else (including to yourself if you're not the owner).
        if strings.TrimSpace(it.OwnerActorID) != next {
                now := time.Now().UTC()
                prev := strings.TrimSpace(it.OwnerActorID)
                if prev != "" {
                        it.OwnerDelegatedFrom = &prev
                        it.OwnerDelegatedAt = &now
                } else {
                        it.OwnerDelegatedFrom = nil
                        it.OwnerDelegatedAt = nil
                }
                it.OwnerActorID = next
        }
        tmp := next
        it.AssignedActorID = &tmp

        return AssignResult{
                Item:         it,
                Changed:      true,
                EventPayload: map[string]any{"assignedActorId": it.AssignedActorID},
        }, nil
}
