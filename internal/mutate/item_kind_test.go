package mutate

import (
	"testing"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func TestSetItemKind_ValidatesAndClears(t *testing.T) {
	db := &store.DB{
		Actors: []model.Actor{
			{ID: "act-human", Kind: model.ActorKindHuman, Name: "Human", UserID: strPtr("act-human")},
		},
		Items: []model.Item{
			{ID: "item-1", OwnerActorID: "act-human"},
		},
	}

	if _, err := SetItemKind(db, "act-human", "item-1", "wat"); err == nil {
		t.Fatalf("expected error")
	} else if err != ErrInvalidItemKind {
		t.Fatalf("expected ErrInvalidItemKind; got %v", err)
	}

	res, err := SetItemKind(db, "act-human", "item-1", "status")
	if err != nil {
		t.Fatalf("SetItemKind error: %v", err)
	}
	if !res.Changed {
		t.Fatalf("expected changed=true")
	}
	if got := res.Item.ItemKind; got != "status" {
		t.Fatalf("expected itemKind=status; got %q", got)
	}

	res2, err := SetItemKind(db, "act-human", "item-1", "inherit")
	if err != nil {
		t.Fatalf("SetItemKind clear error: %v", err)
	}
	if !res2.Changed {
		t.Fatalf("expected changed=true for clear")
	}
	if got := res2.Item.ItemKind; got != "" {
		t.Fatalf("expected itemKind cleared; got %q", got)
	}
}
