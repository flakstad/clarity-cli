package cli

import (
        "strings"

        "clarity-cli/internal/store"
)

func resolveStatusIDByLabel(db *store.DB, outlineID, maybeLabel string) (string, bool) {
        label := strings.TrimSpace(maybeLabel)
        if label == "" {
                return "", false
        }
        if o, ok := db.FindOutline(outlineID); ok {
                for _, def := range o.StatusDefs {
                        if def.Label == label {
                                return def.ID, true
                        }
                }
        }
        return "", false
}
