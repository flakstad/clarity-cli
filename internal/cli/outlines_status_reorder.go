package cli

import (
        "errors"

        "clarity-cli/internal/model"

        "github.com/spf13/cobra"
)

func newOutlinesStatusReorderCmd(app *App) *cobra.Command {
        var labels []string

        cmd := &cobra.Command{
                Use:   "reorder <outline-id>",
                Short: "Reorder status definitions (by labels; controls cycling order)",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        actorID, err := currentActorID(app, db)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        oid := args[0]
                        o, ok := db.FindOutline(oid)
                        if !ok {
                                return writeErr(cmd, errNotFound("outline", oid))
                        }
                        if len(labels) == 0 {
                                return writeErr(cmd, errors.New("missing --label (repeatable)"))
                        }

                        // Map label -> def (labels must be unique).
                        defByLabel := map[string]int{}
                        for i, def := range o.StatusDefs {
                                defByLabel[def.Label] = i
                        }

                        if len(labels) != len(o.StatusDefs) {
                                return writeErr(cmd, errors.New("must provide all status labels exactly once"))
                        }

                        seen := map[string]bool{}
                        var next []model.OutlineStatusDef
                        for _, l := range labels {
                                if seen[l] {
                                        return writeErr(cmd, errors.New("duplicate label in reorder list"))
                                }
                                seen[l] = true
                                idx, ok := defByLabel[l]
                                if !ok {
                                        return writeErr(cmd, errors.New("unknown status label in reorder list"))
                                }
                                next = append(next, o.StatusDefs[idx])
                        }

                        o.StatusDefs = next
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "outline.status.reorder", oid, map[string]any{"labels": labels})
                        return writeOut(cmd, app, map[string]any{"data": o})
                },
        }

        cmd.Flags().StringSliceVar(&labels, "label", nil, "Status label (repeatable, must include all)")
        return cmd
}
