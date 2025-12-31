package cli

import (
        "os"
        "os/exec"
        "path/filepath"
        "strings"
        "testing"
)

func TestStageWorkspaceCanonical_StagesOnlyCanonicalPaths(t *testing.T) {
        if _, err := exec.LookPath("git"); err != nil {
                t.Skip("git not installed")
        }

        repoRoot := t.TempDir()
        runGitCmd(t, repoRoot, "git", "init")
        runGitCmd(t, repoRoot, "git", "config", "user.email", "test@example.com")
        runGitCmd(t, repoRoot, "git", "config", "user.name", "Test")

        workspaceDir := filepath.Join(repoRoot, "clarity")
        mustMkdirAll(t, filepath.Join(workspaceDir, "events"))
        mustMkdirAll(t, filepath.Join(workspaceDir, "meta"))
        mustMkdirAll(t, filepath.Join(workspaceDir, "resources"))
        mustMkdirAll(t, filepath.Join(workspaceDir, ".clarity"))

        mustWriteFile(t, filepath.Join(workspaceDir, "events", "events.r1.jsonl"), []byte("{}\n"))
        mustWriteFile(t, filepath.Join(workspaceDir, "meta", "workspace.json"), []byte("{\"workspaceId\":\"w\"}\n"))
        mustWriteFile(t, filepath.Join(workspaceDir, "resources", "note.txt"), []byte("x\n"))
        mustWriteFile(t, filepath.Join(workspaceDir, ".clarity", "index.sqlite"), []byte("sqlite\n"))

        staged, err := stageWorkspaceCanonical(workspaceDir, repoRoot)
        if err != nil {
                t.Fatalf("stageWorkspaceCanonical: %v", err)
        }
        if !staged {
                t.Fatalf("expected staged=true")
        }

        out := runGitCmdOut(t, repoRoot, "git", "diff", "--cached", "--name-only")
        lines := nonEmptyLines(out)

        want := map[string]bool{
                filepath.ToSlash(filepath.Join("clarity", "events", "events.r1.jsonl")): true,
                filepath.ToSlash(filepath.Join("clarity", "meta", "workspace.json")):    true,
                filepath.ToSlash(filepath.Join("clarity", "resources", "note.txt")):     true,
        }
        for _, ln := range lines {
                if strings.Contains(ln, ".clarity/") {
                        t.Fatalf("staged derived file unexpectedly: %q", ln)
                }
                if !want[ln] {
                        t.Fatalf("unexpected staged path: %q", ln)
                }
                delete(want, ln)
        }
        if len(want) != 0 {
                t.Fatalf("missing staged paths: %v", keys(want))
        }
}

func runGitCmd(t *testing.T, dir string, bin string, args ...string) {
        t.Helper()
        cmd := exec.Command(bin, args...)
        cmd.Dir = dir
        out, err := cmd.CombinedOutput()
        if err != nil {
                t.Fatalf("%s %v: %v\n%s", bin, args, err, string(out))
        }
}

func runGitCmdOut(t *testing.T, dir string, bin string, args ...string) string {
        t.Helper()
        cmd := exec.Command(bin, args...)
        cmd.Dir = dir
        out, err := cmd.CombinedOutput()
        if err != nil {
                t.Fatalf("%s %v: %v\n%s", bin, args, err, string(out))
        }
        return string(out)
}

func mustMkdirAll(t *testing.T, path string) {
        t.Helper()
        if err := os.MkdirAll(path, 0o755); err != nil {
                t.Fatalf("mkdir %s: %v", path, err)
        }
}

func mustWriteFile(t *testing.T, path string, b []byte) {
        t.Helper()
        if err := os.WriteFile(path, b, 0o644); err != nil {
                t.Fatalf("write %s: %v", path, err)
        }
}

func nonEmptyLines(s string) []string {
        var out []string
        for _, ln := range strings.Split(s, "\n") {
                ln = strings.TrimSpace(ln)
                if ln == "" {
                        continue
                }
                out = append(out, ln)
        }
        return out
}

func keys(m map[string]bool) []string {
        var out []string
        for k := range m {
                out = append(out, k)
        }
        return out
}
