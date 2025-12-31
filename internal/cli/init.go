package cli

import (
        "os"
        "path/filepath"
        "strings"

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

                        // If we're initializing inside a Git repo (or already have v1 files),
                        // also bootstrap the Git-backed JSONL v1 layout.
                        //
                        // This is intentionally best-effort; even if this fails, the legacy
                        // SQLite-based store remains usable.
                        var v1Init *store.GitBackedV1InitResult
                        shouldInitV1 := false
                        if v := strings.ToLower(strings.TrimSpace(os.Getenv("CLARITY_EVENTLOG"))); v == "jsonl" {
                                shouldInitV1 = true
                        }
                        if _, err := os.Stat(filepath.Join(app.Dir, "meta", "workspace.json")); err == nil {
                                shouldInitV1 = true
                        }
                        if _, err := os.Stat(filepath.Join(app.Dir, "events")); err == nil {
                                shouldInitV1 = true
                        }
                        if _, err := os.Stat(filepath.Join(app.Dir, ".git")); err == nil {
                                shouldInitV1 = true
                        }
                        if shouldInitV1 {
                                if res, err := store.EnsureGitBackedV1Layout(app.Dir); err == nil {
                                        v1Init = &res
                                }
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "dir":         app.Dir,
                                        "sqlitePath":  filepath.Join(app.Dir, ".clarity", "index.sqlite"),
                                        "gitBackedV1": v1Init,
                                },
                        })
                },
        }
        return cmd
}
