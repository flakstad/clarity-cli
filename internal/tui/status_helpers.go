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

// countProgressChildren counts direct children used for the progress cookie.
//
// Rules:
// - Only children with an explicit status are counted (items without status are ignored).
// - "Done" is determined by the outline's end-state definitions.
func countProgressChildren(outline model.Outline, children []model.Item) (done int, total int) {
        for _, ch := range children {
                sid := strings.TrimSpace(ch.StatusID)
                if sid == "" {
                        continue
                }
                total++
                if isEndState(outline, sid) {
                        done++
                }
        }
        return done, total
}
