package cli

import (
        "context"

        "clarity-cli/internal/gitrepo"

        "github.com/spf13/cobra"
)

func newSyncCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "sync",
                Short: "Git sync helpers for Git-backed workspaces (v1)",
        }

        cmd.AddCommand(newSyncStatusCmd(app))
        return cmd
}

func newSyncStatusCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "status",
                Short: "Show Git working tree + upstream status for the current workspace dir",
                RunE: func(cmd *cobra.Command, args []string) error {
                        dir, err := resolveDir(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        st, err := gitrepo.GetStatus(context.Background(), dir)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        hints := []string{}
                        if st.IsRepo && (st.Dirty || st.Unmerged) {
                                hints = append(hints, "git status")
                        }
                        if st.IsRepo && st.Behind > 0 {
                                hints = append(hints, "git pull --rebase")
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data":   st,
                                "_hints": hints,
                        })
                },
        }

        return cmd
}
