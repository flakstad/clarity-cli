package cli

import (
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
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "comment.add", c.ID, c)
                        return writeOut(cmd, app, map[string]any{"data": c})
                },
        }

        cmd.Flags().StringVar(&body, "body", "", "Comment body")
        _ = cmd.MarkFlagRequired("body")
        return cmd
}

func newCommentsListCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "list <item-id>",
                Short: "List comments for an item",
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
                        var out []model.Comment
                        for _, c := range db.Comments {
                                if c.ItemID == itemID {
                                        out = append(out, c)
                                }
                        }
                        return writeOut(cmd, app, map[string]any{"data": out})
                },
        }
        return cmd
}
