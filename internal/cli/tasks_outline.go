package cli

import (
        "errors"
        "sort"
        "time"

        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newItemsMoveCmd(app *App) *cobra.Command {
        var before string
        var after string
        cmd := &cobra.Command{
                Use:   "move <item-id>",
                Short: "Reorder an item among siblings (owner-only)",
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
                        if (before == "" && after == "") || (before != "" && after != "") {
                                return writeErr(cmd, errors.New("provide exactly one of --before or --after"))
                        }
                        refID := before
                        mode := "before"
                        if after != "" {
                                refID = after
                                mode = "after"
                        }
                        ref, ok := db.FindItem(refID)
                        if !ok {
                                return writeErr(cmd, errNotFound("item", refID))
                        }
                        if ref.ProjectID != t.ProjectID {
                                return writeErr(cmd, errors.New("items must be in the same project"))
                        }
                        if ref.OutlineID != t.OutlineID {
                                return writeErr(cmd, errors.New("items must be in the same outline"))
                        }
                        if !sameParent(ref.ParentID, t.ParentID) {
                                return writeErr(cmd, errors.New("items must have the same parent to reorder"))
                        }

                        sibs := siblings(db, t.ProjectID, t.ParentID)
                        sibs = removeTask(sibs, id)

                        refIdx := indexOfTask(sibs, refID)
                        if refIdx < 0 {
                                return writeErr(cmd, errors.New("reference item not found among siblings"))
                        }
                        insertAt := refIdx
                        if mode == "after" {
                                insertAt = refIdx + 1
                        }
                        sibs = insertTask(sibs, insertAt, id)
                        applySiblingOrders(db, sibs)
                        t.UpdatedAt = time.Now().UTC()

                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.move", t.ID, map[string]any{"before": before, "after": after})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().StringVar(&before, "before", "", "Move before item id")
        cmd.Flags().StringVar(&after, "after", "", "Move after item id")
        return cmd
}

func newItemsIndentCmd(app *App) *cobra.Command {
        var under string
        cmd := &cobra.Command{
                Use:   "indent <item-id>",
                Short: "Indent an item under another item (owner-only)",
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
                        if under == "" {
                                return writeErr(cmd, errors.New("missing --under"))
                        }
                        parent, ok := db.FindItem(under)
                        if !ok {
                                return writeErr(cmd, errNotFound("item", under))
                        }
                        if parent.ProjectID != t.ProjectID {
                                return writeErr(cmd, errors.New("items must be in the same project"))
                        }
                        if parent.OutlineID != t.OutlineID {
                                return writeErr(cmd, errors.New("items must be in the same outline"))
                        }
                        if under == id {
                                return writeErr(cmd, errors.New("cannot indent under itself"))
                        }
                        if isAncestor(db, under, id) {
                                return writeErr(cmd, errors.New("cannot indent under a descendant"))
                        }

                        // Normalize old siblings after removal.
                        oldSibs := siblings(db, t.ProjectID, t.ParentID)
                        oldSibs = removeTask(oldSibs, id)
                        applySiblingOrders(db, oldSibs)

                        // Set new parent and order at end of new siblings.
                        t.ParentID = &under
                        newSibs := siblings(db, t.ProjectID, t.ParentID)
                        newSibs = removeTask(newSibs, id)
                        newSibs = append(newSibs, id)
                        applySiblingOrders(db, newSibs)

                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.indent", t.ID, map[string]any{"under": under})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().StringVar(&under, "under", "", "Parent item id to indent under")
        _ = cmd.MarkFlagRequired("under")
        return cmd
}

func newItemsOutdentCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "outdent <item-id>",
                Short: "Outdent an item to its parent's parent (owner-only)",
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
                        if t.ParentID == nil {
                                return writeErr(cmd, errors.New("item has no parent"))
                        }
                        parentID := *t.ParentID
                        parent, ok := db.FindItem(parentID)
                        if !ok {
                                return writeErr(cmd, errNotFound("item", parentID))
                        }
                        if parent.OutlineID != t.OutlineID {
                                return writeErr(cmd, errors.New("parent must be in the same outline"))
                        }
                        grandParentID := parent.ParentID // may be nil

                        // Normalize old siblings after removal.
                        oldSibs := siblings(db, t.ProjectID, t.ParentID)
                        oldSibs = removeTask(oldSibs, id)
                        applySiblingOrders(db, oldSibs)

                        // Move under grandparent, insert after parent.
                        t.ParentID = grandParentID
                        newSibs := siblings(db, t.ProjectID, grandParentID)
                        newSibs = removeTask(newSibs, id)
                        parentIdx := indexOfTask(newSibs, parentID)
                        insertAt := parentIdx + 1
                        if parentIdx < 0 {
                                insertAt = len(newSibs)
                        }
                        newSibs = insertTask(newSibs, insertAt, id)
                        applySiblingOrders(db, newSibs)

                        t.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.outdent", t.ID, map[string]any{"fromParent": parentID})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        return cmd
}

func sameParent(a, b *string) bool {
        if a == nil && b == nil {
                return true
        }
        if a == nil || b == nil {
                return false
        }
        return *a == *b
}

func siblings(db *store.DB, projectID string, parentID *string) []string {
        type pair struct {
                id    string
                order int
        }
        var ps []pair
        for _, t := range db.Items {
                if t.ProjectID != projectID {
                        continue
                }
                if !sameParent(t.ParentID, parentID) {
                        continue
                }
                ps = append(ps, pair{id: t.ID, order: t.Order})
        }
        sort.Slice(ps, func(i, j int) bool { return ps[i].order < ps[j].order })
        out := make([]string, 0, len(ps))
        for _, p := range ps {
                out = append(out, p.id)
        }
        return out
}

func applySiblingOrders(db *store.DB, ids []string) {
        for idx, id := range ids {
                if t, ok := db.FindItem(id); ok {
                        t.Order = idx + 1
                }
        }
}

func removeTask(ids []string, id string) []string {
        var out []string
        for _, x := range ids {
                if x == id {
                        continue
                }
                out = append(out, x)
        }
        return out
}

func insertTask(ids []string, idx int, id string) []string {
        if idx < 0 {
                idx = 0
        }
        if idx > len(ids) {
                idx = len(ids)
        }
        out := append([]string{}, ids[:idx]...)
        out = append(out, id)
        out = append(out, ids[idx:]...)
        return out
}

func indexOfTask(ids []string, id string) int {
        for i, x := range ids {
                if x == id {
                        return i
                }
        }
        return -1
}

func isAncestor(db *store.DB, ancestorID, taskID string) bool {
        cur := taskID
        for {
                t, ok := db.FindItem(cur)
                if !ok || t.ParentID == nil {
                        return false
                }
                if *t.ParentID == ancestorID {
                        return true
                }
                cur = *t.ParentID
        }
}
