package cli

import (
        "errors"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newItemsClaimCmd(app *App) *cobra.Command {
        var takeAssigned bool
        cmd := &cobra.Command{
                Use:     "claim <item-id>",
                Aliases: []string{"assign-self"},
                Short:   "Assign current actor to the item (agents can claim unassigned items)",
                Long: strings.TrimSpace(`
Assigns the current actor to the item.

By default, this refuses to take an item that is already assigned to a different actor.
Use --take-assigned to explicitly take it anyway (soft coordination between agents).
`),
                Example: strings.TrimSpace(`
clarity items ready
clarity items claim <item-id>

# Explicitly take an already-assigned item:
clarity items claim <item-id> --take-assigned
`),
                Args: cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        actorID, err := currentActorID(app, db)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        id := strings.TrimSpace(args[0])
                        updated, err := claimItemAsCurrentActor(app, db, s, id, claimOpts{TakeAssigned: takeAssigned})
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        hints := []string{
                                "clarity items show " + id,
                                "clarity worklog add " + id + " --body \"...\"",
                                "clarity comments add " + id + " --body \"...\"",
                        }
                        _ = actorID // for readability in future edits
                        return writeOut(cmd, app, map[string]any{"data": updated, "_hints": hints})
                },
        }
        cmd.Flags().BoolVar(&takeAssigned, "take-assigned", false, "Take the item even if it's already assigned to another actor")
        return cmd
}

type claimOpts struct {
        TakeAssigned bool
}

func claimItemAsCurrentActor(app *App, db *store.DB, s store.Store, itemID string, opts claimOpts) (*model.Item, error) {
        actorID, err := currentActorID(app, db)
        if err != nil {
                return nil, err
        }

        t, ok := db.FindItem(itemID)
        if !ok {
                return nil, errNotFound("item", itemID)
        }

        // No-op: already assigned to current actor.
        if t.AssignedActorID != nil && *t.AssignedActorID == actorID {
                return t, nil
        }

        // Soft coordination: don't take an already-assigned item unless explicitly requested.
        if t.AssignedActorID != nil && strings.TrimSpace(*t.AssignedActorID) != "" && *t.AssignedActorID != actorID && !opts.TakeAssigned {
                return nil, errors.New("item is already assigned; pass --take-assigned to take it anyway")
        }

        // Mirrors the behavior of items set-assign, but always targets the current actor.
        isUnassigned := t.AssignedActorID == nil
        if isUnassigned {
                // Allow an agent to claim an unassigned item belonging to the same human user.
                curHuman, ok1 := db.HumanUserIDForActor(actorID)
                ownerHuman, ok2 := db.HumanUserIDForActor(t.OwnerActorID)
                if ok1 && ok2 && curHuman == ownerHuman {
                        tmp := actorID
                        t.AssignedActorID = &tmp
                        t.OwnerActorID = actorID
                        t.OwnerDelegatedFrom = nil
                        t.OwnerDelegatedAt = nil
                } else if !canEditTask(db, actorID, t) {
                        return nil, errorsOwnerOnly(actorID, t.OwnerActorID, itemID)
                } else {
                        // Owner can assign to self; transfer ownership to self with delegation.
                        if actorID != t.OwnerActorID {
                                now := time.Now().UTC()
                                prev := t.OwnerActorID
                                t.OwnerDelegatedFrom = &prev
                                t.OwnerDelegatedAt = &now
                                t.OwnerActorID = actorID
                        }
                        tmp := actorID
                        t.AssignedActorID = &tmp
                }
        } else {
                // Assigned to someone else; owner-only (or delegated) to reassign.
                if !canEditTask(db, actorID, t) {
                        return nil, errorsOwnerOnly(actorID, t.OwnerActorID, itemID)
                }
                if actorID != t.OwnerActorID {
                        now := time.Now().UTC()
                        prev := t.OwnerActorID
                        t.OwnerDelegatedFrom = &prev
                        t.OwnerDelegatedAt = &now
                        t.OwnerActorID = actorID
                }
                tmp := actorID
                t.AssignedActorID = &tmp
        }

        t.UpdatedAt = time.Now().UTC()
        if err := s.Save(db); err != nil {
                return nil, err
        }
        _ = s.AppendEvent(actorID, "item.set_assign", t.ID, map[string]any{"assignedActorId": t.AssignedActorID})
        return t, nil
}

var _ = errors.New // keep imports stable if we tweak error paths later
