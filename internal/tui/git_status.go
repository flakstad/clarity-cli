package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"clarity-cli/internal/gitrepo"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *appModel) shouldRefreshGitStatus() bool {
	if m == nil {
		return false
	}
	if m.gitStatusFetching {
		return false
	}
	if m.gitStatusAt.IsZero() {
		return true
	}
	return time.Since(m.gitStatusAt) > 3*time.Second
}

func (m *appModel) startGitStatusRefresh() tea.Cmd {
	if m == nil {
		return nil
	}
	m.gitStatusFetchSeq++
	seq := m.gitStatusFetchSeq
	m.gitStatusFetching = true
	dir := m.dir

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		st, err := gitrepo.GetStatus(ctx, dir)
		out := gitStatusMsg{seq: seq, status: st}
		if err != nil {
			out.err = err.Error()
		}
		return out
	}
}

func (m *appModel) gitStatusBadgeText() string {
	if m == nil {
		return ""
	}
	if strings.TrimSpace(m.gitStatusErr) != "" {
		return "[git: ?]"
	}
	st := m.gitStatus
	if !st.IsRepo {
		return ""
	}
	if st.Unmerged || st.InProgress {
		return "[git: conflict]"
	}
	if st.DirtyTracked {
		return "[git: dirty]"
	}
	if st.Behind > 0 {
		return fmt.Sprintf("[git: behind %d]", st.Behind)
	}
	if st.Ahead > 0 {
		return fmt.Sprintf("[git: ahead %d]", st.Ahead)
	}
	if strings.TrimSpace(st.Upstream) == "" {
		return "[git: no upstream]"
	}
	return "[git: synced]"
}
