package cli

import "clarity-cli/internal/store"

func isEndState(db *store.DB, outlineID, statusID string) bool {
        if def, ok := db.StatusDef(outlineID, statusID); ok {
                return def.IsEndState
        }
        // Fallback for older data / unknown outlines.
        return statusID == "done"
}
