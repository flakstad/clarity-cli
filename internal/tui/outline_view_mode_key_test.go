package tui

import (
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOutlineView_V_TogglesListAndColumns(t *testing.T) {
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
			Title:        "A",
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
	m.width = 120
	m.height = 40
	m.view = viewOutline
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.refreshItems(db.Outlines[0])

	if got := m.curOutlineViewMode(); got != outlineViewModeList {
		t.Fatalf("expected list mode initially, got %v", got)
	}

	// v -> columns
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m2 := mAny.(appModel)
	if got := m2.curOutlineViewMode(); got != outlineViewModeColumns {
		t.Fatalf("expected columns mode after v, got %v", got)
	}

	// v -> list (handled inside updateOutlineColumns)
	mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m3 := mAny.(appModel)
	if got := m3.curOutlineViewMode(); got != outlineViewModeList {
		t.Fatalf("expected list mode after v again, got %v", got)
	}
}
