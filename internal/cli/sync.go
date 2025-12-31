package cli

import (
        "context"
        "errors"
        "fmt"
        "os"
        "os/exec"
        "path/filepath"
        "strings"
        "time"

        "clarity-cli/internal/gitrepo"

        "github.com/spf13/cobra"
)

func newSyncCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "sync",
                Short: "Git sync helpers for Git-backed workspaces (v1)",
        }

        cmd.AddCommand(newSyncStatusCmd(app))
        cmd.AddCommand(newSyncPullCmd(app))
        cmd.AddCommand(newSyncPushCmd(app))
        cmd.AddCommand(newSyncResolveCmd(app))
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

                        // Stage canonical workspace paths (ignore derived files like .clarity/index.sqlite).
                        added, err := stageWorkspaceCanonical(dir, before.Root)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        committed := false
                        commitMsg := strings.TrimSpace(message)
                        if commitMsg == "" {
                                commitMsg = fmt.Sprintf("clarity: update (%s)", time.Now().UTC().Format(time.RFC3339))
                                if strings.TrimSpace(app.ActorID) != "" {
                                        commitMsg = fmt.Sprintf("clarity: %s update (%s)", strings.TrimSpace(app.ActorID), time.Now().UTC().Format(time.RFC3339))
                                }
                        }

                        if added {
                                // Commit only if there's something staged.
                                if out, err := runGit(dir, "diff", "--cached", "--name-only"); err == nil && strings.TrimSpace(out) != "" {
                                        if _, err := runGit(dir, "commit", "-m", commitMsg); err != nil {
                                                return writeErr(cmd, err)
                                        }
                                        committed = true
                                }
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
                        } else {
                                // If we couldn't push (no remote, offline, auth), still return state details.
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

func stageWorkspaceCanonical(workspaceDir string, repoRoot string) (bool, error) {
        workspaceDir = filepath.Clean(workspaceDir)
        repoRoot = filepath.Clean(repoRoot)

        rel, err := filepath.Rel(repoRoot, workspaceDir)
        if err != nil {
                return false, err
        }
        rel = filepath.Clean(rel)

        type entry struct {
                abs string
                rel string
        }

        var targets []entry
        addIfExists := func(subRel string) {
                subRel = filepath.Clean(subRel)
                abs := filepath.Join(workspaceDir, subRel)
                if _, err := os.Stat(abs); err == nil {
                        if rel == "." {
                                targets = append(targets, entry{abs: abs, rel: subRel})
                        } else {
                                targets = append(targets, entry{abs: abs, rel: filepath.Join(rel, subRel)})
                        }
                }
        }

        addIfExists("events")
        addIfExists(filepath.Join("meta", "workspace.json"))
        addIfExists("resources")

        if len(targets) == 0 {
                return false, nil
        }

        args := []string{"-C", repoRoot, "add", "--"}
        for _, t := range targets {
                args = append(args, t.rel)
        }
        _, err = runGit(repoRoot, args...)
        if err != nil {
                return false, err
        }
        return true, nil
}
