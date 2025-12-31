package cli

import (
        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newReindexCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "reindex",
                Short: "Rebuild derived local SQLite state from JSONL events",
                RunE: func(cmd *cobra.Command, args []string) error {
                        dir, err := resolveDir(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        res, err := store.ReplayEventsV1(dir)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        s := store.Store{Dir: dir}
                        if err := s.Save(res.DB); err != nil {
                                return writeErr(cmd, err)
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "dir":          dir,
                                        "applied":      res.AppliedCount,
                                        "skipped":      res.SkippedCount,
                                        "skippedTypes": res.SkippedTypes,
                                },
                                "meta": map[string]any{
                                        "actors":   len(res.DB.Actors),
                                        "projects": len(res.DB.Projects),
                                        "outlines": len(res.DB.Outlines),
                                        "items":    len(res.DB.Items),
                                        "deps":     len(res.DB.Deps),
                                        "comments": len(res.DB.Comments),
                                        "worklog":  len(res.DB.Worklog),
                                },
                                "_hints": []string{
                                        "clarity status",
                                        "clarity doctor --fail",
                                },
                        })
                },
        }

        return cmd
}
