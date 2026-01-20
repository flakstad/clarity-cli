package mutate

import (
	"testing"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func TestSetItemChildrenKind_ValidatesAndToggles(t *testing.T) {
	db := &store.DB{
		Actors: []model.Actor{
			{ID: "act-human", Kind: model.ActorKindHuman, Name: "Human", UserID: strPtr("act-human")},
		},
		Items: []model.Item{
			{
				ID:           "item-1",
				OwnerActorID: "act-human",
			},
		},
	}

	if _, err := SetItemChildrenKind(db, "act-human", "item-1", "wat"); err == nil {
		t.Fatalf("expected error")
	} else if err != ErrInvalidChildrenKind {
		t.Fatalf("expected ErrInvalidChildrenKind; got %v", err)
	}

	res, err := SetItemChildrenKind(db, "act-human", "item-1", "checkbox")
	if err != nil {
		t.Fatalf("SetItemChildrenKind error: %v", err)
	}
	if !res.Changed {
		t.Fatalf("expected changed=true")
	}
	if got := res.Item.ChildrenKind; got != "checkbox" {
		t.Fatalf("expected childrenKind=checkbox; got %q", got)
	}

	res2, err := SetItemChildrenKind(db, "act-human", "item-1", "")
	if err != nil {
		t.Fatalf("SetItemChildrenKind clear error: %v", err)
	}
	if !res2.Changed {
		t.Fatalf("expected changed=true for clear")
	}
	if got := res2.Item.ChildrenKind; got != "" {
		t.Fatalf("expected childrenKind cleared; got %q", got)
	}
}
