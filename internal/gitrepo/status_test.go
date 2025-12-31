package gitrepo

import (
        "context"
        "os"
        "os/exec"
        "path/filepath"
        "strings"
        "testing"
)

func TestGetStatus_NonRepo(t *testing.T) {
        st, err := GetStatus(context.Background(), t.TempDir())
        if err != nil {
                t.Fatalf("GetStatus: %v", err)
        }
        if st.IsRepo {
                t.Fatalf("expected non-repo status")
        }
}

func TestGetStatus_DirtyAndUnmerged(t *testing.T) {
        if _, err := exec.LookPath("git"); err != nil {
                t.Skip("git not installed")
        }

        ctx := context.Background()
        repo := t.TempDir()

        run(t, repo, "git", "init")
        run(t, repo, "git", "config", "user.email", "test@example.com")
        run(t, repo, "git", "config", "user.name", "Test")

        writeFile(t, filepath.Join(repo, "a.txt"), "base\n")
        run(t, repo, "git", "add", ".")
        run(t, repo, "git", "commit", "-m", "base")
        defaultBranch := strings.TrimSpace(runOut(t, repo, "git", "rev-parse", "--abbrev-ref", "HEAD"))
        if defaultBranch == "" {
                t.Fatalf("expected default branch")
        }

        st, err := GetStatus(ctx, repo)
        if err != nil {
                t.Fatalf("GetStatus: %v", err)
        }
        if !st.IsRepo || st.Dirty || st.Unmerged {
                t.Fatalf("unexpected clean status: %+v", st)
        }

        writeFile(t, filepath.Join(repo, "dirty.txt"), "x\n")
        st, err = GetStatus(ctx, repo)
        if err != nil {
                t.Fatalf("GetStatus (dirty): %v", err)
        }
        if !st.Dirty {
                t.Fatalf("expected dirty=true: %+v", st)
        }

        // Create a merge conflict to ensure unmerged detection works.
        run(t, repo, "git", "checkout", "-b", "feature")
        writeFile(t, filepath.Join(repo, "a.txt"), "feature\n")
        run(t, repo, "git", "add", "a.txt")
        run(t, repo, "git", "commit", "-m", "feature")

        run(t, repo, "git", "checkout", defaultBranch)
        writeFile(t, filepath.Join(repo, "a.txt"), "master\n")
        run(t, repo, "git", "add", "a.txt")
        run(t, repo, "git", "commit", "-m", "master")

        // This leaves the repo in a conflicted state.
        _ = exec.Command("git", "-C", repo, "merge", "feature").Run()

        st, err = GetStatus(ctx, repo)
        if err != nil {
                t.Fatalf("GetStatus (conflict): %v", err)
        }
        if !st.Unmerged {
                t.Fatalf("expected unmerged=true: %+v", st)
        }
}

func run(t *testing.T, dir string, bin string, args ...string) {
        t.Helper()
        cmd := exec.Command(bin, args...)
        cmd.Dir = dir
        out, err := cmd.CombinedOutput()
        if err != nil {
                t.Fatalf("%s %v: %v\n%s", bin, args, err, string(out))
        }
}

func runOut(t *testing.T, dir string, bin string, args ...string) string {
        t.Helper()
        cmd := exec.Command(bin, args...)
        cmd.Dir = dir
        out, err := cmd.CombinedOutput()
        if err != nil {
                t.Fatalf("%s %v: %v\n%s", bin, args, err, string(out))
        }
        return string(out)
}

func writeFile(t *testing.T, path string, body string) {
        t.Helper()
        if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
                t.Fatalf("write %s: %v", path, err)
        }
}
