package tui

import (
	"strings"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOutlineView_ItemDescription_OpenLinksPicker(t *testing.T) {
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
	m.width = 120
	m.height = 40
	m.view = viewOutline
	m.pane = paneOutline
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.refreshItems(db.Outlines[0])
	selectListItemByID(&m.itemsList, "item-a")

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

func TestOutlineView_CommentRow_OpenLinksPicker(t *testing.T) {
	dir := t.TempDir()

	actorID := "act-human"
	now := time.Now().UTC()

	itemID := "item-a"
	commentID := "c1"

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
			ID:           itemID,
			ProjectID:    "proj-a",
			OutlineID:    "out-a",
			Rank:         "h",
			Title:        "Title",
			StatusID:     "todo",
			OwnerActorID: actorID,
			CreatedBy:    actorID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}},
		Comments: []model.Comment{{
			ID:        commentID,
			ItemID:    itemID,
			AuthorID:  actorID,
			Body:      "See https://example.com",
			CreatedAt: now.Add(1 * time.Minute),
		}},
	}

	m := newAppModel(dir, db)
	m.width = 120
	m.height = 40
	m.view = viewOutline
	m.pane = paneOutline
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	if m.collapsed == nil {
		m.collapsed = map[string]bool{}
	}
	m.collapsed[itemID] = false
	m.collapsed[activityCommentsRootID(itemID)] = false
	m.refreshItems(db.Outlines[0])
	selectListItemByID(&m.itemsList, commentID)

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

func TestOutlineView_WorklogRow_OpenLinksPicker(t *testing.T) {
	dir := t.TempDir()

	actorID := "act-human"
	now := time.Now().UTC()

	itemID := "item-a"
	worklogID := "w1"

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
			ID:           itemID,
			ProjectID:    "proj-a",
			OutlineID:    "out-a",
			Rank:         "h",
			Title:        "Title",
			StatusID:     "todo",
			OwnerActorID: actorID,
			CreatedBy:    actorID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}},
		Worklog: []model.WorklogEntry{{
			ID:        worklogID,
			ItemID:    itemID,
			AuthorID:  actorID,
			Body:      "See https://example.com",
			CreatedAt: now.Add(1 * time.Minute),
		}},
	}

	m := newAppModel(dir, db)
	m.width = 120
	m.height = 40
	m.view = viewOutline
	m.pane = paneOutline
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	if m.collapsed == nil {
		m.collapsed = map[string]bool{}
	}
	m.collapsed[itemID] = false
	m.collapsed[activityWorklogRootID(itemID)] = false
	m.refreshItems(db.Outlines[0])
	selectListItemByID(&m.itemsList, worklogID)

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
