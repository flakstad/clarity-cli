package perm

import (
        "os"
        "strconv"
        "strings"
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
// - Assignment acts as a human-level "edit lock":
//   - If an item is assigned to a different human user (including their agents), you can't edit it.
//
// - Otherwise:
//   - The owner can edit.
//   - The assignee can edit.
//   - A human can edit items owned by their own agents.
//   - A human can edit items assigned to their own agents.
//   - A delegated-from actor can edit until OwnerDelegatedAt + grace duration (when not locked).
func CanEditItem(db *store.DB, actorID string, it *model.Item) bool {
        if db == nil || it == nil {
                return false
        }
        actorID = strings.TrimSpace(actorID)
        if actorID == "" {
                return false
        }

        actorHuman, ok := db.HumanUserIDForActor(actorID)
        if !ok || strings.TrimSpace(actorHuman) == "" {
                return false
        }

        // Assignment is a human-level lock: if assigned to some other human, deny edits.
        if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) != "" {
                if assignedHuman, ok := db.HumanUserIDForActor(strings.TrimSpace(*it.AssignedActorID)); ok {
                        if strings.TrimSpace(assignedHuman) != "" && assignedHuman != actorHuman {
                                return false
                        }
                }
        }

        if it.OwnerActorID == actorID {
                return true
        }

        // Assignee can edit (agent or human).
        if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) == actorID {
                return true
        }

        // Human override: a human user can edit items owned by their own agents.
        if ownerHuman, ok := db.HumanUserIDForActor(it.OwnerActorID); ok && actorHuman == ownerHuman {
                owner, _ := db.FindActor(it.OwnerActorID)
                if owner != nil && owner.Kind == model.ActorKindAgent {
                        return true
                }
        }

        // Human override: a human can edit items assigned to their own agents.
        if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) != "" {
                assigneeID := strings.TrimSpace(*it.AssignedActorID)
                assignee, ok := db.FindActor(assigneeID)
                if ok && assignee.Kind == model.ActorKindAgent {
                        if assigneeHuman, ok := db.HumanUserIDForActor(assigneeID); ok && assigneeHuman == actorHuman {
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
