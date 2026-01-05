package gitrepo

import (
        "bytes"
        "context"
        "errors"
        "fmt"
        "os/exec"
        "strconv"
        "strings"
)

type Status struct {
        IsRepo bool `json:"isRepo"`

        Root string `json:"root,omitempty"`

        Branch   string `json:"branch,omitempty"`
        Upstream string `json:"upstream,omitempty"`

        // UpstreamRemote is derived from Upstream (e.g. origin from origin/main).
        UpstreamRemote string `json:"upstreamRemote,omitempty"`
        // UpstreamRemoteURL is the configured fetch URL for UpstreamRemote (best-effort).
        UpstreamRemoteURL string `json:"upstreamRemoteURL,omitempty"`

        Head string `json:"head,omitempty"`

        Dirty bool `json:"dirty"`
        // DirtyTracked is like Dirty, but ignores untracked files (??).
        // This is typically what you want for safe pull/rebase operations, since Clarity's
        // derived SQLite index is local-only and often untracked.
        DirtyTracked bool `json:"dirtyTracked"`
        Unmerged     bool `json:"unmerged"`

        InProgress     bool   `json:"inProgress"`
        InProgressKind string `json:"inProgressKind,omitempty"` // merge|rebase|cherry-pick|revert

        Ahead  int `json:"ahead,omitempty"`
        Behind int `json:"behind,omitempty"`
}

func GetStatus(ctx context.Context, dir string) (Status, error) {
        root, err := git(ctx, dir, "rev-parse", "--show-toplevel")
        if err != nil {
                // "not a git repository" is common; treat as non-repo rather than error.
                return Status{IsRepo: false}, nil
        }
        root = strings.TrimSpace(root)
        if root == "" {
                return Status{}, errors.New("git rev-parse returned empty root")
        }

        branch, _ := git(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
        head, _ := git(ctx, dir, "rev-parse", "--short", "HEAD")
        upstream, _ := git(ctx, dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")

        upstreamRemote := ""
        upstreamRemoteURL := ""
        if u := strings.TrimSpace(upstream); u != "" {
                // rev-parse @{u} usually returns "<remote>/<branch>" (e.g. "origin/main").
                if remote, _, ok := strings.Cut(u, "/"); ok {
                        upstreamRemote = strings.TrimSpace(remote)
                        if upstreamRemote != "" {
                                if url, err := RemoteURL(ctx, dir, upstreamRemote); err == nil {
                                        upstreamRemoteURL = strings.TrimSpace(url)
                                }
                        }
                }
        }

        porcelain, _ := git(ctx, dir, "status", "--porcelain=v1")
        dirty, unmerged := parsePorcelain(porcelain)
        porcelainTracked, _ := git(ctx, dir, "status", "--porcelain=v1", "--untracked-files=no")
        dirtyTracked, _ := parsePorcelain(porcelainTracked)

        inProgress, inProgressKind := detectInProgress(ctx, dir)

        ahead, behind := 0, 0
        if strings.TrimSpace(upstream) != "" {
                if counts, err := git(ctx, dir, "rev-list", "--left-right", "--count", "HEAD...@{u}"); err == nil {
                        a, b, ok := parseAheadBehind(counts)
                        if ok {
                                ahead, behind = a, b
                        }
                }
        }

        return Status{
                IsRepo: true,
                Root:   strings.TrimSpace(root),

                Branch:            strings.TrimSpace(branch),
                Upstream:          strings.TrimSpace(upstream),
                UpstreamRemote:    upstreamRemote,
                UpstreamRemoteURL: upstreamRemoteURL,
                Head:              strings.TrimSpace(head),

                Dirty:        dirty,
                DirtyTracked: dirtyTracked,
                Unmerged:     unmerged,

                InProgress:     inProgress,
                InProgressKind: inProgressKind,

                Ahead:  ahead,
                Behind: behind,
        }, nil
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
        cmd := exec.CommandContext(ctx, "git", args...)
        cmd.Dir = dir

        var stdout bytes.Buffer
        var stderr bytes.Buffer
        cmd.Stdout = &stdout
        cmd.Stderr = &stderr

        if err := cmd.Run(); err != nil {
                msg := strings.TrimSpace(stderr.String())
                if msg == "" {
                        msg = err.Error()
                }
                return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
        }
        return stdout.String(), nil
}

func parsePorcelain(out string) (dirty bool, unmerged bool) {
        lines := strings.Split(out, "\n")
        for _, ln := range lines {
                ln = strings.TrimRight(ln, "\r")
                if len(ln) < 2 {
                        continue
                }
                xy := ln[:2]
                if strings.TrimSpace(xy) == "" {
                        continue
                }
                dirty = true
                if isUnmergedXY(xy) {
                        unmerged = true
                }
        }
        return dirty, unmerged
}

func detectInProgress(ctx context.Context, dir string) (bool, string) {
        switch {
        case gitRefExists(ctx, dir, "MERGE_HEAD"):
                return true, "merge"
        case gitRefExists(ctx, dir, "REBASE_HEAD"):
                return true, "rebase"
        case gitRefExists(ctx, dir, "CHERRY_PICK_HEAD"):
                return true, "cherry-pick"
        case gitRefExists(ctx, dir, "REVERT_HEAD"):
                return true, "revert"
        default:
                return false, ""
        }
}

func gitRefExists(ctx context.Context, dir string, ref string) bool {
        ref = strings.TrimSpace(ref)
        if ref == "" {
                return false
        }
        cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "-q", ref)
        cmd.Dir = dir
        return cmd.Run() == nil
}

func isUnmergedXY(xy string) bool {
        if len(xy) != 2 {
                return false
        }
        switch xy {
        case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
                return true
        }
        // "U?" and "?U" variants.
        return xy[0] == 'U' || xy[1] == 'U'
}

func parseAheadBehind(out string) (ahead int, behind int, ok bool) {
        // git rev-list --left-right --count HEAD...@{u}
        // => "<ahead>\t<behind>\n"
        out = strings.TrimSpace(out)
        fields := strings.Fields(out)
        if len(fields) != 2 {
                return 0, 0, false
        }
        a, err1 := strconv.Atoi(fields[0])
        b, err2 := strconv.Atoi(fields[1])
        if err1 != nil || err2 != nil {
                return 0, 0, false
        }
        return a, b, true
}
