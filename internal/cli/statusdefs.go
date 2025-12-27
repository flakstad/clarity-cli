package cli

import (
        "clarity-cli/internal/model"
        "clarity-cli/internal/statusutil"
        "clarity-cli/internal/store"
)

func isEndState(db *store.DB, outlineID, statusID string) bool {
        if db != nil {
                if o, ok := db.FindOutline(outlineID); ok && o != nil {
                        return statusutil.IsEndState(*o, statusID)
                }
        }
        // Fallback for older data / unknown outlines.
        return statusutil.IsEndState(model.Outline{}, statusID)
}
