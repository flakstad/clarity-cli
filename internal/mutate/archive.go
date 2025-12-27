package mutate

import (
        "strings"

        "clarity-cli/internal/model"
        "clarity-cli/internal/perm"
        "clarity-cli/internal/store"
)

type ArchiveResult struct {
        Item         *model.Item
        Changed      bool
        EventPayload map[string]any
}

// SetItemArchived sets item.Archived. It enforces permissions via internal/perm.
// Callers are responsible for saving db and appending the item.archive event.
func SetItemArchived(db *store.DB, actorID, itemID string, archived bool) (ArchiveResult, error) {
        itemID = strings.TrimSpace(itemID)
        actorID = strings.TrimSpace(actorID)
        if db == nil || itemID == "" || actorID == "" {
                return ArchiveResult{}, nil
        }

        it, ok := db.FindItem(itemID)
        if !ok {
                return ArchiveResult{}, NotFoundError{Kind: "item", ID: itemID}
        }
        if !perm.CanEditItem(db, actorID, it) {
                return ArchiveResult{}, OwnerOnlyError{ActorID: actorID, OwnerActorID: it.OwnerActorID, ItemID: itemID}
        }
        if it.Archived == archived {
                return ArchiveResult{Item: it, Changed: false}, nil
        }
        it.Archived = archived
        return ArchiveResult{
                Item:    it,
                Changed: true,
                EventPayload: map[string]any{
                        "archived": it.Archived,
                },
        }, nil
}
