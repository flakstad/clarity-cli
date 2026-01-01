package tui

import (
        "context"
        "fmt"
        "path/filepath"
        "strings"
        "time"

        "clarity-cli/internal/gitrepo"
        "clarity-cli/internal/store"

        tea "github.com/charmbracelet/bubbletea"
)

func (m *appModel) syncPullCmd() tea.Cmd {
        if m == nil {
                return nil
        }
        dir := m.dir
        return func() tea.Msg {
                ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
                defer cancel()

                st, err := gitrepo.GetStatus(ctx, dir)
                if err != nil {
                        return syncOpDoneMsg{op: "pull", status: st, err: err.Error()}
                }
                if !st.IsRepo {
                        return syncOpDoneMsg{op: "pull", status: st, err: "not a git repository"}
                }
                if st.Unmerged || st.InProgress {
                        return syncOpDoneMsg{op: "pull", status: st, err: "repo has in-progress merge/rebase; resolve first"}
                }
                if st.DirtyTracked {
                        return syncOpDoneMsg{op: "pull", status: st, err: "repo has local changes; commit/push first"}
                }

                if err := gitrepo.PullRebase(ctx, dir); err != nil {
                        st2, _ := gitrepo.GetStatus(ctx, dir)
                        return syncOpDoneMsg{op: "pull", status: st2, err: err.Error()}
                }
                st2, _ := gitrepo.GetStatus(ctx, dir)
                return syncOpDoneMsg{op: "pull", status: st2, err: ""}
        }
}

func (m *appModel) syncPushCmd() tea.Cmd {
        if m == nil {
                return nil
        }
        dir := m.dir
        actorID := strings.TrimSpace(m.editActorID())
        return func() tea.Msg {
                ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
                defer cancel()

                st, err := gitrepo.GetStatus(ctx, dir)
                if err != nil {
                        return syncOpDoneMsg{op: "push", status: st, err: err.Error()}
                }
                if !st.IsRepo {
                        return syncOpDoneMsg{op: "push", status: st, err: "not a git repository"}
                }
                if st.Unmerged || st.InProgress {
                        return syncOpDoneMsg{op: "push", status: st, err: "repo has in-progress merge/rebase; resolve first"}
                }

                msg := fmt.Sprintf("clarity: update (%s)", time.Now().UTC().Format(time.RFC3339))
                if actorID != "" {
                        msg = fmt.Sprintf("clarity: %s update (%s)", actorID, time.Now().UTC().Format(time.RFC3339))
                }

                _, err = gitrepo.CommitWorkspaceCanonical(ctx, dir, msg)
                if err != nil {
                        st2, _ := gitrepo.GetStatus(ctx, dir)
                        return syncOpDoneMsg{op: "push", status: st2, err: err.Error()}
                }

                // Best-effort: if upstream exists, do a pull/rebase first to reduce push rejects.
                st, _ = gitrepo.GetStatus(ctx, dir)
                if strings.TrimSpace(st.Upstream) != "" && !st.Unmerged && !st.InProgress && !st.DirtyTracked {
                        _ = gitrepo.PullRebase(ctx, dir)
                }

                if err := gitrepo.Push(ctx, dir); err != nil {
                        // Retry once on non-fast-forward by pulling/rebasing.
                        if gitrepo.IsNonFastForwardPushErr(err) {
                                if pullErr := gitrepo.PullRebase(ctx, dir); pullErr != nil {
                                        st2, _ := gitrepo.GetStatus(ctx, dir)
                                        return syncOpDoneMsg{op: "push", status: st2, err: pullErr.Error()}
                                }
                                if pushErr := gitrepo.Push(ctx, dir); pushErr != nil {
                                        st2, _ := gitrepo.GetStatus(ctx, dir)
                                        return syncOpDoneMsg{op: "push", status: st2, err: pushErr.Error()}
                                }
                        } else {
                                st2, _ := gitrepo.GetStatus(ctx, dir)
                                return syncOpDoneMsg{op: "push", status: st2, err: err.Error()}
                        }
                }

                st2, _ := gitrepo.GetStatus(ctx, dir)
                return syncOpDoneMsg{op: "push", status: st2, err: ""}
        }
}

func (m *appModel) syncSetupCmd(remoteURL string) tea.Cmd {
        if m == nil {
                return nil
        }
        dir := m.dir
        remoteURL = strings.TrimSpace(remoteURL)

        return func() tea.Msg {
                ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
                defer cancel()

                // Ensure Clarity ignores exist.
                if _, err := store.EnsureGitignoreHasClarityIgnores(filepath.Join(dir, ".gitignore")); err != nil {
                        st, _ := gitrepo.GetStatus(ctx, dir)
                        return syncOpDoneMsg{op: "setup", status: st, err: err.Error()}
                }

                st, err := gitrepo.GetStatus(ctx, dir)
                if err != nil {
                        return syncOpDoneMsg{op: "setup", status: st, err: err.Error()}
                }
                if !st.IsRepo {
                        if err := gitrepo.Init(ctx, dir); err != nil {
                                st2, _ := gitrepo.GetStatus(ctx, dir)
                                return syncOpDoneMsg{op: "setup", status: st2, err: err.Error()}
                        }
                }

                // Commit canonical workspace files (best-effort; no-op when nothing to commit).
                _, err = gitrepo.CommitWorkspaceCanonical(ctx, dir, "")
                if err != nil {
                        st2, _ := gitrepo.GetStatus(ctx, dir)
                        return syncOpDoneMsg{op: "setup", status: st2, err: err.Error()}
                }

                if remoteURL != "" {
                        if err := gitrepo.SetRemoteURL(ctx, dir, "origin", remoteURL); err != nil {
                                st2, _ := gitrepo.GetStatus(ctx, dir)
                                return syncOpDoneMsg{op: "setup", status: st2, err: err.Error()}
                        }
                        st2, _ := gitrepo.GetStatus(ctx, dir)
                        branch := strings.TrimSpace(st2.Branch)
                        if branch == "" {
                                branch = "HEAD"
                        }
                        if err := gitrepo.PushSetUpstream(ctx, dir, "origin", branch); err != nil {
                                st3, _ := gitrepo.GetStatus(ctx, dir)
                                return syncOpDoneMsg{op: "setup", status: st3, err: err.Error()}
                        }
                }

                st2, _ := gitrepo.GetStatus(ctx, dir)
                return syncOpDoneMsg{op: "setup", status: st2, err: ""}
        }
}

func (m *appModel) syncResolveNote() {
        if m == nil {
                return
        }
        st := m.gitStatus
        if !st.IsRepo {
                m.showMinibuffer("Sync: not a git repository")
                return
        }
        if !(st.Unmerged || st.InProgress) {
                m.showMinibuffer("Sync: no in-progress merge/rebase detected")
                return
        }
        // Keep short; point to CLI for details.
        m.showMinibuffer("Sync: resolve Git conflicts first (try: clarity sync resolve)")
}

func (m *appModel) syncRefreshStatusNow() tea.Cmd {
        if m == nil {
                return nil
        }
        if m.gitStatusFetching {
                return nil
        }
        return m.startGitStatusRefresh()
}
