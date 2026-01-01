package tui

import (
        "clarity-cli/internal/gitrepo"
)

func shouldAutoCommit() bool {
        return gitrepo.AutoCommitEnabled()
}

func shouldAutoPush() bool {
        return gitrepo.AutoPushEnabled()
}
