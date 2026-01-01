package gitrepo

import (
        "bufio"
        "errors"
        "os"
        "path/filepath"
        "strings"
)

// FindGitDir walks up from start and returns the git directory path (e.g. /repo/.git or linked gitdir).
// It does not invoke the git binary.
func FindGitDir(start string) (gitDir string, ok bool, err error) {
        dir := filepath.Clean(strings.TrimSpace(start))
        if dir == "" {
                return "", false, errors.New("empty start dir")
        }

        for {
                candidate := filepath.Join(dir, ".git")
                st, statErr := os.Stat(candidate)
                switch {
                case statErr == nil && st.IsDir():
                        return candidate, true, nil
                case statErr == nil && !st.IsDir():
                        // Worktrees/submodules can use a .git file pointing at the real gitdir.
                        target, err := readGitdirFile(candidate)
                        if err != nil {
                                return "", false, err
                        }
                        if target != "" {
                                return target, true, nil
                        }
                default:
                        // keep walking up
                }

                parent := filepath.Dir(dir)
                if parent == dir {
                        return "", false, nil
                }
                dir = parent
        }
}

func readGitdirFile(path string) (string, error) {
        f, err := os.Open(path)
        if err != nil {
                return "", err
        }
        defer f.Close()

        sc := bufio.NewScanner(f)
        for sc.Scan() {
                ln := strings.TrimSpace(sc.Text())
                if ln == "" {
                        continue
                }
                // Expected: "gitdir: /path/to/dir"
                if strings.HasPrefix(strings.ToLower(ln), "gitdir:") {
                        p := strings.TrimSpace(strings.TrimPrefix(ln, "gitdir:"))
                        if p == "" {
                                return "", nil
                        }
                        // Resolve relative paths relative to the .git file dir.
                        if !filepath.IsAbs(p) {
                                p = filepath.Join(filepath.Dir(path), p)
                        }
                        return filepath.Clean(p), nil
                }
                // Unexpected content; stop.
                break
        }
        if err := sc.Err(); err != nil {
                return "", err
        }
        return "", nil
}
