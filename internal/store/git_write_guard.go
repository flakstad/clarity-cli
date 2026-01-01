package store

import (
	"context"
	"errors"
	"strings"

	"clarity-cli/internal/gitrepo"
)

var ErrGitWriteBlocked = errors.New("workspace write blocked: git merge/rebase in progress (try: clarity sync resolve)")

func (s Store) ensureWritableForAppend(ctx context.Context) error {
	// Only enforce this guard when JSONL is canonical (Git-backed flow).
	if s.eventLogBackend() != EventLogBackendJSONL {
		return nil
	}

	st, err := gitrepo.GetStatus(ctx, s.workspaceRoot())
	if err != nil {
		// Best-effort: don't block writes due to git tooling issues.
		return nil
	}
	if !st.IsRepo {
		return nil
	}
	if st.Unmerged || st.InProgress {
		return ErrGitWriteBlocked
	}
	return nil
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
