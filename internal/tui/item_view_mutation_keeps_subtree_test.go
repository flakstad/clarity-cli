package tui

import (
	"os"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func TestItemView_MutateItemDoesNotUnnarrowList(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	actorID := "act-human"

	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	outline := model.Outline{
		ID:         "out-a",
		ProjectID:  "proj-a",
		StatusDefs: store.DefaultOutlineStatusDefs(),
		CreatedBy:  actorID,
		CreatedAt:  now,
	}

	db := &store.DB{
		CurrentActorID: actorID,
		Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "A"}},
		Projects: []model.Project{{
			ID:        "proj-a",
			Name:      "Project A",
			CreatedBy: actorID,
			CreatedAt: now,
		}},
		Outlines: []model.Outline{outline},
		Items: []model.Item{
			{
				ID:           "item-a",
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				Rank:         "h",
				Title:        "A",
				StatusID:     "todo",
				OwnerActorID: actorID,
				CreatedBy:    actorID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           "item-b",
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				Rank:         "i",
				Title:        "B (sibling)",
				StatusID:     "todo",
				OwnerActorID: actorID,
				CreatedBy:    actorID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
	}

	s := store.Store{Dir: dir}
	if err := s.Save(db); err != nil {
		t.Fatalf("save db: %v", err)
	}

	m := newAppModel(dir, db)
	m.view = viewItem
	m.pane = paneOutline
	if m.itemsListActive != nil {
		*m.itemsListActive = true
	}
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.openItemID = "item-a"
	m.refreshItemSubtree(db.Outlines[0], "item-a")
	selectListItemByID(&m.itemsList, "item-a")

	// Toggle priority: implemented via mutateItem and must keep the list narrowed.
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m2 := mAny.(appModel)
	if m2.view != viewItem {
		t.Fatalf("expected to stay in viewItem, got %v", m2.view)
	}

	for _, it := range m2.itemsList.Items() {
		if row, ok := it.(outlineRowItem); ok {
			if row.row.item.ID == "item-b" {
				t.Fatalf("expected list to remain narrowed; found sibling item-b in itemsList")
			}
		}
	}
}
