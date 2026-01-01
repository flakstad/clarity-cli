package store

import (
        "context"
        "errors"
        "strings"

        "clarity-cli/internal/gitrepo"
)

var ErrGitWriteBlocked = errors.New("workspace write blocked: git merge/rebase in progress (try: clarity sync resolve)")

func (s Store) ensureWritableForAppend(_ context.Context) error {
        // Only enforce this guard when JSONL is canonical (Git-backed flow).
        if s.eventLogBackend() != EventLogBackendJSONL {
                return nil
        }

        // Avoid invoking `git` on every write (hot path). A fast filesystem-based check
        // is sufficient for gating: while merge/rebase is in progress, we block writes.
        inp, err := gitrepo.DetectInProgress(s.workspaceRoot())
        if err != nil {
                return nil // best-effort
        }
        if !inp.InProgress {
                return nil
        }
        return ErrGitWriteBlocked
}

func isGitBlockedError(err error) bool {
        if err == nil {
                return false
        }
        return errors.Is(err, ErrGitWriteBlocked) || strings.Contains(err.Error(), "write blocked")
}

// IsGitWriteBlocked reports whether err indicates that Clarity write operations were blocked
// due to an in-progress Git merge/rebase (conflict gating for Git-backed workspaces).
func IsGitWriteBlocked(err error) bool {
        return isGitBlockedError(err)
}
