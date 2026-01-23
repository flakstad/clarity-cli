package tui

import (
	"strings"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func TestItemDescription_OpenLinksPicker_FromItemView(t *testing.T) {
	dir := t.TempDir()

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
			Title:        "Title",
			StatusID:     "todo",
			Description:  "See https://example.com",
			OwnerActorID: actorID,
			CreatedBy:    actorID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}},
	}

	m := newAppModel(dir, db)
	m.view = viewItem
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.openItemID = "item-a"
	m.pane = paneOutline
	if m.collapsed == nil {
		m.collapsed = map[string]bool{}
	}
	m.itemCollapsed = map[string]bool{
		"item-a": false,
	}
	m.refreshItemSubtree(db.Outlines[0], "item-a")
	selectListItemByID(&m.itemsList, "item-a")

	// L => open links picker from description (not outline navigation).
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	m2 := mAny.(appModel)
	if m2.modal != modalPickTargets {
		t.Fatalf("expected modalPickTargets, got %v", m2.modal)
	}

	found := false
	for _, t0 := range m2.targetPickTargets {
		if t0.Kind == targetPickTargetURL && strings.TrimSpace(t0.Target) == "https://example.com" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected target picker to include url %q; got %+v", "https://example.com", m2.targetPickTargets)
	}
}
