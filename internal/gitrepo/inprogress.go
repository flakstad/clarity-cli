package gitrepo

import (
        "os"
        "path/filepath"
)

type InProgress struct {
        InProgress bool   `json:"inProgress"`
        Kind       string `json:"kind,omitempty"` // merge|rebase|cherry-pick|revert
}

// DetectInProgress checks for merge/rebase state by looking for marker files in .git.
// This is a fast, best-effort check meant for gating Clarity writes.
func DetectInProgress(dir string) (InProgress, error) {
        gitDir, ok, err := FindGitDir(dir)
        if err != nil || !ok {
                return InProgress{}, err
        }

        // Merge in progress (including conflicts).
        if exists(filepath.Join(gitDir, "MERGE_HEAD")) {
                return InProgress{InProgress: true, Kind: "merge"}, nil
        }

        // Rebase in progress (including conflicts).
        if exists(filepath.Join(gitDir, "rebase-apply")) || exists(filepath.Join(gitDir, "rebase-merge")) {
                return InProgress{InProgress: true, Kind: "rebase"}, nil
        }

        if exists(filepath.Join(gitDir, "CHERRY_PICK_HEAD")) {
                return InProgress{InProgress: true, Kind: "cherry-pick"}, nil
        }
        if exists(filepath.Join(gitDir, "REVERT_HEAD")) {
                return InProgress{InProgress: true, Kind: "revert"}, nil
        }

        return InProgress{InProgress: false}, nil
}

func exists(path string) bool {
        _, err := os.Stat(path)
        return err == nil
}
