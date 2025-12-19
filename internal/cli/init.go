package cli

import (
        "path/filepath"

        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newInitCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "init",
                Short: "Initialize local storage (workspace-first)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := s.Ensure(); err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }

                        // If we're in workspace mode but no current workspace is set, set it.
                        if app.Workspace != "" {
                                cfg, err := store.LoadConfig()
                                if err == nil && cfg.CurrentWorkspace == "" {
                                        cfg.CurrentWorkspace = app.Workspace
                                        _ = store.SaveConfig(cfg)
                                }
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "dir":        app.Dir,
                                        "dbPath":     filepath.Join(app.Dir, "db.json"),
                                        "eventsPath": filepath.Join(app.Dir, "events.jsonl"),
                                },
                        })
                },
        }
        return cmd
}
