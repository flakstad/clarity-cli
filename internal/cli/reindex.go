package cli

import (
        "context"
        "strings"

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

                        s := store.Store{Dir: dir}

                        // Preserve local UI/session meta across reindex.
                        // This meta is not part of the canonical event stream, so it must be carried forward
                        // from the existing derived state.
                        prevActorID := ""
                        prevProjectID := ""
                        if existing, err := s.LoadSQLite(context.Background()); err == nil && existing != nil {
                                prevActorID = strings.TrimSpace(existing.CurrentActorID)
                                prevProjectID = strings.TrimSpace(existing.CurrentProjectID)
                        }

                        res, err := store.ReplayEventsV1(dir)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        if strings.TrimSpace(prevActorID) != "" {
                                if _, ok := res.DB.FindActor(prevActorID); ok {
                                        res.DB.CurrentActorID = prevActorID
                                }
                        }
                        if strings.TrimSpace(res.DB.CurrentActorID) == "" {
                                // First-time bootstrap convenience: if there's exactly one human actor (common for
                                // personal workspaces and migrations), pick it.
                                pick := ""
                                for _, a := range res.DB.Actors {
                                        if a.Kind == "human" && strings.TrimSpace(a.ID) != "" {
                                                pick = strings.TrimSpace(a.ID)
                                                break
                                        }
                                }
                                if pick == "" && len(res.DB.Actors) > 0 {
                                        pick = strings.TrimSpace(res.DB.Actors[0].ID)
                                }
                                res.DB.CurrentActorID = pick
                        }

                        if strings.TrimSpace(prevProjectID) != "" {
                                if _, ok := res.DB.FindProject(prevProjectID); ok {
                                        res.DB.CurrentProjectID = prevProjectID
                                }
                        }
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
