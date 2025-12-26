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
        cmd.AddCommand(newItemsEventsCmd(app))
        cmd.AddCommand(newItemsClaimCmd(app))
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
        var filedFrom string
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

                        outline, ok := db.FindOutline(oid)
                        if !ok || outline == nil {
                                return writeErr(cmd, errNotFound("outline", oid))
                        }

                        desc := description
                        if ff := strings.TrimSpace(filedFrom); ff != "" {
                                origin := "Filed from: " + ff
                                // Avoid duplicating a manually-provided origin line.
                                if !strings.Contains(desc, "Filed from:") {
                                        if strings.TrimSpace(desc) == "" {
                                                desc = origin
                                        } else {
                                                desc = origin + "\n\n" + desc
                                        }
                                }
                        }

                        now := time.Now().UTC()
                        t := model.Item{
                                ID:                 s.NextID(db, "item"),
                                ProjectID:          pid,
                                OutlineID:          oid,
                                ParentID:           p,
                                Rank:               nextSiblingRank(db, oid, p),
                                Title:              strings.TrimSpace(title),
                                Description:        desc,
                                StatusID:           store.FirstStatusID(outline.StatusDefs),
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
        cmd.Flags().StringVar(&filedFrom, "filed-from", "", "Origin reference to include at top of description (e.g. item id; optional)")
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
                Aliases: []string{
                        "get",
                },
                Args: cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        return showItem(app, cmd, args[0])
                },
        }
        return cmd
}

func newItemsEventsCmd(app *App) *cobra.Command {
        var limit int
        cmd := &cobra.Command{
                Use:   "events <item-id>",
                Short: "List an item's event history (oldest-first)",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        id := strings.TrimSpace(args[0])
                        if id == "" {
                                return writeErr(cmd, errors.New("missing item id"))
                        }

                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if _, ok := db.FindItem(id); !ok {
                                return writeErr(cmd, errNotFound("item", id))
                        }

                        evs, err := store.ReadEventsForEntity(s.Dir, id, limit)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{"data": evs})
                },
        }
        cmd.Flags().IntVar(&limit, "limit", 200, "Max events to return (0 = all)")
        return cmd
}

func showItem(app *App, cmd *cobra.Command, id string) error {
        db, _, err := loadDB(app)
        if err != nil {
                return writeErr(cmd, err)
        }
        t, ok := db.FindItem(id)
        if !ok {
                return writeErr(cmd, errNotFound("item", id))
        }

        // Progressive disclosure: keep large collections out by default, but DO include
        // subitems and dependency edges so callers can reason about completion blockers.
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

        // Children (direct subitems).
        type childSummary struct {
                ID        string `json:"id"`
                Title     string `json:"title"`
                StatusID  string `json:"status"`
                Archived  bool   `json:"archived"`
                OutlineID string `json:"outlineId"`
                ProjectID string `json:"projectId"`
        }
        children := make([]childSummary, 0)
        childrenDone := 0
        childrenOpen := 0
        childrenArchived := 0
        for _, it := range db.Items {
                if it.ParentID == nil || strings.TrimSpace(*it.ParentID) != id {
                        continue
                }
                ch := childSummary{
                        ID:        it.ID,
                        Title:     it.Title,
                        StatusID:  strings.TrimSpace(it.StatusID),
                        Archived:  it.Archived,
                        OutlineID: it.OutlineID,
                        ProjectID: it.ProjectID,
                }
                children = append(children, ch)
                if it.Archived {
                        childrenArchived++
                        continue
                }
                if isEndState(db, it.OutlineID, it.StatusID) {
                        childrenDone++
                } else {
                        childrenOpen++
                }
        }

        // Dependencies (include edges so clients can show blockers without extra calls).
        type depEdge struct {
                Type          string `json:"type"`      // blocks|related
                Direction     string `json:"direction"` // in|out
                OtherItemID   string `json:"otherItemId"`
                OtherTitle    string `json:"otherTitle,omitempty"`
                OtherStatus   string `json:"otherStatus,omitempty"`
                OtherDone     bool   `json:"otherDone"`
                OtherArchived bool   `json:"otherArchived"`
        }
        depsBlocksOut := make([]depEdge, 0)
        depsBlocksIn := make([]depEdge, 0)
        depsRelated := make([]depEdge, 0)
        for _, d := range db.Deps {
                switch d.Type {
                case model.DependencyBlocks:
                        if d.FromItemID == id {
                                other, _ := db.FindItem(d.ToItemID)
                                edge := depEdge{Type: string(d.Type), Direction: "out", OtherItemID: d.ToItemID}
                                if other != nil {
                                        edge.OtherTitle = other.Title
                                        edge.OtherStatus = strings.TrimSpace(other.StatusID)
                                        edge.OtherDone = isEndState(db, other.OutlineID, other.StatusID)
                                        edge.OtherArchived = other.Archived
                                }
                                depsBlocksOut = append(depsBlocksOut, edge)
                        }
                        if d.ToItemID == id {
                                other, _ := db.FindItem(d.FromItemID)
                                edge := depEdge{Type: string(d.Type), Direction: "in", OtherItemID: d.FromItemID}
                                if other != nil {
                                        edge.OtherTitle = other.Title
                                        edge.OtherStatus = strings.TrimSpace(other.StatusID)
                                        edge.OtherDone = isEndState(db, other.OutlineID, other.StatusID)
                                        edge.OtherArchived = other.Archived
                                }
                                depsBlocksIn = append(depsBlocksIn, edge)
                        }
                case model.DependencyRelated:
                        // Include related edges in both directions (so callers can show adjacency).
                        if d.FromItemID == id || d.ToItemID == id {
                                otherID := d.ToItemID
                                dir := "out"
                                if d.ToItemID == id {
                                        otherID = d.FromItemID
                                        dir = "in"
                                }
                                other, _ := db.FindItem(otherID)
                                edge := depEdge{Type: string(d.Type), Direction: dir, OtherItemID: otherID}
                                if other != nil {
                                        edge.OtherTitle = other.Title
                                        edge.OtherStatus = strings.TrimSpace(other.StatusID)
                                        edge.OtherDone = isEndState(db, other.OutlineID, other.StatusID)
                                        edge.OtherArchived = other.Archived
                                }
                                depsRelated = append(depsRelated, edge)
                        }
                }
        }

        hints := []string{
                "clarity comments list " + id,
                "clarity worklog list " + id,
                "clarity deps list " + id,
                "clarity deps tree " + id,
                "clarity items events " + id,
        }

        // Agent-friendly pickup hints: nudge agents into the intended workflow without
        // changing any core command semantics.
        if actorID, err := currentActorID(app, db); err == nil {
                if act, ok := db.FindActor(actorID); ok && act.Kind == model.ActorKindAgent {
                        doingID, hasDoing := preferredInProgressStatusID(db, t.OutlineID)
                        doneID := preferredDoneStatusID(db, t.OutlineID)
                        curStatus := strings.TrimSpace(t.StatusID)

                        pickup := []string{
                                "clarity items claim " + id,
                        }
                        if !isEndState(db, t.OutlineID, curStatus) {
                                if hasDoing && doingID != "" && doingID != curStatus {
                                        pickup = append(pickup, "clarity items set-status "+id+" --status "+doingID)
                                } else if !hasDoing {
                                        pickup = append(pickup, "clarity items set-status "+id+" --status <doing-status>")
                                }
                                pickup = append(pickup, "clarity worklog add "+id+" --body \"...\"")
                                pickup = append(pickup, "clarity comments add "+id+" --body \"...\"")
                                if doneID != "" && doneID != curStatus {
                                        pickup = append(pickup, "clarity items set-status "+id+" --status "+doneID)
                                }
                        }
                        hints = append(pickup, hints...)
                }
        }

        return writeOut(cmd, app, map[string]any{
                "data": map[string]any{
                        "item": t,
                        "children": map[string]any{
                                "items": children,
                                "counts": map[string]any{
                                        "total":    len(children),
                                        "open":     childrenOpen,
                                        "done":     childrenDone,
                                        "archived": childrenArchived,
                                },
                        },
                        "deps": map[string]any{
                                "blocks": map[string]any{
                                        "out": depsBlocksOut,
                                        "in":  depsBlocksIn,
                                },
                                "related": depsRelated,
                        },
                },
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
}

func newItemsSetTitleCmd(app *App) *cobra.Command {
        var title string
        cmd := &cobra.Command{
                Use:   "set-title <item-id>",
                Short: "Set item title (owner-only)",
                Aliases: []string{
                        "title",
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
                Aliases: []string{
                        "status",
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
                        prev := strings.TrimSpace(t.StatusID)
                        t.StatusID = st
                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.set_status", t.ID, map[string]any{
                                "from":   prev,
                                "to":     strings.TrimSpace(t.StatusID),
                                "status": strings.TrimSpace(t.StatusID), // backwards-compat
                        })
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().StringVar(&status, "status", "", "New status (status id, label, or 'none')")
        _ = cmd.MarkFlagRequired("status")
        return cmd
}

func newItemsReadyCmd(app *App) *cobra.Command {
        var includeAssigned bool
        var includeOnHold bool
        cmd := &cobra.Command{
                Use:   "ready",
                Short: "List ready items (good for picking the next task)",
                Long: strings.TrimSpace(`
List items that are not archived, not in an end-state, and have no blocking dependencies.
By default, items that are on-hold are excluded.

This is the recommended way to find the next thing to work on.
`),
                Example: strings.TrimSpace(`
clarity items ready
clarity items ready --pretty
clarity items ready --include-assigned
clarity items ready --include-on-hold

# Open an item from the list
clarity <item-id>
`),
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
                                if !includeOnHold && t.OnHold {
                                        continue
                                }
                                if blocked[t.ID] {
                                        continue
                                }
                                if !includeAssigned && t.AssignedActorID != nil && strings.TrimSpace(*t.AssignedActorID) != "" {
                                        continue
                                }
                                if isEndState(db, t.OutlineID, t.StatusID) {
                                        continue
                                }
                                out = append(out, t)
                        }
                        hints := []string{
                                "clarity <item-id>",
                                "clarity items show <item-id>",
                                "clarity items claim <item-id>",
                                "clarity items set-status <item-id> --status <doing-status>",
                                "clarity worklog add <item-id> --body \"...\"",
                                "clarity comments add <item-id> --body \"...\"",
                        }
                        return writeOut(cmd, app, map[string]any{"data": out, "_hints": hints})
                },
        }
        cmd.Flags().BoolVar(&includeAssigned, "include-assigned", false, "Include items already assigned to an actor")
        cmd.Flags().BoolVar(&includeOnHold, "include-on-hold", false, "Include items marked as on-hold")
        return cmd
}

func preferredInProgressStatusID(db *store.DB, outlineID string) (string, bool) {
        o, ok := db.FindOutline(outlineID)
        if !ok || o == nil {
                return "", false
        }

        // 1) If "doing" is explicitly present, use it.
        for _, def := range o.StatusDefs {
                if def.IsEndState {
                        continue
                }
                if strings.EqualFold(strings.TrimSpace(def.ID), "doing") {
                        return def.ID, true
                }
        }

        // 2) Heuristics on label/id.
        for _, def := range o.StatusDefs {
                if def.IsEndState {
                        continue
                }
                id := strings.ToLower(strings.TrimSpace(def.ID))
                label := strings.ToLower(strings.TrimSpace(def.Label))
                if id == "wip" || strings.Contains(id, "progress") || strings.Contains(id, "active") {
                        return def.ID, true
                }
                if label == "doing" || label == "wip" || strings.Contains(label, "in progress") || strings.Contains(label, "in-progress") || strings.Contains(label, "progress") || strings.Contains(label, "active") {
                        return def.ID, true
                }
        }

        // 3) Fallback: choose the second non-end-state status (often the "in progress" column).
        nonEnd := make([]string, 0)
        for _, def := range o.StatusDefs {
                if def.IsEndState {
                        continue
                }
                nonEnd = append(nonEnd, def.ID)
        }
        if len(nonEnd) >= 2 {
                return nonEnd[1], true
        }

        return "", false
}

func preferredDoneStatusID(db *store.DB, outlineID string) string {
        o, ok := db.FindOutline(outlineID)
        if ok && o != nil {
                for _, def := range o.StatusDefs {
                        if def.IsEndState {
                                return def.ID
                        }
                }
        }
        return "done"
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
