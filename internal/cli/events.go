package cli

import (
        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newEventsCmd(app *App) *cobra.Command {
        var limit int

        cmd := &cobra.Command{
                Use:   "events",
                Short: "Inspect the local event log (for future sync)",
        }

        listCmd := &cobra.Command{
                Use:   "list",
                Short: "List events (oldest-first)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        _, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        evs, err := store.ReadEvents(s.Dir, limit)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{"data": evs})
                },
        }
        listCmd.Flags().IntVar(&limit, "limit", 200, "Max events to return (0 = all)")

        cmd.AddCommand(listCmd)
        return cmd
}
