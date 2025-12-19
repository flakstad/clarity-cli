package cli

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

func canEditTask(db *store.DB, actorID string, t *model.Item) bool {
        if t.OwnerActorID == actorID {
                return true
        }

        // Human override: a human user can edit items owned by their own agents.
        if actorHuman, ok := db.HumanUserIDForActor(actorID); ok {
                if ownerHuman, ok := db.HumanUserIDForActor(t.OwnerActorID); ok && actorHuman == ownerHuman {
                        owner, _ := db.FindActor(t.OwnerActorID)
                        if owner != nil && owner.Kind == model.ActorKindAgent {
                                return true
                        }
                }
        }

        if t.OwnerDelegatedFrom == nil || t.OwnerDelegatedAt == nil {
                return false
        }
        if *t.OwnerDelegatedFrom != actorID {
                return false
        }
        return time.Now().UTC().Before(t.OwnerDelegatedAt.Add(assignGraceDuration()))
}
