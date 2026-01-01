package gitrepo

import (
        "os"
        "strconv"
        "strings"
)

func boolEnvDefault(k string, def bool) bool {
        v := strings.TrimSpace(os.Getenv(k))
        if v == "" {
                return def
        }
        if b, err := strconv.ParseBool(v); err == nil {
                return b
        }
        switch strings.ToLower(v) {
        case "y", "yes", "on", "1":
                return true
        case "n", "no", "off", "0":
                return false
        default:
                return def
        }
}

// AutoCommitEnabled controls whether Clarity automatically commits canonical changes.
// Default: true. Disable with CLARITY_AUTOCOMMIT=0.
func AutoCommitEnabled() bool {
        v := strings.TrimSpace(os.Getenv("CLARITY_AUTOCOMMIT"))
        if v == "" {
                // Backwards-compatible alias used in earlier experiments.
                v = strings.TrimSpace(os.Getenv("CLARITY_GIT_AUTOCOMMIT"))
        }
        if v == "" {
                return true
        }
        if b, err := strconv.ParseBool(v); err == nil {
                return b
        }
        switch strings.ToLower(v) {
        case "y", "yes", "on", "1":
                return true
        case "n", "no", "off", "0":
                return false
        default:
                return true
        }
}

// AutoPushEnabled controls whether Clarity automatically pushes after committing.
// Default: true. Disable with CLARITY_AUTOPUSH=0.
func AutoPushEnabled() bool {
        return boolEnvDefault("CLARITY_AUTOPUSH", true)
}

// AutoPullRebaseEnabled controls whether Clarity retries non-fast-forward pushes by pulling with rebase first.
// Default: true. Disable with CLARITY_AUTOPULL_REBASE=0.
func AutoPullRebaseEnabled() bool {
        return boolEnvDefault("CLARITY_AUTOPULL_REBASE", true)
}
