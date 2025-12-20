package cli

import (
        "clarity-cli/internal/model"
        "clarity-cli/internal/perm"
        "clarity-cli/internal/store"
)

// Backwards-compatible wrapper used throughout the CLI package.
func canEditTask(db *store.DB, actorID string, t *model.Item) bool {
        return perm.CanEditItem(db, actorID, t)
}
