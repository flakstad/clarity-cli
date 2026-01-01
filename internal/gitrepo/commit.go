package gitrepo

import (
        "context"
        "errors"
        "fmt"
        "os"
        "os/exec"
        "path/filepath"
        "strings"
        "time"
)

// CommitWorkspaceCanonical stages and commits canonical Clarity workspace paths.
//
// It intentionally ignores derived/local-only files like `.clarity/index.sqlite` and `.clarity/device.json`.
// Returns committed=false when there's nothing to commit.
func CommitWorkspaceCanonical(ctx context.Context, workspaceDir string, message string) (committed bool, err error) {
        workspaceDir = filepath.Clean(workspaceDir)

        st, err := GetStatus(ctx, workspaceDir)
        if err != nil {
                return false, err
        }
        if !st.IsRepo {
                return false, nil
        }
        if st.Unmerged || st.InProgress {
                return false, errors.New("git repo has an in-progress merge/rebase; resolve first")
        }

        added, err := stageWorkspaceCanonical(ctx, workspaceDir, st.Root)
        if err != nil {
                return false, err
        }
        if !added {
                return false, nil
        }

        // Commit only if there's something staged.
        out, err := runGit(ctx, workspaceDir, "diff", "--cached", "--name-only")
        if err != nil {
                return false, err
        }
        if strings.TrimSpace(out) == "" {
                return false, nil
        }

        msg := strings.TrimSpace(message)
        if msg == "" {
                msg = fmt.Sprintf("clarity: update (%s)", time.Now().UTC().Format(time.RFC3339))
        }

        if _, err := runGit(ctx, workspaceDir, "commit", "-m", msg); err != nil {
                return false, err
        }
        return true, nil
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
        cmd := exec.CommandContext(ctx, "git", args...)
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

func stageWorkspaceCanonical(ctx context.Context, workspaceDir string, repoRoot string) (bool, error) {
        workspaceDir = filepath.Clean(workspaceDir)
        repoRoot = filepath.Clean(repoRoot)

        // On macOS, temp dirs may involve symlinks like /var -> /private/var. Git often
        // reports a canonicalized repo root, so normalize both sides before Rel() to avoid
        // "path is outside repository" errors.
        if v, err := filepath.EvalSymlinks(workspaceDir); err == nil {
                workspaceDir = v
        }
        if v, err := filepath.EvalSymlinks(repoRoot); err == nil {
                repoRoot = v
        }

        rel, err := filepath.Rel(repoRoot, workspaceDir)
        if err != nil {
                return false, err
        }
        rel = filepath.Clean(rel)

        type entry struct{ rel string }

        var targets []entry
        addIfExists := func(subRel string) {
                subRel = filepath.Clean(subRel)
                abs := filepath.Join(workspaceDir, subRel)
                if _, err := os.Stat(abs); err == nil {
                        if rel == "." {
                                targets = append(targets, entry{rel: subRel})
                        } else {
                                targets = append(targets, entry{rel: filepath.Join(rel, subRel)})
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
        _, err = runGit(ctx, repoRoot, args...)
        if err != nil {
                return false, err
        }
        return true, nil
}
