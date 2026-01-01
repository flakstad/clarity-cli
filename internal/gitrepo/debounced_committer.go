package gitrepo

import (
        "context"
        "strings"
        "sync"
        "time"
)

type DebouncedCommitter struct {
        workspaceDir string
        debounce     time.Duration
        autoPush     bool
        autoPull     bool

        mu      sync.Mutex
        timer   *time.Timer
        pending bool
        running bool

        lastActorLabel string
}

type DebouncedCommitterOpts struct {
        WorkspaceDir string
        Debounce     time.Duration

        // AutoPush enables best-effort `git push` after committing when an upstream exists.
        AutoPush bool
        // AutoPullRebase enables a best-effort `git pull --rebase` retry on non-fast-forward push errors.
        AutoPullRebase bool
}

func NewDebouncedCommitter(opts DebouncedCommitterOpts) *DebouncedCommitter {
        debounce := opts.Debounce
        if debounce <= 0 {
                debounce = 2 * time.Second
        }
        return &DebouncedCommitter{
                workspaceDir: opts.WorkspaceDir,
                debounce:     debounce,
                autoPush:     opts.AutoPush,
                autoPull:     opts.AutoPullRebase,
        }
}

func (d *DebouncedCommitter) Notify(actorLabel string) {
        if d == nil {
                return
        }

        d.mu.Lock()
        d.pending = true
        d.lastActorLabel = actorLabel
        if d.timer == nil {
                d.timer = time.AfterFunc(d.debounce, d.onTimer)
                d.mu.Unlock()
                return
        }
        d.timer.Reset(d.debounce)
        d.mu.Unlock()
}

func (d *DebouncedCommitter) onTimer() {
        d.mu.Lock()
        if d.running {
                // Another run is in-flight; schedule again to pick up pending changes.
                if d.timer != nil {
                        d.timer.Reset(d.debounce)
                }
                d.mu.Unlock()
                return
        }
        if !d.pending {
                d.mu.Unlock()
                return
        }
        d.pending = false
        d.running = true
        actorLabel := d.lastActorLabel
        d.mu.Unlock()

        // Best-effort: commit canonical paths if possible. Errors are intentionally dropped;
        // the user can always run `clarity sync push` / `git status` manually.
        ctx := context.Background()
        committed, _ := CommitWorkspaceCanonicalAuto(ctx, d.workspaceDir, actorLabel)
        if committed && d.autoPush {
                st, err := GetStatus(ctx, d.workspaceDir)
                if err == nil && st.IsRepo && !st.Unmerged && !st.InProgress && strings.TrimSpace(st.Upstream) != "" {
                        if err := Push(ctx, d.workspaceDir); err != nil && d.autoPull && IsNonFastForwardPushErr(err) {
                                _ = PullRebase(ctx, d.workspaceDir)
                                _ = Push(ctx, d.workspaceDir)
                        }
                }
        }

        d.mu.Lock()
        d.running = false
        // If another Notify happened while running, schedule another run.
        if d.pending && d.timer != nil {
                d.timer.Reset(d.debounce)
        }
        d.mu.Unlock()
}
