package cli

import (
        "errors"
        "sort"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newItemsCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "items",
                Short: "Item commands",
        }

        cmd.AddCommand(newItemsCreateCmd(app))
        cmd.AddCommand(newItemsListCmd(app))
        cmd.AddCommand(newItemsShowCmd(app))
        cmd.AddCommand(newItemsSetTitleCmd(app))
        cmd.AddCommand(newItemsSetDescriptionCmd(app))
        cmd.AddCommand(newItemsSetStatusCmd(app))
        cmd.AddCommand(newItemsSetPriorityCmd(app))
        cmd.AddCommand(newItemsSetOnHoldCmd(app))
        cmd.AddCommand(newItemsSetDueCmd(app))
        cmd.AddCommand(newItemsSetScheduleCmd(app))
        cmd.AddCommand(newItemsSetAssignCmd(app))
        cmd.AddCommand(newItemsTagsCmd(app))
        cmd.AddCommand(newItemsArchiveCmd(app))
        cmd.AddCommand(newItemsReadyCmd(app))
        cmd.AddCommand(newItemsMoveCmd(app))
        cmd.AddCommand(newItemsSetParentCmd(app))
        cmd.AddCommand(newItemsMoveOutlineCmd(app))

        return cmd
}

func newItemsCreateCmd(app *App) *cobra.Command {
        var projectID string
        var outlineID string
        var parentID string
        var title string
        var description string
        var ownerID string
        var assignID string

        cmd := &cobra.Command{
                Use:   "create",
                Short: "Create an item",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        actorID, err := currentActorID(app, db)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        if _, ok := db.FindActor(actorID); !ok {
                                return writeErr(cmd, errNotFound("actor", actorID))
                        }
                        pid := strings.TrimSpace(projectID)
                        if pid == "" {
                                pid = strings.TrimSpace(db.CurrentProjectID)
                                if pid == "" {
                                        return writeErr(cmd, errors.New("missing --project (or set a current project with `clarity projects use <project-id>`)"))
                                }
                        }
                        if _, ok := db.FindProject(pid); !ok {
                                return writeErr(cmd, errNotFound("project", pid))
                        }

                        oid := strings.TrimSpace(outlineID)
                        if oid != "" {
                                o, ok := db.FindOutline(oid)
                                if !ok {
                                        return writeErr(cmd, errNotFound("outline", oid))
                                }
                                if o.ProjectID != pid {
                                        return writeErr(cmd, errors.New("outline must belong to the same project"))
                                }
                        } else {
                                // Outline selection rules:
                                // - 0 outlines in project: create default unnamed outline
                                // - 1 outline in project: use it
                                // - >1 outlines: require --outline (or user creates a new outline explicitly)
                                var outlines []model.Outline
                                for _, o := range db.Outlines {
                                        if o.ProjectID == pid {
                                                outlines = append(outlines, o)
                                        }
                                }
                                if len(outlines) == 0 {
                                        oid = db.EnsureDefaultOutline(pid, actorID, func(prefix string) string { return s.NextID(db, prefix) })
                                } else if len(outlines) == 1 {
                                        oid = outlines[0].ID
                                } else {
                                        return writeErr(cmd, errors.New("multiple outlines in project; pass --outline or create a new outline first"))
                                }
                        }

                        o := ownerID
                        if o == "" {
                                o = actorID
                        }
                        if _, ok := db.FindActor(o); !ok {
                                return writeErr(cmd, errNotFound("actor", o))
                        }

                        var a *string
                        if strings.TrimSpace(assignID) != "" {
                                if _, ok := db.FindActor(assignID); !ok {
                                        return writeErr(cmd, errNotFound("actor", assignID))
                                }
                                a = &assignID
                        } else {
                                // Default: if an agent creates an item, assign it to itself.
                                if act, ok := db.FindActor(actorID); ok && act.Kind == model.ActorKindAgent {
                                        tmp := actorID
                                        a = &tmp
                                }
                        }

                        var p *string
                        if strings.TrimSpace(parentID) != "" {
                                if _, ok := db.FindItem(parentID); !ok {
                                        return writeErr(cmd, errNotFound("item", parentID))
                                }
                                parent, _ := db.FindItem(parentID)
                                if parent != nil && parent.OutlineID != oid {
                                        return writeErr(cmd, errors.New("parent must be in the same outline"))
                                }
                                p = &parentID
                        }

                        now := time.Now().UTC()
                        t := model.Item{
                                ID:                 s.NextID(db, "item"),
                                ProjectID:          pid,
                                OutlineID:          oid,
                                ParentID:           p,
                                Rank:               nextSiblingRank(db, oid, p),
                                Title:              strings.TrimSpace(title),
                                Description:        description,
                                StatusID:           "todo",
                                Priority:           false,
                                OnHold:             false,
                                Due:                nil,
                                Schedule:           nil,
                                LegacyDueAt:        nil,
                                LegacyScheduledAt:  nil,
                                Tags:               nil,
                                Archived:           false,
                                OwnerActorID:       o,
                                AssignedActorID:    a,
                                OwnerDelegatedFrom: nil,
                                OwnerDelegatedAt:   nil,
                                CreatedBy:          actorID,
                                CreatedAt:          now,
                                UpdatedAt:          now,
                        }
                        db.Items = append(db.Items, t)
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.create", t.ID, t)
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }

        cmd.Flags().StringVar(&projectID, "project", "", "Project id (optional if a current project is set)")
        cmd.Flags().StringVar(&outlineID, "outline", "", "Outline id (optional; default: project's first outline)")
        cmd.Flags().StringVar(&parentID, "parent", "", "Parent item id (for outline nesting)")
        cmd.Flags().StringVar(&title, "title", "", "Item title")
        cmd.Flags().StringVar(&description, "description", "", "Markdown description (optional)")
        cmd.Flags().StringVar(&ownerID, "owner", "", "Owner actor id (default: current actor)")
        cmd.Flags().StringVar(&assignID, "assign", "", "Assigned actor id (optional; default: agent assigns to itself, human leaves unassigned)")
        _ = cmd.MarkFlagRequired("title")

        return cmd
}

func newItemsListCmd(app *App) *cobra.Command {
        var projectID string
        var outlineID string
        var mine bool
        var status string
        var includeArchived bool

        cmd := &cobra.Command{
                Use:   "list",
                Short: "List items",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        var actorID string
                        if mine {
                                a, err := currentActorID(app, db)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                actorID = a
                        }

                        var wantStatusID string
                        var filterStatus bool
                        if strings.TrimSpace(status) != "" {
                                s, err := store.ParseStatusID(status)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                wantStatusID = s
                                filterStatus = true
                        }

                        out := make([]model.Item, 0)
                        for _, t := range db.Items {
                                if !includeArchived && t.Archived {
                                        continue
                                }
                                if projectID != "" && t.ProjectID != projectID {
                                        continue
                                }
                                if outlineID != "" && t.OutlineID != outlineID {
                                        continue
                                }
                                if mine && (t.AssignedActorID == nil || *t.AssignedActorID != actorID) {
                                        continue
                                }
                                if filterStatus && t.StatusID != wantStatusID {
                                        continue
                                }
                                out = append(out, t)
                        }

                        sort.Slice(out, func(i, j int) bool {
                                if out[i].ProjectID != out[j].ProjectID {
                                        return out[i].ProjectID < out[j].ProjectID
                                }
                                if out[i].OutlineID != out[j].OutlineID {
                                        return out[i].OutlineID < out[j].OutlineID
                                }
                                pi := ""
                                pj := ""
                                if out[i].ParentID != nil {
                                        pi = *out[i].ParentID
                                }
                                if out[j].ParentID != nil {
                                        pj = *out[j].ParentID
                                }
                                if pi != pj {
                                        return pi < pj
                                }
                                ri := strings.TrimSpace(out[i].Rank)
                                rj := strings.TrimSpace(out[j].Rank)
                                if ri != "" && rj != "" {
                                        return ri < rj
                                }
                                if ri != "" && rj == "" {
                                        return false
                                }
                                if ri == "" && rj != "" {
                                        return true
                                }
                                return out[i].CreatedAt.Before(out[j].CreatedAt)
                        })

                        return writeOut(cmd, app, map[string]any{"data": out})
                },
        }

        cmd.Flags().StringVar(&projectID, "project", "", "Project id (optional)")
        cmd.Flags().StringVar(&outlineID, "outline", "", "Outline id (optional)")
        cmd.Flags().BoolVar(&mine, "mine", false, "Only items assigned to current actor")
        cmd.Flags().StringVar(&status, "status", "", "Filter by status id (e.g. todo|doing|done)")
        cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived items")

        return cmd
}

func newItemsShowCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "show <item-id>",
                Short: "Show an item",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        id := args[0]
                        t, ok := db.FindItem(id)
                        if !ok {
                                return writeErr(cmd, errNotFound("item", id))
                        }

                        // Progressive disclosure: don't inline large collections by default.
                        commentsCount := 0
                        for _, c := range db.Comments {
                                if c.ItemID == id {
                                        commentsCount++
                                }
                        }

                        var worklogCount *int
                        if actorID, err := currentActorID(app, db); err == nil {
                                if humanID, ok := db.HumanUserIDForActor(actorID); ok {
                                        n := 0
                                        for _, w := range db.Worklog {
                                                if w.ItemID != id {
                                                        continue
                                                }
                                                if authorHuman, ok := db.HumanUserIDForActor(w.AuthorID); ok && authorHuman == humanID {
                                                        n++
                                                }
                                        }
                                        worklogCount = &n
                                }
                        }

                        depsOut := 0
                        depsIn := 0
                        for _, d := range db.Deps {
                                if d.FromItemID == id && d.Type == model.DependencyBlocks {
                                        depsOut++
                                }
                                if d.ToItemID == id && d.Type == model.DependencyBlocks {
                                        depsIn++
                                }
                        }

                        hints := []string{
                                "clarity comments list " + id,
                                "clarity worklog list " + id,
                                "clarity deps list " + id,
                                "clarity deps tree " + id,
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": t,
                                "meta": map[string]any{
                                        "comments": map[string]any{
                                                "count": commentsCount,
                                        },
                                        "worklog": map[string]any{
                                                "count": worklogCount,
                                        },
                                        "deps": map[string]any{
                                                "blocks": map[string]any{
                                                        "out": depsOut,
                                                        "in":  depsIn,
                                                },
                                        },
                                },
                                "_hints": hints,
                        })
                },
        }
        return cmd
}

func newItemsSetTitleCmd(app *App) *cobra.Command {
        var title string
        cmd := &cobra.Command{
                Use:   "set-title <item-id>",
                Short: "Set item title (owner-only)",
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
                        t.Title = strings.TrimSpace(title)
                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.set_title", t.ID, map[string]any{"title": t.Title})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().StringVar(&title, "title", "", "New title")
        _ = cmd.MarkFlagRequired("title")
        return cmd
}

func newItemsSetStatusCmd(app *App) *cobra.Command {
        var status string
        cmd := &cobra.Command{
                Use:   "set-status <item-id>",
                Short: "Set item status (owner-only)",
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
                        st, err := store.ParseStatusID(status)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if st == "" {
                                // allow "no status"
                        } else if _, ok := db.StatusDef(t.OutlineID, st); !ok {
                                // also allow labels as input by resolving them
                                if sid, ok := resolveStatusIDByLabel(db, t.OutlineID, st); ok {
                                        st = sid
                                } else {
                                        return writeErr(cmd, errors.New("invalid status for this outline"))
                                }
                        }
                        if isEndState(db, t.OutlineID, st) {
                                if hasIncompleteChildren(db, t.ID) || isBlockedByUndoneDeps(db, t.ID) {
                                        return writeErr(cmd, completionBlockedError{taskID: t.ID, reason: explainCompletionBlockers(db, t.ID)})
                                }
                        }
                        t.StatusID = st
                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.set_status", t.ID, map[string]any{"status": t.StatusID})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().StringVar(&status, "status", "", "New status (status id, label, or 'none')")
        _ = cmd.MarkFlagRequired("status")
        return cmd
}

func newItemsReadyCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "ready",
                Short: "List items with no blocking dependencies (simple check)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        blocked := map[string]bool{}
                        for _, d := range db.Deps {
                                if d.Type == model.DependencyBlocks {
                                        blocked[d.FromItemID] = true
                                }
                        }

                        out := make([]model.Item, 0)
                        for _, t := range db.Items {
                                if t.Archived {
                                        continue
                                }
                                if blocked[t.ID] {
                                        continue
                                }
                                if isEndState(db, t.OutlineID, t.StatusID) {
                                        continue
                                }
                                out = append(out, t)
                        }
                        return writeOut(cmd, app, map[string]any{"data": out})
                },
        }
        return cmd
}

func nextSiblingRank(db *store.DB, outlineID string, parentID *string) string {
        // Append to end of sibling list.
        max := ""
        for _, t := range db.Items {
                if t.OutlineID != outlineID {
                        continue
                }
                if (t.ParentID == nil) != (parentID == nil) {
                        continue
                }
                if t.ParentID != nil && parentID != nil && *t.ParentID != *parentID {
                        continue
                }
                r := strings.TrimSpace(t.Rank)
                if r != "" && r > max {
                        max = r
                }
        }
        if max == "" {
                r, err := store.RankInitial()
                if err != nil {
                        return "h"
                }
                return r
        }
        r, err := store.RankAfter(max)
        if err != nil {
                return max + "0"
        }
        return r
}
