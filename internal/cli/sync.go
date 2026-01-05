package cli

import (
        "context"
        "errors"
        "fmt"
        "os/exec"
        "path/filepath"
        "strings"

        "clarity-cli/internal/gitrepo"
        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newSyncCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "sync",
                Short: "Git sync helpers for Git-backed workspaces (v1)",
        }

        cmd.AddCommand(newSyncStatusCmd(app))
        cmd.AddCommand(newSyncRemotesCmd(app))
        cmd.AddCommand(newSyncSetupCmd(app))
        cmd.AddCommand(newSyncPullCmd(app))
        cmd.AddCommand(newSyncPushCmd(app))
        cmd.AddCommand(newSyncResolveCmd(app))
        return cmd
}

func newSyncSetupCmd(app *App) *cobra.Command {
        var remoteURL string
        var remoteName string
        var push bool
        var commit bool
        var message string

        cmd := &cobra.Command{
                Use:   "setup",
                Short: "Initialize Git + optionally set remote + push (recommended for teams)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        dir, err := resolveDir(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        ctx := context.Background()

                        // Ensure Clarity ignores exist (derived/local-only).
                        if _, err := store.EnsureGitignoreHasClarityIgnores(filepath.Join(dir, ".gitignore")); err != nil {
                                return writeErr(cmd, err)
                        }

                        before, err := gitrepo.GetStatus(ctx, dir)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if !before.IsRepo {
                                if err := gitrepo.Init(ctx, dir); err != nil {
                                        return writeErr(cmd, err)
                                }
                        }

                        afterInit, err := gitrepo.GetStatus(ctx, dir)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if !afterInit.IsRepo {
                                return writeErr(cmd, errors.New("sync setup: failed to initialize git repo"))
                        }
                        if afterInit.Unmerged || afterInit.InProgress {
                                return writeErr(cmd, errors.New("sync setup: repo has an in-progress merge/rebase; resolve first"))
                        }

                        committed := false
                        if commit {
                                committed, err = gitrepo.CommitWorkspaceCanonical(ctx, dir, message)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                        }

                        remoteSet := false
                        if strings.TrimSpace(remoteURL) != "" {
                                if err := gitrepo.SetRemoteURL(ctx, dir, remoteName, remoteURL); err != nil {
                                        return writeErr(cmd, err)
                                }
                                remoteSet = true
                        }

                        pushed := false
                        if push {
                                st, err := gitrepo.GetStatus(ctx, dir)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                // Only attempt push when a remote exists (either already configured or newly set).
                                if remoteSet || strings.TrimSpace(st.Upstream) != "" {
                                        branch := st.Branch
                                        if strings.TrimSpace(branch) == "" || strings.TrimSpace(branch) == "HEAD" {
                                                branch = "HEAD"
                                        }
                                        if err := gitrepo.PushSetUpstream(ctx, dir, remoteName, branch); err != nil {
                                                return writeErr(cmd, err)
                                        }
                                        pushed = true
                                }
                        }

                        final, _ := gitrepo.GetStatus(ctx, dir)
                        hints := []string{"git status"}
                        if strings.TrimSpace(remoteURL) == "" && strings.TrimSpace(final.Upstream) == "" {
                                hints = append(hints, "git remote add origin <url>")
                        }
                        if strings.TrimSpace(final.Upstream) == "" {
                                hints = append(hints, "git push -u origin HEAD")
                        }
                        hints = append(hints, "clarity sync status", "clarity reindex", "clarity doctor --fail")

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "before":    before,
                                        "afterInit": afterInit,
                                        "final":     final,
                                        "committed": committed,
                                        "remoteSet": remoteSet,
                                        "pushed":    pushed,
                                },
                                "_hints": hints,
                        })
                },
        }

        cmd.Flags().StringVar(&remoteURL, "remote-url", "", "Remote URL (if set, configures origin)")
        cmd.Flags().StringVar(&remoteName, "remote-name", "origin", "Remote name (default: origin)")
        cmd.Flags().BoolVar(&push, "push", false, "After setup, run `git push -u` (requires remote)")
        cmd.Flags().BoolVar(&commit, "commit", true, "Create an initial commit (recommended)")
        cmd.Flags().StringVar(&message, "message", "", "Commit message for initial commit (optional)")

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

func newSyncRemotesCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "remotes",
                Short: "List configured Git remotes for the current workspace directory",
                RunE: func(cmd *cobra.Command, args []string) error {
                        dir, err := resolveDir(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        ctx := context.Background()
                        st, err := gitrepo.GetStatus(ctx, dir)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if !st.IsRepo {
                                return writeErr(cmd, errors.New("sync remotes: not a git repository"))
                        }

                        remotes, err := gitrepo.ListRemotes(ctx, dir)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "dir":      dir,
                                        "branch":   st.Branch,
                                        "upstream": st.Upstream,
                                        "remotes":  remotes,
                                },
                                "_hints": []string{
                                        "git remote -v",
                                        "clarity sync status",
                                },
                        })
                },
        }
        return cmd
}

func newSyncPullCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "pull",
                Short: "Pull (rebase) the workspace repo (safe-by-default)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        dir, err := resolveDir(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        before, err := gitrepo.GetStatus(context.Background(), dir)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if !before.IsRepo {
                                return writeErr(cmd, errors.New("sync pull: not a git repository"))
                        }
                        if before.Unmerged || before.InProgress {
                                return writeErr(cmd, errors.New("sync pull: repo has an in-progress merge/rebase; resolve first (try: clarity sync resolve)"))
                        }
                        if before.DirtyTracked {
                                return writeErr(cmd, errors.New("sync pull: repo has local changes; commit/push first (try: clarity sync push)"))
                        }

                        if _, err := runGit(dir, "pull", "--rebase"); err != nil {
                                return writeErr(cmd, err)
                        }

                        after, _ := gitrepo.GetStatus(context.Background(), dir)
                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "before": before,
                                        "after":  after,
                                },
                                "_hints": []string{
                                        "clarity reindex",
                                        "clarity doctor --fail",
                                },
                        })
                },
        }
        return cmd
}

func newSyncPushCmd(app *App) *cobra.Command {
        var message string
        var doPull bool

        cmd := &cobra.Command{
                Use:   "push",
                Short: "Stage+commit workspace changes and push (safe-by-default)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        dir, err := resolveDir(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        ctx := context.Background()
                        before, err := gitrepo.GetStatus(ctx, dir)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if !before.IsRepo {
                                return writeErr(cmd, errors.New("sync push: not a git repository"))
                        }
                        if before.Unmerged || before.InProgress {
                                return writeErr(cmd, errors.New("sync push: repo has an in-progress merge/rebase; resolve first (try: clarity sync resolve)"))
                        }

                        committed := false
                        commitMsg := strings.TrimSpace(message)
                        if commitMsg != "" {
                                committed, err = gitrepo.CommitWorkspaceCanonical(ctx, dir, commitMsg)
                        } else {
                                actorLabel := guessActorLabelForDir(app, dir)
                                committed, err = gitrepo.CommitWorkspaceCanonicalAuto(ctx, dir, actorLabel)
                        }
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        // Pull/rebase (optional) then push.
                        pulled := false
                        if doPull {
                                cur, err := gitrepo.GetStatus(ctx, dir)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                if cur.Unmerged || cur.InProgress {
                                        return writeErr(cmd, errors.New("sync push: repo entered conflict state; resolve first (try: clarity sync resolve)"))
                                }
                                if cur.DirtyTracked {
                                        return writeErr(cmd, errors.New("sync push: repo has local changes after commit; resolve manually"))
                                }
                                if strings.TrimSpace(cur.Upstream) != "" {
                                        if _, err := runGit(dir, "pull", "--rebase"); err != nil {
                                                return writeErr(cmd, err)
                                        }
                                        pulled = true
                                }
                        }

                        pushed := false
                        if _, err := runGit(dir, "push"); err == nil {
                                pushed = true
                        } else if doPull && !pulled && isNonFastForwardPushErr(err) {
                                // Retry once: pull --rebase + push.
                                if _, pullErr := runGit(dir, "pull", "--rebase"); pullErr != nil {
                                        return writeErr(cmd, pullErr)
                                }
                                pulled = true
                                if _, pushErr := runGit(dir, "push"); pushErr != nil {
                                        return writeErr(cmd, pushErr)
                                }
                                pushed = true
                        } else {
                                return writeErr(cmd, err)
                        }

                        after, _ := gitrepo.GetStatus(ctx, dir)
                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "before":    before,
                                        "after":     after,
                                        "committed": committed,
                                        "pulled":    pulled,
                                        "pushed":    pushed,
                                },
                        })
                },
        }

        cmd.Flags().StringVar(&message, "message", "", "Commit message (optional)")
        cmd.Flags().BoolVar(&doPull, "pull", true, "Pull --rebase before pushing (recommended)")

        return cmd
}

func newSyncResolveCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "resolve",
                Short: "Show conflict status and suggested resolution steps",
                RunE: func(cmd *cobra.Command, args []string) error {
                        dir, err := resolveDir(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        st, err := gitrepo.GetStatus(context.Background(), dir)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if !st.IsRepo {
                                return writeErr(cmd, errors.New("sync resolve: not a git repository"))
                        }

                        hints := []string{"git status"}
                        if st.InProgressKind == "merge" {
                                hints = append(hints, "git merge --abort")
                        }
                        if st.InProgressKind == "rebase" {
                                hints = append(hints, "git rebase --abort")
                        }
                        hints = append(hints, "clarity reindex", "clarity doctor --fail")

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "status": st,
                                        "note":   "Resolve Git conflicts first; Clarity blocks writes while merge/rebase is in progress.",
                                },
                                "_hints": hints,
                        })
                },
        }
        return cmd
}

func guessActorLabelForDir(app *App, dir string) string {
        // Best-effort: use current actor name if available; do not hard-fail sync operations.
        dir = strings.TrimSpace(dir)
        if dir == "" {
                return ""
        }
        s := store.Store{Dir: dir}
        db, err := s.Load()
        if err != nil || db == nil {
                return strings.TrimSpace(app.ActorID)
        }
        actorID := strings.TrimSpace(app.ActorID)
        if actorID == "" {
                actorID = strings.TrimSpace(db.CurrentActorID)
        }
        if actorID == "" {
                return ""
        }
        if a, ok := db.FindActor(actorID); ok {
                if strings.TrimSpace(a.Name) != "" {
                        return strings.TrimSpace(a.Name)
                }
        }
        return actorID
}

func runGit(dir string, args ...string) (string, error) {
        cmd := execCommand("git", args...)
        cmd.Dir = dir
        out, err := cmd.CombinedOutput()
        if err != nil {
                msg := strings.TrimSpace(string(out))
                if msg == "" {
                        msg = err.Error()
                }
                return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
        }
        return string(out), nil
}

// execCommand is a small seam for tests (if needed later).
var execCommand = func(name string, args ...string) *exec.Cmd {
        return exec.Command(name, args...)
}

func isNonFastForwardPushErr(err error) bool {
        if err == nil {
                return false
        }
        msg := strings.ToLower(err.Error())
        for _, needle := range []string{
                "non-fast-forward",
                "fetch first",
                "rejected",
                "updates were rejected",
        } {
                if strings.Contains(msg, needle) {
                        return true
                }
        }
        return false
}
