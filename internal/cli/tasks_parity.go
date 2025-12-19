package cli

import (
        "errors"
        "strings"
        "time"

        "github.com/spf13/cobra"
)

func newItemsSetPriorityCmd(app *App) *cobra.Command {
        var on bool
        var off bool

        cmd := &cobra.Command{
                Use:   "set-priority <item-id>",
                Short: "Set/unset priority (owner-only)",
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
        var actor string
        var clear bool
        cmd := &cobra.Command{
                Use:   "set-assign <item-id>",
                Short: "Set/clear assigned actor (owner-only; agents can self-assign unassigned items)",
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

                        if clear {
                                // Owner-only: clearing assignment does not transfer ownership.
                                if !canEditTask(db, actorID, t) {
                                        return writeErr(cmd, errorsOwnerOnly(actorID, t.OwnerActorID, id))
                                }
                                t.AssignedActorID = nil
                                t.UpdatedAt = time.Now().UTC()
                                if err := s.Save(db); err != nil {
                                        return writeErr(cmd, err)
                                }
                                _ = s.AppendEvent(actorID, "item.set_assign", t.ID, map[string]any{"assignedActorId": nil})
                                return writeOut(cmd, app, map[string]any{"data": t})
                        }

                        if strings.TrimSpace(actor) == "" {
                                return writeErr(cmd, errors.New("missing --actor (or pass --clear)"))
                        }
                        if _, ok := db.FindActor(actor); !ok {
                                return writeErr(cmd, errNotFound("actor", actor))
                        }

                        // Special case: allow an agent to self-assign an unassigned item
                        // belonging to the same human user, even if they're not the current owner.
                        isUnassigned := t.AssignedActorID == nil
                        isSelfAssign := actor == actorID
                        if isUnassigned && isSelfAssign {
                                curHuman, ok1 := db.HumanUserIDForActor(actorID)
                                ownerHuman, ok2 := db.HumanUserIDForActor(t.OwnerActorID)
                                if ok1 && ok2 && curHuman == ownerHuman {
                                        // OK: claim the item. Transfer ownership to the agent.
                                        tmp := actorID
                                        t.AssignedActorID = &tmp
                                        t.OwnerActorID = actorID
                                        t.OwnerDelegatedFrom = nil
                                        t.OwnerDelegatedAt = nil
                                } else if !canEditTask(db, actorID, t) {
                                        return writeErr(cmd, errorsOwnerOnly(actorID, t.OwnerActorID, id))
                                }
                        } else {
                                // Normal path: owner-only.
                                if !canEditTask(db, actorID, t) {
                                        return writeErr(cmd, errorsOwnerOnly(actorID, t.OwnerActorID, id))
                                }

                                // Transfer ownership when assigning to someone else.
                                if actor != t.OwnerActorID {
                                        now := time.Now().UTC()
                                        prev := t.OwnerActorID
                                        t.OwnerDelegatedFrom = &prev
                                        t.OwnerDelegatedAt = &now
                                        t.OwnerActorID = actor
                                }
                                tmp := actor
                                t.AssignedActorID = &tmp
                        }

                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.set_assign", t.ID, map[string]any{"assignedActorId": t.AssignedActorID})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().StringVar(&actor, "actor", "", "Actor id to assign to")
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
