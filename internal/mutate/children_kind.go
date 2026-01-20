package mutate

import (
	"errors"
	"strings"

	"clarity-cli/internal/model"
	"clarity-cli/internal/perm"
	"clarity-cli/internal/store"
)

var ErrInvalidChildrenKind = errors.New("invalid children kind")

type SetChildrenKindResult struct {
	Item         *model.Item
	Changed      bool
	EventPayload map[string]any
}

func normalizeChildrenKind(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "status", "normal", "default", "none":
		return "", nil
	case "checkbox", "checkboxes":
		return "checkbox", nil
	default:
		return "", ErrInvalidChildrenKind
	}
}

// SetItemChildrenKind sets it.ChildrenKind, validating the value.
// Callers are responsible for saving db and appending the item.set_children_kind event.
func SetItemChildrenKind(db *store.DB, actorID, itemID, kind string) (SetChildrenKindResult, error) {
	itemID = strings.TrimSpace(itemID)
	actorID = strings.TrimSpace(actorID)
	if db == nil || itemID == "" || actorID == "" {
		return SetChildrenKindResult{}, nil
	}

	it, ok := db.FindItem(itemID)
	if !ok {
		return SetChildrenKindResult{}, NotFoundError{Kind: "item", ID: itemID}
	}
	if !perm.CanEditItem(db, actorID, it) {
		return SetChildrenKindResult{}, OwnerOnlyError{ActorID: actorID, OwnerActorID: it.OwnerActorID, ItemID: itemID}
	}

	next, err := normalizeChildrenKind(kind)
	if err != nil {
		return SetChildrenKindResult{}, err
	}
	prev := strings.TrimSpace(it.ChildrenKind)
	if prev == next {
		return SetChildrenKindResult{Item: it, Changed: false}, nil
	}

	it.ChildrenKind = next
	return SetChildrenKindResult{
		Item:    it,
		Changed: true,
		EventPayload: map[string]any{
			"kind": strings.TrimSpace(it.ChildrenKind),
		},
	}, nil
}
