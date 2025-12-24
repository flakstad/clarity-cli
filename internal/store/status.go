package store

import (
        "strings"

        "clarity-cli/internal/model"
)

// FirstStatusID returns the first status id for an outline, or "" if none exist.
func FirstStatusID(defs []model.OutlineStatusDef) string {
        if len(defs) == 0 {
                return ""
        }
        return strings.TrimSpace(defs[0].ID)
}
