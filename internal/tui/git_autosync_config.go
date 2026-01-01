package tui

import (
        "os"
        "strings"
)

func shouldAutoCommit() bool {
        // Default-on for Git-backed v1 workspaces: keep working tree clean so users rarely see [git: dirty].
        // Disable with CLARITY_AUTOCOMMIT=0.
        v := strings.TrimSpace(os.Getenv("CLARITY_AUTOCOMMIT"))
        if v == "" {
                v = strings.TrimSpace(os.Getenv("CLARITY_GIT_AUTOCOMMIT"))
        }
        v = strings.ToLower(strings.TrimSpace(v))
        if v == "0" || v == "false" || v == "off" || v == "no" {
                return false
        }
        return true
}

func shouldAutoPush() bool {
        // Default-on (best-effort) when upstream exists; disable with CLARITY_AUTOPUSH=0.
        v := strings.ToLower(strings.TrimSpace(os.Getenv("CLARITY_AUTOPUSH")))
        if v == "0" || v == "false" || v == "off" || v == "no" {
                return false
        }
        return true
}
