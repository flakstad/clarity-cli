package mutate

import (
	"errors"
	"strings"

	"clarity-cli/internal/model"
	"clarity-cli/internal/perm"
	"clarity-cli/internal/store"
)

var ErrInvalidItemKind = errors.New("invalid item kind")

type SetItemKindResult struct {
	Item         *model.Item
	Changed      bool
	EventPayload map[string]any
}

func normalizeItemKind(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "inherit", "default", "none":
		return "", nil
	case "checkbox", "checklist":
		return "checkbox", nil
	case "status", "normal":
		return "status", nil
	default:
		return "", ErrInvalidItemKind
	}
}

// SetItemKind sets it.ItemKind.
// Callers are responsible for saving db and appending the item.set_item_kind event.
func SetItemKind(db *store.DB, actorID, itemID, kind string) (SetItemKindResult, error) {
	itemID = strings.TrimSpace(itemID)
	actorID = strings.TrimSpace(actorID)
	if db == nil || itemID == "" || actorID == "" {
		return SetItemKindResult{}, nil
	}

	it, ok := db.FindItem(itemID)
	if !ok {
		return SetItemKindResult{}, NotFoundError{Kind: "item", ID: itemID}
	}
	if !perm.CanEditItem(db, actorID, it) {
		return SetItemKindResult{}, OwnerOnlyError{ActorID: actorID, OwnerActorID: it.OwnerActorID, ItemID: itemID}
	}

	next, err := normalizeItemKind(kind)
	if err != nil {
		return SetItemKindResult{}, err
	}
	prev := strings.TrimSpace(it.ItemKind)
	if prev == next {
		return SetItemKindResult{Item: it, Changed: false}, nil
	}

	it.ItemKind = next
	return SetItemKindResult{
		Item:    it,
		Changed: true,
		EventPayload: map[string]any{
			"kind": strings.TrimSpace(it.ItemKind),
		},
	}, nil
}
