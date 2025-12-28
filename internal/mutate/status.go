package mutate

import (
        "errors"
        "strings"

        "clarity-cli/internal/model"
        "clarity-cli/internal/perm"
        "clarity-cli/internal/statusutil"
        "clarity-cli/internal/store"
)

var ErrInvalidStatus = errors.New("invalid status")
var ErrStatusNoteRequired = errors.New("status note required")

type SetStatusResult struct {
        Item         *model.Item
        Changed      bool
        EventPayload map[string]any
}

// SetItemStatus sets item.StatusID, validating against the item's outline status defs (empty is allowed).
// Callers are responsible for saving db and appending the item.set_status event.
func SetItemStatus(db *store.DB, actorID, itemID, statusID string, note *string) (SetStatusResult, error) {
        itemID = strings.TrimSpace(itemID)
        actorID = strings.TrimSpace(actorID)
        statusID = strings.TrimSpace(statusID)
        if db == nil || itemID == "" || actorID == "" {
                return SetStatusResult{}, nil
        }

        it, ok := db.FindItem(itemID)
        if !ok {
                return SetStatusResult{}, NotFoundError{Kind: "item", ID: itemID}
        }
        if !perm.CanEditItem(db, actorID, it) {
                return SetStatusResult{}, OwnerOnlyError{ActorID: actorID, OwnerActorID: it.OwnerActorID, ItemID: itemID}
        }

        prev := strings.TrimSpace(it.StatusID)
        if prev == statusID {
                return SetStatusResult{Item: it, Changed: false}, nil
        }

        // Validate against outline status defs when present (empty allowed).
        if statusID != "" {
                o, ok := db.FindOutline(strings.TrimSpace(it.OutlineID))
                if !ok || o == nil {
                        return SetStatusResult{}, errors.New("outline not found")
                }
                if !statusutil.ValidateStatusID(*o, statusID) {
                        return SetStatusResult{}, ErrInvalidStatus
                }
                if statusutil.RequiresNote(*o, statusID) && note == nil {
                        return SetStatusResult{}, ErrStatusNoteRequired
                }
        }

        it.StatusID = statusID
        payload := map[string]any{
                "from":   prev,
                "to":     strings.TrimSpace(it.StatusID),
                "status": strings.TrimSpace(it.StatusID), // backwards-compat
        }
        if note != nil {
                payload["note"] = *note
        }
        return SetStatusResult{
                Item:         it,
                Changed:      true,
                EventPayload: payload,
        }, nil
}
