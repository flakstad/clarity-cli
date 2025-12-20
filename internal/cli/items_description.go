package cli

import (
        "strings"
        "time"

        "github.com/spf13/cobra"
)

func newItemsSetDescriptionCmd(app *App) *cobra.Command {
        var description string

        cmd := &cobra.Command{
                Use:   "set-description <item-id>",
                Short: "Set item description (Markdown; owner-only)",
                Aliases: []string{
                        "desc",
                        "description",
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
                        it, ok := db.FindItem(id)
                        if !ok {
                                return writeErr(cmd, errNotFound("item", id))
                        }
                        if !canEditTask(db, actorID, it) {
                                return writeErr(cmd, errorsOwnerOnly(actorID, it.OwnerActorID, id))
                        }

                        it.Description = strings.TrimSpace(description)
                        it.UpdatedAt = time.Now().UTC()
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "item.set_description", it.ID, map[string]any{"description": it.Description})
                        return writeOut(cmd, app, map[string]any{"data": it})
                },
        }

        cmd.Flags().StringVar(&description, "description", "", "Markdown description")
        return cmd
}
