package cli

import (
        "context"
        "errors"
        "os/exec"
        "strings"

        "clarity-cli/internal/gitrepo"
        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newWorkspaceMigrateCmd(app *App) *cobra.Command {
        var from string
        var to string
        var gitInit bool
        var gitCommit bool
        var message string

        cmd := &cobra.Command{
                Use:   "migrate",
                Short: "Migrate a legacy SQLite workspace into Git-backed JSONL v1 layout (one-shot)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        from = strings.TrimSpace(from)
                        to = strings.TrimSpace(to)
                        if from == "" || to == "" {
                                return writeErr(cmd, errors.New("missing --from/--to"))
                        }

                        res, err := store.MigrateSQLiteToGitBackedV1(context.Background(), from, to)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        if gitInit {
                                if st, err := gitrepo.GetStatus(context.Background(), to); err != nil {
                                        return writeErr(cmd, err)
                                } else if !st.IsRepo {
                                        c := exec.Command("git", "init")
                                        c.Dir = to
                                        if out, err := c.CombinedOutput(); err != nil {
                                                return writeErr(cmd, errors.New("git init: "+strings.TrimSpace(string(out))))
                                        }
                                }
                        }

                        committed := false
                        if gitCommit {
                                if st, err := gitrepo.GetStatus(context.Background(), to); err != nil {
                                        return writeErr(cmd, err)
                                } else if !st.IsRepo {
                                        return writeErr(cmd, errors.New("cannot commit: target dir is not a git repo (try: --git-init)"))
                                }

                                msg := strings.TrimSpace(message)
                                if msg == "" {
                                        msg = "clarity: migrate sqlite -> jsonl"
                                        if strings.TrimSpace(app.ActorID) != "" {
                                                msg += " (" + strings.TrimSpace(app.ActorID) + ")"
                                        }
                                }
                                ok, err := gitrepo.CommitWorkspaceCanonical(context.Background(), to, msg)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                committed = ok
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "migration":  res,
                                        "gitInited":  gitInit,
                                        "gitCommit":  gitCommit,
                                        "committed":  committed,
                                        "commitHint": "git -C " + to + " log -1 --oneline",
                                },
                                "_hints": []string{
                                        "clarity --dir " + to + " reindex",
                                        "clarity --dir " + to + " doctor --fail",
                                },
                        })
                },
        }

        cmd.Flags().StringVar(&from, "from", "", "Source legacy SQLite workspace directory")
        cmd.Flags().StringVar(&to, "to", "", "Target directory for Git-backed JSONL v1 workspace (must be empty)")
        cmd.Flags().BoolVar(&gitInit, "git-init", false, "Run `git init` in the target directory (optional)")
        cmd.Flags().BoolVar(&gitCommit, "git-commit", false, "Create an initial commit for canonical workspace files (optional)")
        cmd.Flags().StringVar(&message, "message", "", "Commit message (optional; used with --git-commit)")
        _ = cmd.MarkFlagRequired("from")
        _ = cmd.MarkFlagRequired("to")

        return cmd
}
