package perm

import (
        "os"
        "strconv"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func assignGraceDuration() time.Duration {
        // Default 1 hour. Override with CLARITY_ASSIGN_GRACE_SECONDS.
        if s := os.Getenv("CLARITY_ASSIGN_GRACE_SECONDS"); s != "" {
                if n, err := strconv.Atoi(s); err == nil && n >= 0 {
                        return time.Duration(n) * time.Second
                }
        }
        return 1 * time.Hour
}

// CanEditItem enforces Clarity ownership rules for mutating an item.
//
// Rules:
// - The owner can always edit.
// - A human can edit items owned by their own agent(s).
// - A delegated-from actor can edit until OwnerDelegatedAt + grace duration.
func CanEditItem(db *store.DB, actorID string, it *model.Item) bool {
        if db == nil || it == nil {
                return false
        }
        if it.OwnerActorID == actorID {
                return true
        }

        // Human override: a human user can edit items owned by their own agents.
        if actorHuman, ok := db.HumanUserIDForActor(actorID); ok {
                if ownerHuman, ok := db.HumanUserIDForActor(it.OwnerActorID); ok && actorHuman == ownerHuman {
                        owner, _ := db.FindActor(it.OwnerActorID)
                        if owner != nil && owner.Kind == model.ActorKindAgent {
                                return true
                        }
                }
        }

        if it.OwnerDelegatedFrom == nil || it.OwnerDelegatedAt == nil {
                return false
        }
        if *it.OwnerDelegatedFrom != actorID {
                return false
        }
        return time.Now().UTC().Before(it.OwnerDelegatedAt.Add(assignGraceDuration()))
}
