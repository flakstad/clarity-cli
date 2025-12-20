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

                        // Siblings ordered by rank.
                        sibs := siblingItems(db, t.OutlineID, t.ParentID)
                        // Remove the moved item.
                        sibs = filterItems(sibs, func(x *model.Item) bool { return x.ID != id })

                        refIdx := indexOfItem(sibs, refID)
                        if refIdx < 0 {
                                return writeErr(cmd, errors.New("reference item not found among siblings"))
                        }

                        var lower string
                        var upper string

                        switch mode {
                        case "before":
                                upper = sibs[refIdx].Rank
                                if refIdx > 0 {
                                        lower = sibs[refIdx-1].Rank
                                }
                        case "after":
                                lower = sibs[refIdx].Rank
                                if refIdx+1 < len(sibs) {
                                        upper = sibs[refIdx+1].Rank
                                }
                        }

                        r, err := store.RankBetween(lower, upper)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        t.Rank = r
                        t.UpdatedAt = time.Now().UTC()

                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.move", t.ID, map[string]any{"before": before, "after": after, "rank": t.Rank})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }
        cmd.Flags().StringVar(&before, "before", "", "Move before item id")
        cmd.Flags().StringVar(&after, "after", "", "Move after item id")
        return cmd
}

func newItemsSetParentCmd(app *App) *cobra.Command {
        var parent string
        var before string
        var after string

        cmd := &cobra.Command{
                Use:   "set-parent <item-id>",
                Short: "Reparent an item (owner-only); ordering uses ranks",
                Aliases: []string{
                        "parent",
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

                        if (before != "" && after != "") || (before != "" && strings.TrimSpace(parent) == "") || (after != "" && strings.TrimSpace(parent) == "") {
                                // You can reparent without before/after. But before/after must be interpreted in the *new* sibling set,
                                // so we require --parent to be explicit in that case.
                                return writeErr(cmd, errors.New("use at most one of --before/--after; and pass --parent when using them"))
                        }

                        var newParentID *string
                        if strings.TrimSpace(parent) != "" && strings.ToLower(strings.TrimSpace(parent)) != "none" {
                                pid := strings.TrimSpace(parent)
                                p, ok := db.FindItem(pid)
                                if !ok {
                                        return writeErr(cmd, errNotFound("item", pid))
                                }
                                if p.OutlineID != t.OutlineID {
                                        return writeErr(cmd, errors.New("parent must be in the same outline"))
                                }
                                if pid == t.ID || isAncestor(db, t.ID, pid) {
                                        return writeErr(cmd, errors.New("cannot set parent (cycle)"))
                                }
                                newParentID = &pid
                        }

                        // Determine new rank in the destination sibling set.
                        var sibs []*model.Item
                        sibs = siblingItems(db, t.OutlineID, newParentID)
                        sibs = filterItems(sibs, func(x *model.Item) bool { return x.ID != t.ID })

                        var lower string
                        var upper string

                        if before != "" || after != "" {
                                refID := before
                                mode := "before"
                                if after != "" {
                                        refID = after
                                        mode = "after"
                                }
                                refIdx := indexOfItem(sibs, refID)
                                if refIdx < 0 {
                                        return writeErr(cmd, errors.New("reference item not found among destination siblings"))
                                }
                                switch mode {
                                case "before":
                                        upper = sibs[refIdx].Rank
                                        if refIdx > 0 {
                                                lower = sibs[refIdx-1].Rank
                                        }
                                case "after":
                                        lower = sibs[refIdx].Rank
                                        if refIdx+1 < len(sibs) {
                                                upper = sibs[refIdx+1].Rank
                                        }
                                }
                        } else {
                                // Default: append to end.
                                if len(sibs) > 0 {
                                        lower = sibs[len(sibs)-1].Rank
                                }
                        }

                        r, err := store.RankBetween(lower, upper)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        t.ParentID = newParentID
                        t.Rank = r
                        t.UpdatedAt = time.Now().UTC()

                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.set_parent", t.ID, map[string]any{"parent": parent, "before": before, "after": after, "rank": t.Rank})
                        return writeOut(cmd, app, map[string]any{"data": t})
                },
        }

        cmd.Flags().StringVar(&parent, "parent", "none", "New parent item id (or 'none' for root)")
        cmd.Flags().StringVar(&before, "before", "", "Place before sibling id (in destination parent)")
        cmd.Flags().StringVar(&after, "after", "", "Place after sibling id (in destination parent)")
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

func siblingItems(db *store.DB, outlineID string, parentID *string) []*model.Item {
        var out []*model.Item
        for i := range db.Items {
                it := &db.Items[i]
                if it.OutlineID != outlineID {
                        continue
                }
                if !sameParent(it.ParentID, parentID) {
                        continue
                }
                out = append(out, it)
        }
        sort.Slice(out, func(i, j int) bool { return compareItemsByRank(*out[i], *out[j]) < 0 })
        return out
}

func compareItemsByRank(a, b model.Item) int {
        ra := strings.TrimSpace(a.Rank)
        rb := strings.TrimSpace(b.Rank)
        switch {
        case ra == "" && rb == "":
                if a.CreatedAt.Before(b.CreatedAt) {
                        return -1
                }
                if a.CreatedAt.After(b.CreatedAt) {
                        return 1
                }
                return 0
        case ra == "":
                return -1
        case rb == "":
                return 1
        default:
                if ra < rb {
                        return -1
                }
                if ra > rb {
                        return 1
                }
                return 0
        }
}

func filterItems(xs []*model.Item, keep func(*model.Item) bool) []*model.Item {
        out := make([]*model.Item, 0, len(xs))
        for _, x := range xs {
                if keep(x) {
                        out = append(out, x)
                }
        }
        return out
}

func indexOfItem(xs []*model.Item, id string) int {
        for i, x := range xs {
                if x.ID == id {
                        return i
                }
        }
        return -1
}

func isAncestor(db *store.DB, ancestorID, itemID string) bool {
        cur := itemID
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
