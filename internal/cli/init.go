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

                        // Bootstrap the JSONL v1 workspace layout (events/ + meta/workspace.json).
                        //
                        // Git is optional: Clarity can use the v1 event log + derived SQLite state
                        // even when `git` isn't installed. Sync commands require Git, but the core
                        // storage model does not.
                        //
                        // This is intentionally best-effort; even if this fails, the SQLite-based
                        // derived state remains usable.
                        var v1Init *store.GitBackedV1InitResult
                        backend := strings.ToLower(strings.TrimSpace(os.Getenv("CLARITY_EVENTLOG")))
                        // Don't write v1 workspace files for the legacy "store root is .clarity/" mode.
                        // This mode is used for fixtures/tests or isolated experiments.
                        isClarityDirStore := filepath.Base(filepath.Clean(app.Dir)) == ".clarity"
                        if backend != "sqlite" && !isClarityDirStore {
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
