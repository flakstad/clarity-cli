package cli

import (
        "context"
        "errors"
        "os/exec"
        "path/filepath"
        "strings"
        "time"

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
        var register bool
        var use bool
        var name string

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

                        absTo, err := filepath.Abs(to)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        absTo = filepath.Clean(absTo)

                        registered := false
                        if register || use || strings.TrimSpace(name) != "" {
                                if strings.TrimSpace(name) == "" {
                                        // Default: carry over the name of the source directory (best-effort).
                                        name = filepath.Base(filepath.Clean(from))
                                }
                                nm, err := store.NormalizeWorkspaceName(name)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                cfg, err := store.LoadConfig()
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                if cfg.Workspaces == nil {
                                        cfg.Workspaces = map[string]store.WorkspaceRef{}
                                }
                                cfg.Workspaces[nm] = store.WorkspaceRef{
                                        Path:       absTo,
                                        Kind:       "git",
                                        LastOpened: time.Now().UTC().Format(time.RFC3339Nano),
                                }
                                if use {
                                        cfg.CurrentWorkspace = nm
                                        app.Workspace = nm
                                        app.Dir = absTo
                                }
                                if err := store.SaveConfig(cfg); err != nil {
                                        return writeErr(cmd, err)
                                }
                                registered = true
                                name = nm
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "migration":  res,
                                        "gitInited":  gitInit,
                                        "gitCommit":  gitCommit,
                                        "committed":  committed,
                                        "registered": registered,
                                        "name":       strings.TrimSpace(name),
                                        "used":       use,
                                        "commitHint": "git -C " + absTo + " log -1 --oneline",
                                },
                                "_hints": []string{
                                        "clarity --dir " + absTo + " reindex",
                                        "clarity --dir " + absTo + " doctor --fail",
                                },
                        })
                },
        }

        cmd.Flags().StringVar(&from, "from", "", "Source legacy SQLite workspace directory")
        cmd.Flags().StringVar(&to, "to", "", "Target directory for Git-backed JSONL v1 workspace (must be empty)")
        cmd.Flags().BoolVar(&gitInit, "git-init", false, "Run `git init` in the target directory (optional)")
        cmd.Flags().BoolVar(&gitCommit, "git-commit", false, "Create an initial commit for canonical workspace files (optional)")
        cmd.Flags().StringVar(&message, "message", "", "Commit message (optional; used with --git-commit)")
        cmd.Flags().BoolVar(&register, "register", false, "Register the migrated workspace in ~/.clarity/config.json")
        cmd.Flags().BoolVar(&use, "use", false, "Also set the migrated workspace as current (implies --register)")
        cmd.Flags().StringVar(&name, "name", "", "Workspace name to register (optional; used with --register/--use)")
        _ = cmd.MarkFlagRequired("from")
        _ = cmd.MarkFlagRequired("to")

        return cmd
}
