package tui

import (
        "strings"

        "clarity-cli/internal/model"
)

func isEndState(outline model.Outline, statusID string) bool {
        sid := strings.TrimSpace(statusID)
        if sid == "" {
                return false
        }
        for _, def := range outline.StatusDefs {
                if def.ID == sid {
                        return def.IsEndState
                }
        }
        // Fallback for legacy outlines or stores without status defs.
        return strings.ToLower(sid) == "done"
}
