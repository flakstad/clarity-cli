package gitrepo

import (
        "context"
        "sync"
        "time"
)

type DebouncedCommitter struct {
        workspaceDir string
        debounce     time.Duration
        message      func() string

        mu      sync.Mutex
        timer   *time.Timer
        pending bool
        running bool
}

type DebouncedCommitterOpts struct {
        WorkspaceDir string
        Debounce     time.Duration
        Message      func() string
}

func NewDebouncedCommitter(opts DebouncedCommitterOpts) *DebouncedCommitter {
        debounce := opts.Debounce
        if debounce <= 0 {
                debounce = 2 * time.Second
        }
        msgFn := opts.Message
        if msgFn == nil {
                msgFn = func() string { return "" }
        }
        return &DebouncedCommitter{
                workspaceDir: opts.WorkspaceDir,
                debounce:     debounce,
                message:      msgFn,
        }
}

func (d *DebouncedCommitter) Notify() {
        if d == nil {
                return
        }

        d.mu.Lock()
        d.pending = true
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
        d.mu.Unlock()

        // Best-effort: commit canonical paths if possible. Errors are intentionally dropped;
        // the user can always run `clarity sync push` / `git status` manually.
        _, _ = CommitWorkspaceCanonical(context.Background(), d.workspaceDir, d.message())

        d.mu.Lock()
        d.running = false
        // If another Notify happened while running, schedule another run.
        if d.pending && d.timer != nil {
                d.timer.Reset(d.debounce)
        }
        d.mu.Unlock()
}
