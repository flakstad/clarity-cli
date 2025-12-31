package cli

import (
        "errors"
        "time"

        "github.com/spf13/cobra"
)

func newItemsMoveOutlineCmd(app *App) *cobra.Command {
        var to string
        var setStatus string

        cmd := &cobra.Command{
                Use:   "move-outline <item-id>",
                Short: "Move an item to another outline (owner-only) and optionally set status",
                Aliases: []string{
                        "move-to-outline",
                },
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

                        id := args[0]
                        t, ok := db.FindItem(id)
                        if !ok {
                                return writeErr(cmd, errNotFound("item", id))
                        }
                        if !canEditTask(db, actorID, t) {
                                return writeErr(cmd, errorsOwnerOnly(actorID, t.OwnerActorID, id))
                        }
                        if to == "" {
                                return writeErr(cmd, errors.New("missing --to"))
                        }
                        o, ok := db.FindOutline(to)
                        if !ok {
                                return writeErr(cmd, errNotFound("outline", to))
                        }
                        if o.ProjectID != t.ProjectID {
                                return writeErr(cmd, errors.New("target outline must belong to the same project"))
                        }

                        // Block move if status would be invalid in target outline unless user provides --set-status.
                        statusToUse := t.StatusID
                        if setStatus != "" {
                                statusToUse = setStatus
                        }
                        if _, ok := db.StatusDef(o.ID, statusToUse); !ok {
                                return writeErr(cmd, errors.New("invalid status id for target outline; pass --set-status"))
                        }

                        // Moving outlines detaches from parent (since parent must be same outline).
                        t.ParentID = nil
                        t.OutlineID = o.ID
                        t.StatusID = statusToUse
                        t.Rank = nextSiblingRank(db, o.ID, nil)
                        t.UpdatedAt = time.Now().UTC()

                        if err := s.AppendEvent(actorID, "item.move_outline", t.ID, map[string]any{"to": o.ID, "status": t.StatusID}); err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }

        cmd.Flags().StringVar(&to, "to", "", "Target outline id")
        cmd.Flags().StringVar(&setStatus, "set-status", "", "Set status id during move (required if current status invalid)")
        _ = cmd.MarkFlagRequired("to")
        return cmd
}
