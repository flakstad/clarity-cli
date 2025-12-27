package cli

import (
        "errors"
        "strings"
        "time"

        "clarity-cli/internal/mutate"

        "github.com/spf13/cobra"
)

func newItemsSetPriorityCmd(app *App) *cobra.Command {
        var on bool
        var off bool

        cmd := &cobra.Command{
                Use:   "set-priority <item-id>",
                Short: "Set/unset priority (owner-only)",
                Aliases: []string{
                        "priority",
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

                        if on && off {
                                return writeErr(cmd, errors.New("use only one of --on or --off"))
                        }
                        if !on && !off {
                                // Toggle by default.
                                t.Priority = !t.Priority
                        } else if on {
                                t.Priority = true
                        } else {
                                t.Priority = false
                        }

                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.set_priority", t.ID, map[string]any{"priority": t.Priority})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }

        cmd.Flags().BoolVar(&on, "on", false, "Enable priority")
        cmd.Flags().BoolVar(&off, "off", false, "Disable priority")
        return cmd
}

func newItemsSetOnHoldCmd(app *App) *cobra.Command {
        var on bool
        var off bool
        cmd := &cobra.Command{
                Use:   "set-on-hold <item-id>",
                Short: "Set/unset on-hold (owner-only)",
                Aliases: []string{
                        "on-hold",
                        "hold",
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

                        if on && off {
                                return writeErr(cmd, errors.New("use only one of --on or --off"))
                        }
                        if !on && !off {
                                t.OnHold = !t.OnHold
                        } else if on {
                                t.OnHold = true
                        } else {
                                t.OnHold = false
                        }

                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.set_on_hold", t.ID, map[string]any{"onHold": t.OnHold})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().BoolVar(&on, "on", false, "Enable on-hold")
        cmd.Flags().BoolVar(&off, "off", false, "Disable on-hold")
        return cmd
}

func newItemsSetDueCmd(app *App) *cobra.Command {
        var at string
        var clear bool
        cmd := &cobra.Command{
                Use:   "set-due <item-id>",
                Short: "Set/clear due date (owner-only); accepts RFC3339 or local date/time",
                Aliases: []string{
                        "due",
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

                        if clear {
                                t.Due = nil
                        } else {
                                if strings.TrimSpace(at) == "" {
                                        return writeErr(cmd, errors.New("missing --at (or pass --clear)"))
                                }
                                dt, err := parseDateTime(at)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                t.Due = dt
                        }

                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.set_due", t.ID, map[string]any{"due": t.Due})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().StringVar(&at, "at", "", "Due datetime (RFC3339 or YYYY-MM-DD[ HH:MM])")
        cmd.Flags().BoolVar(&clear, "clear", false, "Clear due date")
        return cmd
}

func newItemsSetScheduleCmd(app *App) *cobra.Command {
        var at string
        var clear bool
        cmd := &cobra.Command{
                Use:   "set-schedule <item-id>",
                Short: "Set/clear schedule date (owner-only); accepts RFC3339 or local date/time",
                Aliases: []string{
                        "schedule",
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

                        if clear {
                                t.Schedule = nil
                        } else {
                                if strings.TrimSpace(at) == "" {
                                        return writeErr(cmd, errors.New("missing --at (or pass --clear)"))
                                }
                                dt, err := parseDateTime(at)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                t.Schedule = dt
                        }

                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.set_schedule", t.ID, map[string]any{"schedule": t.Schedule})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().StringVar(&at, "at", "", "Schedule datetime (RFC3339 or YYYY-MM-DD[ HH:MM])")
        cmd.Flags().BoolVar(&clear, "clear", false, "Clear schedule date")
        return cmd
}

func newItemsSetAssignCmd(app *App) *cobra.Command {
        var assignee string
        var clear bool
        cmd := &cobra.Command{
                Use:   "set-assign <item-id>",
                Short: "Set/clear assigned actor (owner-only; agents can self-assign unassigned items)",
                Aliases: []string{
                        "assign",
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
                        var target *string
                        if clear {
                                target = nil
                        } else {
                                if strings.TrimSpace(assignee) == "" {
                                        return writeErr(cmd, errors.New("missing --assignee (or pass --clear)"))
                                }
                                tmp := strings.TrimSpace(assignee)
                                target = &tmp
                        }

                        res, err := mutate.SetAssignedActor(db, actorID, id, target, mutate.AssignOpts{TakeAssigned: true})
                        if err != nil {
                                switch e := err.(type) {
                                case mutate.NotFoundError:
                                        return writeErr(cmd, errNotFound(e.Kind, e.ID))
                                case mutate.OwnerOnlyError:
                                        return writeErr(cmd, errorsOwnerOnly(actorID, e.OwnerActorID, id))
                                default:
                                        return writeErr(cmd, err)
                                }
                        }
                        if !res.Changed {
                                return writeOut(cmd, app, map[string]any{"data": res.Item})
                        }

                        res.Item.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.set_assign", res.Item.ID, res.EventPayload)
                        return writeOut(cmd, app, map[string]any{"data": res.Item})
                },
        }
        cmd.Flags().StringVar(&assignee, "assignee", "", "Actor id to assign to")
        cmd.Flags().StringVar(&assignee, "to", "", "Alias for --assignee")
        cmd.Flags().BoolVar(&clear, "clear", false, "Clear assignment")
        return cmd
}

func newItemsArchiveCmd(app *App) *cobra.Command {
        var unarchive bool
        cmd := &cobra.Command{
                Use:   "archive <item-id>",
                Short: "Archive (or unarchive) an item (owner-only)",
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
                        id := args[0]
                        t, ok := db.FindItem(id)
                        if !ok {
                                return writeErr(cmd, errNotFound("item", id))
                        }
                        if !canEditTask(db, actorID, t) {
                                return writeErr(cmd, errorsOwnerOnly(actorID, t.OwnerActorID, id))
                        }

                        t.Archived = !unarchive
                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.archive", t.ID, map[string]any{"archived": t.Archived})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().BoolVar(&unarchive, "unarchive", false, "Unarchive instead of archive")
        return cmd
}

func newItemsTagsCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "tags",
                Short: "Tag commands (owner-only mutations)",
        }
        cmd.AddCommand(newItemsTagsAddCmd(app))
        cmd.AddCommand(newItemsTagsRemoveCmd(app))
        cmd.AddCommand(newItemsTagsSetCmd(app))
        return cmd
}

func newItemsTagsAddCmd(app *App) *cobra.Command {
        var tag string
        cmd := &cobra.Command{
                Use:   "add <item-id>",
                Short: "Add a tag (owner-only)",
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
                        id := args[0]
                        t, ok := db.FindItem(id)
                        if !ok {
                                return writeErr(cmd, errNotFound("item", id))
                        }
                        if !canEditTask(db, actorID, t) {
                                return writeErr(cmd, errorsOwnerOnly(actorID, t.OwnerActorID, id))
                        }
                        tag = strings.TrimSpace(tag)
                        if tag == "" {
                                return writeErr(cmd, errors.New("missing --tag"))
                        }
                        if !containsString(t.Tags, tag) {
                                t.Tags = append(t.Tags, tag)
                        }
                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.tags_add", t.ID, map[string]any{"tag": tag})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().StringVar(&tag, "tag", "", "Tag to add")
        _ = cmd.MarkFlagRequired("tag")
        return cmd
}

func newItemsTagsRemoveCmd(app *App) *cobra.Command {
        var tag string
        cmd := &cobra.Command{
                Use:   "remove <item-id>",
                Short: "Remove a tag (owner-only)",
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
                        id := args[0]
                        t, ok := db.FindItem(id)
                        if !ok {
                                return writeErr(cmd, errNotFound("item", id))
                        }
                        if !canEditTask(db, actorID, t) {
                                return writeErr(cmd, errorsOwnerOnly(actorID, t.OwnerActorID, id))
                        }
                        tag = strings.TrimSpace(tag)
                        if tag == "" {
                                return writeErr(cmd, errors.New("missing --tag"))
                        }
                        t.Tags = removeString(t.Tags, tag)
                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.tags_remove", t.ID, map[string]any{"tag": tag})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().StringVar(&tag, "tag", "", "Tag to remove")
        _ = cmd.MarkFlagRequired("tag")
        return cmd
}

func newItemsTagsSetCmd(app *App) *cobra.Command {
        var tags []string
        cmd := &cobra.Command{
                Use:   "set <item-id>",
                Short: "Replace tags (owner-only)",
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
                        id := args[0]
                        t, ok := db.FindItem(id)
                        if !ok {
                                return writeErr(cmd, errNotFound("item", id))
                        }
                        if !canEditTask(db, actorID, t) {
                                return writeErr(cmd, errorsOwnerOnly(actorID, t.OwnerActorID, id))
                        }
                        var cleaned []string
                        for _, it := range tags {
                                it = strings.TrimSpace(it)
                                if it == "" {
                                        continue
                                }
                                if !containsString(cleaned, it) {
                                        cleaned = append(cleaned, it)
                                }
                        }
                        t.Tags = cleaned
                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.tags_set", t.ID, map[string]any{"tags": t.Tags})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().StringSliceVar(&tags, "tag", nil, "Tags (repeatable)")
        return cmd
}

func containsString(xs []string, s string) bool {
        for _, x := range xs {
                if x == s {
                        return true
                }
        }
        return false
}

func removeString(xs []string, s string) []string {
        var out []string
        for _, x := range xs {
                if x == s {
                        continue
                }
                out = append(out, x)
        }
        return out
}
