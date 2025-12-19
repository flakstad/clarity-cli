package cli

import (
        "os"

        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newStatusCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "status",
                Short: "Show local Clarity DB status",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        var eventsCount int
                        if st, err := os.Stat(s.Dir + "/events.jsonl"); err == nil && st.Size() > 0 {
                                // count lines lazily
                                evs, err := store.ReadEvents(s.Dir, 0)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                eventsCount = len(evs)
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "dir":            s.Dir,
                                        "currentActorId": db.CurrentActorID,
                                        "actors":         len(db.Actors),
                                        "projects":       len(db.Projects),
                                        "items":          len(db.Items),
                                        "deps":           len(db.Deps),
                                        "comments":       len(db.Comments),
                                        "events":         eventsCount,
                                },
                        })
                },
        }
        return cmd
}
