package tui

import (
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCommentActivity_TabCycle_CollapsesAfterOpenAll(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	s := store.Store{Dir: dir}

	db := &store.DB{
		Actors:   []model.Actor{{ID: "human-a", Kind: model.ActorKindHuman, Name: "andreas"}},
		Projects: []model.Project{{ID: "proj-a", Name: "P", CreatedBy: "human-a", CreatedAt: now}},
		Outlines: []model.Outline{{ID: "out-a", ProjectID: "proj-a", CreatedBy: "human-a", CreatedAt: now}},
		Items: []model.Item{
			{ID: "item-a", ProjectID: "proj-a", OutlineID: "out-a", Title: "Item", CreatedBy: "human-a", CreatedAt: now},
		},
	}
	parentID := "c-parent"
	childID := "c-child"
	db.Comments = []model.Comment{
		{ID: parentID, ItemID: "item-a", AuthorID: "human-a", Body: "parent"},
		{ID: childID, ItemID: "item-a", AuthorID: "human-a", ReplyToCommentID: &parentID, Body: "child"},
	}

	if err := s.Save(db); err != nil {
		t.Fatalf("save db: %v", err)
	}

	m := newAppModel(dir, db)
	m.width = 120
	m.height = 40
	m.view = viewItem
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.openItemID = "item-a"
	m.itemCollapsed = map[string]bool{
		"item-a":                         false,
		activityCommentsRootID("item-a"): false,
	}

	m.refreshItemSubtree(db.Outlines[0], "item-a")
	selectListItemByID(&m.itemsList, parentID)

	// collapsed -> open first layer
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2 := mAny.(appModel)
	if m2.itemCollapsed[parentID] {
		t.Fatalf("expected parent to be expanded after first tab")
	}
	if !m2.itemCollapsed[childID] {
		t.Fatalf("expected child to be collapsed in first-layer state")
	}

	// open first layer -> open all
	mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3 := mAny.(appModel)
	if m3.itemCollapsed[parentID] {
		t.Fatalf("expected parent to stay expanded in open-all state")
	}
	if m3.itemCollapsed[childID] {
		t.Fatalf("expected child to be expanded in open-all state")
	}

	// open all -> collapsed
	mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyTab})
	m4 := mAny.(appModel)
	if !m4.itemCollapsed[parentID] {
		t.Fatalf("expected parent to be collapsed after third tab")
	}
	if !m4.itemCollapsed[childID] {
		t.Fatalf("expected child to be collapsed after third tab")
	}
}
