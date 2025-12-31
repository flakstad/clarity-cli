package cli

import (
        "errors"
        "sort"
        "strconv"
        "strings"
        "time"

        "clarity-cli/internal/model"

        "github.com/spf13/cobra"
)

func newWorklogCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "worklog",
                Short: "Private worklog entries (visible only to the owning human user)",
        }
        cmd.AddCommand(newWorklogAddCmd(app))
        cmd.AddCommand(newWorklogListCmd(app))
        return cmd
}

func newWorklogAddCmd(app *App) *cobra.Command {
        var body string

        cmd := &cobra.Command{
                Use:   "add <item-id>",
                Short: "Add a worklog entry",
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
                        body = strings.TrimSpace(body)
                        if body == "" {
                                return writeErr(cmd, errors.New("missing --body"))
                        }

                        w := model.WorklogEntry{
                                ID:        s.NextID(db, "wlg"),
                                ItemID:    itemID,
                                AuthorID:  actorID,
                                Body:      body,
                                CreatedAt: time.Now().UTC(),
                        }
                        db.Worklog = append(db.Worklog, w)
                        if err := s.AppendEvent(actorID, "worklog.add", w.ID, w); err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{"data": w})
                },
        }

        cmd.Flags().StringVar(&body, "body", "", "Worklog body")
        _ = cmd.MarkFlagRequired("body")
        return cmd
}

func newWorklogListCmd(app *App) *cobra.Command {
        var limit int
        var offset int
        cmd := &cobra.Command{
                Use:   "list <item-id>",
                Short: "List worklog entries for an item (filtered to your human user; paginated)",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        actorID, err := currentActorID(app, db)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        itemID := args[0]
                        if _, ok := db.FindItem(itemID); !ok {
                                return writeErr(cmd, errNotFound("item", itemID))
                        }

                        humanID, ok := db.HumanUserIDForActor(actorID)
                        if !ok {
                                return writeErr(cmd, errors.New("unable to resolve human user for current actor"))
                        }

                        all := make([]model.WorklogEntry, 0)
                        for _, w := range db.Worklog {
                                if w.ItemID != itemID {
                                        continue
                                }
                                authorHuman, ok := db.HumanUserIDForActor(w.AuthorID)
                                if !ok {
                                        continue
                                }
                                if authorHuman != humanID {
                                        continue
                                }
                                all = append(all, w)
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
                                "clarity worklog list " + itemID + " --limit 0",
                        }
                        if end < total {
                                hints = append(hints, "clarity worklog list "+itemID+" --limit "+strconv.Itoa(limit)+" --offset "+strconv.Itoa(end))
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
        cmd.Flags().IntVar(&limit, "limit", 20, "Max entries to return (0 = all)")
        cmd.Flags().IntVar(&offset, "offset", 0, "Offset into worklog list (for pagination)")
        return cmd
}
