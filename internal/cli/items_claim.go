package cli

import (
        "errors"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/mutate"
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

        res, err := mutate.SetAssignedActor(db, actorID, itemID, &actorID, mutate.AssignOpts{TakeAssigned: opts.TakeAssigned})
        if err != nil {
                switch e := err.(type) {
                case mutate.NotFoundError:
                        return nil, errNotFound(e.Kind, e.ID)
                case mutate.OwnerOnlyError:
                        return nil, errorsOwnerOnly(actorID, e.OwnerActorID, itemID)
                default:
                        return nil, err
                }
        }
        if !res.Changed {
                return res.Item, nil
        }

        res.Item.UpdatedAt = time.Now().UTC()
        if err := s.AppendEvent(actorID, "item.set_assign", res.Item.ID, res.EventPayload); err != nil {
                return nil, err
        }
        if err := s.Save(db); err != nil {
                return nil, err
        }
        return res.Item, nil
}

var _ = errors.New // keep imports stable if we tweak error paths later
