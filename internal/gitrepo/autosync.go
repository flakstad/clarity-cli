package gitrepo

import (
        "context"
        "strings"
)

type AutoSyncOpts struct {
        AutoPush       bool
        AutoPullRebase bool
}

func autoSyncOptsFromEnv() AutoSyncOpts {
        return AutoSyncOpts{
                AutoPush:       AutoPushEnabled(),
                AutoPullRebase: AutoPullRebaseEnabled(),
        }
}

// AutoCommitAndPush is a best-effort helper used by CLI/web to keep Git-backed workspaces synced
// after a successful mutation.
func AutoCommitAndPush(ctx context.Context, workspaceDir, actorLabel string) (committed bool, pushed bool, err error) {
        opts := autoSyncOptsFromEnv()

        committed, err = CommitWorkspaceCanonicalAuto(ctx, workspaceDir, actorLabel)
        if err != nil || !committed || !opts.AutoPush {
                return committed, false, err
        }

        st, stErr := GetStatus(ctx, workspaceDir)
        if stErr != nil || !st.IsRepo || st.Unmerged || st.InProgress || strings.TrimSpace(st.Upstream) == "" {
                return committed, false, nil
        }

        if err := Push(ctx, workspaceDir); err != nil {
                if opts.AutoPullRebase && IsNonFastForwardPushErr(err) {
                        _ = PullRebase(ctx, workspaceDir)
                        if err2 := Push(ctx, workspaceDir); err2 == nil {
                                return committed, true, nil
                        }
                }
                return committed, false, err
        }

        return committed, true, nil
}
