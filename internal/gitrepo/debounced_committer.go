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

        lastAt        time.Time
        lastCommitted bool
        lastPushed    bool
        lastErr       string
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

type DebouncedCommitterStatus struct {
        Pending bool
        Running bool

        LastAt        time.Time
        LastCommitted bool
        LastPushed    bool
        LastError     string
}

func (d *DebouncedCommitter) Status() DebouncedCommitterStatus {
        if d == nil {
                return DebouncedCommitterStatus{}
        }
        d.mu.Lock()
        defer d.mu.Unlock()
        return DebouncedCommitterStatus{
                Pending: d.pending,
                Running: d.running,

                LastAt:        d.lastAt,
                LastCommitted: d.lastCommitted,
                LastPushed:    d.lastPushed,
                LastError:     d.lastErr,
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
        committed, err := CommitWorkspaceCanonicalAuto(ctx, d.workspaceDir, actorLabel)
        pushed := false
        lastErr := ""
        if err != nil {
                lastErr = err.Error()
        }
        if committed && d.autoPush {
                st, err := GetStatus(ctx, d.workspaceDir)
                if err == nil && st.IsRepo && !st.Unmerged && !st.InProgress && strings.TrimSpace(st.Upstream) != "" {
                        if err := Push(ctx, d.workspaceDir); err != nil {
                                if d.autoPull && IsNonFastForwardPushErr(err) {
                                        _ = PullRebase(ctx, d.workspaceDir)
                                        if err2 := Push(ctx, d.workspaceDir); err2 != nil {
                                                lastErr = err2.Error()
                                        } else {
                                                pushed = true
                                        }
                                } else {
                                        lastErr = err.Error()
                                }
                        } else {
                                pushed = true
                        }
                }
        }

        d.mu.Lock()
        d.running = false
        d.lastAt = time.Now().UTC()
        d.lastCommitted = committed
        d.lastPushed = pushed
        d.lastErr = lastErr
        // If another Notify happened while running, schedule another run.
        if d.pending && d.timer != nil {
                d.timer.Reset(d.debounce)
        }
        d.mu.Unlock()
}
