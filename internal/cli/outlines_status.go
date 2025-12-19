package cli

import (
        "errors"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newOutlinesStatusCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "status",
                Short: "Manage outline status definitions",
        }
        cmd.AddCommand(newOutlinesStatusListCmd(app))
        cmd.AddCommand(newOutlinesStatusAddCmd(app))
        cmd.AddCommand(newOutlinesStatusUpdateCmd(app))
        cmd.AddCommand(newOutlinesStatusRemoveCmd(app))
        cmd.AddCommand(newOutlinesStatusReorderCmd(app))
        return cmd
}

func newOutlinesStatusListCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "list <outline-id>",
                Short: "List status definitions for an outline",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        oid := args[0]
                        o, ok := db.FindOutline(oid)
                        if !ok {
                                return writeErr(cmd, errNotFound("outline", oid))
                        }
                        return writeOut(cmd, app, map[string]any{"data": o.StatusDefs})
                },
        }
        return cmd
}

func newOutlinesStatusAddCmd(app *App) *cobra.Command {
        var label string
        var end bool

        cmd := &cobra.Command{
                Use:   "add <outline-id>",
                Short: "Add a status definition to an outline",
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
                        label = strings.TrimSpace(label)
                        if label == "" {
                                return writeErr(cmd, errors.New("missing --label"))
                        }
                        for _, def := range o.StatusDefs {
                                if def.Label == label {
                                        return writeErr(cmd, errors.New("status label already exists on this outline"))
                                }
                        }

                        id := store.NewStatusIDFromLabel(o, label)
                        o.StatusDefs = append(o.StatusDefs, model.OutlineStatusDef{ID: id, Label: label, IsEndState: end})
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "outline.status.add", oid, map[string]any{"id": id, "label": label, "isEndState": end})
                        return writeOut(cmd, app, map[string]any{"data": o})
                },
        }

        cmd.Flags().StringVar(&label, "label", "", "Status label (display)")
        cmd.Flags().BoolVar(&end, "end", false, "Mark as end-state")
        _ = cmd.MarkFlagRequired("label")
        return cmd
}

func newOutlinesStatusUpdateCmd(app *App) *cobra.Command {
        var label string
        var end bool
        var notEnd bool

        cmd := &cobra.Command{
                Use:   "update <outline-id> <status-id-or-label>",
                Short: "Update a status definition",
                Args:  cobra.ExactArgs(2),
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
                        key := args[1]
                        o, ok := db.FindOutline(oid)
                        if !ok {
                                return writeErr(cmd, errNotFound("outline", oid))
                        }

                        label = strings.TrimSpace(label)
                        if end && notEnd {
                                return writeErr(cmd, errors.New("use only one of --end or --not-end"))
                        }

                        // Enforce label uniqueness if changing it.
                        if label != "" {
                                for _, def := range o.StatusDefs {
                                        if def.Label == label {
                                                return writeErr(cmd, errors.New("status label already exists on this outline"))
                                        }
                                }
                        }

                        found := false
                        for i := range o.StatusDefs {
                                if o.StatusDefs[i].ID != key && o.StatusDefs[i].Label != key {
                                        continue
                                }
                                found = true
                                if label != "" {
                                        o.StatusDefs[i].Label = label
                                }
                                if end {
                                        o.StatusDefs[i].IsEndState = true
                                }
                                if notEnd {
                                        o.StatusDefs[i].IsEndState = false
                                }
                                break
                        }
                        if !found {
                                return writeErr(cmd, errNotFound("status", key))
                        }

                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "outline.status.update", oid, map[string]any{"key": key, "label": label, "end": end, "notEnd": notEnd, "ts": time.Now().UTC()})
                        return writeOut(cmd, app, map[string]any{"data": o})
                },
        }

        cmd.Flags().StringVar(&label, "label", "", "New label (optional)")
        cmd.Flags().BoolVar(&end, "end", false, "Set end-state true")
        cmd.Flags().BoolVar(&notEnd, "not-end", false, "Set end-state false")
        return cmd
}

func newOutlinesStatusRemoveCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "remove <outline-id> <status-id-or-label>",
                Short: "Remove a status definition (blocked if used by any item)",
                Args:  cobra.ExactArgs(2),
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
                        key := args[1]
                        o, ok := db.FindOutline(oid)
                        if !ok {
                                return writeErr(cmd, errNotFound("outline", oid))
                        }

                        // Resolve key to id
                        sid := ""
                        for _, def := range o.StatusDefs {
                                if def.ID == key || def.Label == key {
                                        sid = def.ID
                                        break
                                }
                        }
                        if sid == "" {
                                return writeErr(cmd, errNotFound("status", key))
                        }

                        // Block removal if any item uses it.
                        for _, it := range db.Items {
                                if it.OutlineID == oid && it.StatusID == sid {
                                        return writeErr(cmd, errors.New("cannot remove status: in use by items"))
                                }
                        }

                        var next []model.OutlineStatusDef
                        for _, def := range o.StatusDefs {
                                if def.ID == sid {
                                        continue
                                }
                                next = append(next, def)
                        }
                        if len(next) == len(o.StatusDefs) {
                                return writeErr(cmd, errNotFound("status", sid))
                        }
                        if len(next) == 0 {
                                return writeErr(cmd, errors.New("cannot remove last status from an outline"))
                        }
                        o.StatusDefs = next
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "outline.status.remove", oid, map[string]any{"id": sid})
                        return writeOut(cmd, app, map[string]any{"data": o})
                },
        }
        return cmd
}
