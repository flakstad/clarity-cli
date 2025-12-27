package tui

import (
        "strings"

        "clarity-cli/internal/model"
        "clarity-cli/internal/statusutil"
)

func isEndState(outline model.Outline, statusID string) bool {
        return statusutil.IsEndState(outline, statusID)
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
                if statusutil.IsEndState(outline, sid) {
                        done++
                }
        }
        return done, total
}
