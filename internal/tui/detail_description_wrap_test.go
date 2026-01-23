package tui

import (
	"strings"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func TestRenderItemDetail_WrapsDescriptionEarlier(t *testing.T) {
	db := &store.DB{
		Actors: []model.Actor{
			{ID: "act-1", Kind: model.ActorKindHuman, Name: "A"},
		},
		Items: []model.Item{},
	}
	outline := model.Outline{
		ID:        "out-1",
		ProjectID: "proj-1",
		StatusDefs: []model.OutlineStatusDef{
			{ID: "todo", Label: "Todo", IsEndState: false},
		},
		CreatedBy: "act-1",
		CreatedAt: time.Now(),
	}

	// Regression: item description should wrap a bit earlier so it isn't cut off in the pane.
	// With width=20 => innerW=18 (padX=1). We intentionally render markdown with a small
	// safety margin (innerW-2 => 16), so this should wrap after the leading "a".
	it := model.Item{
		ID:           "item-1",
		ProjectID:    "proj-1",
		OutlineID:    "out-1",
		Title:        "Title",
		Description:  "a 1234567890123456",
		StatusID:     "todo",
		OwnerActorID: "act-1",
		CreatedBy:    "act-1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	out := renderItemDetail(db, outline, it, 20, 20, false, nil)
	plain := stripANSIEscapes(out)
	lines := strings.Split(plain, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	plain = strings.Join(lines, "\n")

	if !strings.Contains(plain, "Description\na\n1234567890123456") {
		t.Fatalf("expected description to wrap earlier; got:\n%s", plain)
	}
}
