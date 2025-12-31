package cli

import (
        "sort"
        "strconv"
        "strings"
        "time"

        "clarity-cli/internal/model"

        "github.com/spf13/cobra"
)

func newCommentsCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "comments",
                Short: "Comment commands",
        }
        cmd.AddCommand(newCommentsAddCmd(app))
        cmd.AddCommand(newCommentsListCmd(app))
        return cmd
}

func newCommentsAddCmd(app *App) *cobra.Command {
        var body string

        cmd := &cobra.Command{
                Use:   "add <item-id>",
                Short: "Add a comment to an item",
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
                        if _, ok := db.FindActor(actorID); !ok {
                                return writeErr(cmd, errNotFound("actor", actorID))
                        }

                        itemID := args[0]
                        if _, ok := db.FindItem(itemID); !ok {
                                return writeErr(cmd, errNotFound("item", itemID))
                        }

                        c := model.Comment{
                                ID:        s.NextID(db, "cmt"),
                                ItemID:    itemID,
                                AuthorID:  actorID,
                                Body:      strings.TrimSpace(body),
                                CreatedAt: time.Now().UTC(),
                        }
                        db.Comments = append(db.Comments, c)
                        if err := s.AppendEvent(actorID, "comment.add", c.ID, c); err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{"data": c})
                },
        }

        cmd.Flags().StringVar(&body, "body", "", "Comment body")
        _ = cmd.MarkFlagRequired("body")
        return cmd
}

func newCommentsListCmd(app *App) *cobra.Command {
        var limit int
        var offset int

        cmd := &cobra.Command{
                Use:   "list <item-id>",
                Short: "List comments for an item (paginated)",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        itemID := args[0]
                        if _, ok := db.FindItem(itemID); !ok {
                                return writeErr(cmd, errNotFound("item", itemID))
                        }

                        all := make([]model.Comment, 0)
                        for _, c := range db.Comments {
                                if c.ItemID == itemID {
                                        all = append(all, c)
                                }
                        }

                        sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt.Before(all[j].CreatedAt) })

                        total := len(all)
                        if offset < 0 {
                                offset = 0
                        }
                        if offset > total {
                                offset = total
                        }

                        end := total
                        if limit > 0 && offset+limit < end {
                                end = offset + limit
                        }
                        out := all[offset:end]

                        hints := []string{
                                "clarity comments list " + itemID + " --limit 0",
                        }
                        if end < total {
                                hints = append(hints, "clarity comments list "+itemID+" --limit "+strconv.Itoa(limit)+" --offset "+strconv.Itoa(end))
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": out,
                                "meta": map[string]any{
                                        "total":    total,
                                        "limit":    limit,
                                        "offset":   offset,
                                        "returned": len(out),
                                },
                                "_hints": hints,
                        })
                },
        }
        cmd.Flags().IntVar(&limit, "limit", 20, "Max comments to return (0 = all)")
        cmd.Flags().IntVar(&offset, "offset", 0, "Offset into comment list (for pagination)")
        return cmd
}
