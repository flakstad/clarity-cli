package tui

import (
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOutlineView_V_DuplicatesSelectedItem(t *testing.T) {
	dir := t.TempDir()
	s := store.Store{Dir: dir}

	actorID := "act-human"
	now := time.Now().UTC()
	db := &store.DB{
		CurrentActorID: actorID,
		Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "human"}},
		Projects: []model.Project{{
			ID:        "proj-a",
			Name:      "Project A",
			CreatedBy: actorID,
			CreatedAt: now,
		}},
		Outlines: []model.Outline{{
			ID:         "out-a",
			ProjectID:  "proj-a",
			StatusDefs: store.DefaultOutlineStatusDefs(),
			CreatedBy:  actorID,
			CreatedAt:  now,
		}},
		Items: []model.Item{
			{
				ID:           "item-a",
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				Rank:         "h",
				Title:        "Title A",
				Description:  "Desc A",
				StatusID:     "doing",
				Priority:     true,
				OnHold:       true,
				Tags:         []string{"x", "y"},
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
				Title:        "Title B",
				StatusID:     "todo",
				OwnerActorID: actorID,
				CreatedBy:    actorID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
	}
	if err := s.Save(db); err != nil {
		t.Fatalf("save db: %v", err)
	}

	m := newAppModel(dir, db)
	m.view = viewOutline
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.refreshItems(db.Outlines[0])
	selectListItemByID(&m.itemsList, "item-a")

	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'V'}})
	m2 := mAny.(appModel)

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("load db: %v", err)
	}
	if len(loaded.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(loaded.Items))
	}
	var copied *model.Item
	for i := range loaded.Items {
		if loaded.Items[i].ID != "item-a" && loaded.Items[i].ID != "item-b" {
			copied = &loaded.Items[i]
			break
		}
	}
	if copied == nil {
		t.Fatalf("expected copied item")
	}
	if copied.Title != "Title A" || copied.Description != "Desc A" {
		t.Fatalf("unexpected copied content: %#v", copied)
	}
	if copied.StatusID != "todo" || copied.Priority || copied.OnHold {
		t.Fatalf("expected copied item to reset status/flags; got status=%q priority=%v onHold=%v", copied.StatusID, copied.Priority, copied.OnHold)
	}
	if copied.Rank <= "h" || copied.Rank >= "i" {
		t.Fatalf("expected copied rank between %q and %q; got %q", "h", "i", copied.Rank)
	}
	if m2.view != viewOutline {
		t.Fatalf("expected to stay in outline view")
	}
}

func TestItemView_V_DuplicatesFocusedItemAndOpensCopy(t *testing.T) {
	dir := t.TempDir()
	s := store.Store{Dir: dir}

	actorID := "act-human"
	now := time.Now().UTC()
	db := &store.DB{
		CurrentActorID: actorID,
		Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "human"}},
		Projects: []model.Project{{
			ID:        "proj-a",
			Name:      "Project A",
			CreatedBy: actorID,
			CreatedAt: now,
		}},
		Outlines: []model.Outline{{
			ID:         "out-a",
			ProjectID:  "proj-a",
			StatusDefs: store.DefaultOutlineStatusDefs(),
			CreatedBy:  actorID,
			CreatedAt:  now,
		}},
		Items: []model.Item{{
			ID:           "item-a",
			ProjectID:    "proj-a",
			OutlineID:    "out-a",
			Rank:         "h",
			Title:        "Title A",
			StatusID:     "todo",
			OwnerActorID: actorID,
			CreatedBy:    actorID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}},
	}
	if err := s.Save(db); err != nil {
		t.Fatalf("save db: %v", err)
	}

	m := newAppModel(dir, db)
	m.view = viewItem
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.openItemID = "item-a"

	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'V'}})
	m2 := mAny.(appModel)
	if m2.view != viewItem {
		t.Fatalf("expected viewItem, got %v", m2.view)
	}
	if m2.openItemID == "item-a" {
		t.Fatalf("expected openItemID to change")
	}
	if len(m2.itemNavStack) != 1 {
		t.Fatalf("expected nav stack size 1, got %d", len(m2.itemNavStack))
	}
}
