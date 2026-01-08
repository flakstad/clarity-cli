package tui

import (
	"strings"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func TestItemView_TabCyclesFocus_EnterOpensModal(t *testing.T) {
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
			Title:        "Title",
			Description:  "Old description",
			StatusID:     "todo",
			Priority:     false,
			OnHold:       false,
			Archived:     false,
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
	m.itemFocus = itemFocusTitle

	// tab => status
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2 := mAny.(appModel)
	if m2.itemFocus != itemFocusStatus {
		t.Fatalf("expected focus=%v, got %v", itemFocusStatus, m2.itemFocus)
	}

	// tab => assigned; tab => tags; tab => priority
	mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3 := mAny.(appModel) // assigned
	mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3b := mAny.(appModel) // tags
	mAny, _ = m3b.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3 = mAny.(appModel) // priority
	if m3.itemFocus != itemFocusPriority {
		t.Fatalf("expected focus=%v, got %v", itemFocusPriority, m3.itemFocus)
	}

	// tab => description
	mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyTab})
	m4 := mAny.(appModel)
	// New tab order puts Parent/Children before Description.
	if m4.itemFocus != itemFocusChildren {
		t.Fatalf("expected focus=%v, got %v", itemFocusChildren, m4.itemFocus)
	}

	// tab => description
	mAny, _ = m4.Update(tea.KeyMsg{Type: tea.KeyTab})
	m4 = mAny.(appModel)
	if m4.itemFocus != itemFocusDescription {
		t.Fatalf("expected focus=%v, got %v", itemFocusDescription, m4.itemFocus)
	}

	// enter => edit description modal, seeded with existing description.
	mAny, _ = m4.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m5 := mAny.(appModel)
	if m5.modal != modalEditDescription {
		t.Fatalf("expected modalEditDescription, got %v", m5.modal)
	}
	if got := strings.TrimSpace(m5.textarea.Value()); got != "Old description" {
		t.Fatalf("expected textarea seeded with old description; got %q", got)
	}
}

func TestItemView_ShortcutsOpenModals(t *testing.T) {
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
			Title:        "Title",
			Description:  "Old description",
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

	// Shift+D => edit description modal.
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m2 := mAny.(appModel)
	if m2.modal != modalEditDescription {
		t.Fatalf("expected modalEditDescription, got %v", m2.modal)
	}

	// ESC closes modal via modal handler; keep it simple here by manually clearing.
	m2.modal = modalNone
	m2.modalForID = ""

	// C => add comment modal (and focus comments so the side panel stays open after save).
	mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	m3 := mAny.(appModel)
	if m3.modal != modalAddComment {
		t.Fatalf("expected modalAddComment, got %v", m3.modal)
	}
	if m3.itemFocus != itemFocusComments {
		t.Fatalf("expected focus=%v, got %v", itemFocusComments, m3.itemFocus)
	}
}

func TestItemView_P_TogglesPriority(t *testing.T) {
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
			Title:        "Title",
			StatusID:     "todo",
			Priority:     false,
			OnHold:       false,
			Archived:     false,
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

	// p => toggle priority
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	_ = mAny.(appModel)

	db2, err := s.Load()
	if err != nil {
		t.Fatalf("load db: %v", err)
	}
	it, ok := db2.FindItem("item-a")
	if !ok {
		t.Fatalf("expected item to exist")
	}
	if !it.Priority {
		t.Fatalf("expected priority=true after toggle; got false")
	}
}

func TestItemView_O_TogglesOnHold(t *testing.T) {
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
			Title:        "Title",
			StatusID:     "todo",
			Priority:     false,
			OnHold:       false,
			Archived:     false,
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

	// o => toggle on hold
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	_ = mAny.(appModel)

	db2, err := s.Load()
	if err != nil {
		t.Fatalf("load db: %v", err)
	}
	it, ok := db2.FindItem("item-a")
	if !ok {
		t.Fatalf("expected item to exist")
	}
	if !it.OnHold {
		t.Fatalf("expected onHold=true after toggle; got false")
	}
}

func TestItemView_TabToPriority_EnterTogglesPriority(t *testing.T) {
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
			Title:        "Title",
			StatusID:     "todo",
			Priority:     false,
			OnHold:       false,
			Archived:     false,
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
	m.itemFocus = itemFocusTitle

	// tab => status; tab => assigned; tab => tags; tab => priority
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2 := mAny.(appModel)
	mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3 := mAny.(appModel) // assigned
	mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyTab})
	m4 := mAny.(appModel) // tags
	mAny, _ = m4.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3 = mAny.(appModel) // priority
	if m3.itemFocus != itemFocusPriority {
		t.Fatalf("expected focus=%v, got %v", itemFocusPriority, m3.itemFocus)
	}

	// enter => toggle priority
	mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = mAny.(appModel)

	db2, err := s.Load()
	if err != nil {
		t.Fatalf("load db: %v", err)
	}
	it, ok := db2.FindItem("item-a")
	if !ok {
		t.Fatalf("expected item to exist")
	}
	if !it.Priority {
		t.Fatalf("expected priority=true after toggle; got false")
	}
}

func TestItemView_Children_TabSelectAndEnterNavigates(t *testing.T) {
	dir := t.TempDir()
	s := store.Store{Dir: dir}

	actorID := "act-human"
	now := time.Now().UTC()
	parentID := "item-a"
	child1ID := "item-b"
	child2ID := "item-c"
	parentPtr := func(s string) *string { return &s }

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
				ID:           parentID,
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				Rank:         "h0",
				Title:        "Parent",
				StatusID:     "todo",
				Priority:     false,
				Archived:     false,
				OwnerActorID: actorID,
				CreatedBy:    actorID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           child1ID,
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				ParentID:     parentPtr(parentID),
				Rank:         "h1",
				Title:        "Child 1",
				StatusID:     "todo",
				Priority:     false,
				Archived:     false,
				OwnerActorID: actorID,
				CreatedBy:    actorID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           child2ID,
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				ParentID:     parentPtr(parentID),
				Rank:         "h2",
				Title:        "Child 2",
				StatusID:     "todo",
				Priority:     false,
				Archived:     false,
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
	m.view = viewItem
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.openItemID = parentID
	m.itemFocus = itemFocusTitle

	// tab to children (title -> status -> assigned -> tags -> priority -> children)
	var mAny tea.Model = m
	for i := 0; i < 5; i++ {
		mAny, _ = mAny.(appModel).Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	m2 := mAny.(appModel)
	if m2.itemFocus != itemFocusChildren {
		t.Fatalf("expected focus=%v, got %v", itemFocusChildren, m2.itemFocus)
	}

	// down selects second child; enter navigates.
	mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyDown})
	m3 := mAny.(appModel)
	mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m4 := mAny.(appModel)
	if strings.TrimSpace(m4.openItemID) != child2ID {
		t.Fatalf("expected openItemID=%q, got %q", child2ID, m4.openItemID)
	}
	if m4.itemFocus != itemFocusTitle {
		t.Fatalf("expected focus reset to title, got %v", m4.itemFocus)
	}
}

func TestItemView_Children_EnterThenBack_ReturnsToParent(t *testing.T) {
	dir := t.TempDir()
	s := store.Store{Dir: dir}

	actorID := "act-human"
	now := time.Now().UTC()
	parentID := "item-a"
	childID := "item-b"
	parentPtr := func(s string) *string { return &s }

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
				ID:           parentID,
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				Rank:         "h0",
				Title:        "Parent",
				StatusID:     "todo",
				Archived:     false,
				OwnerActorID: actorID,
				CreatedBy:    actorID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           childID,
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				ParentID:     parentPtr(parentID),
				Rank:         "h1",
				Title:        "Child",
				StatusID:     "todo",
				Archived:     false,
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
	m.view = viewItem
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.openItemID = parentID
	m.itemFocus = itemFocusChildren

	// enter navigates to child
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := mAny.(appModel)
	if strings.TrimSpace(m2.openItemID) != childID {
		t.Fatalf("expected openItemID=%q, got %q", childID, m2.openItemID)
	}

	// esc goes back to parent (still in item view)
	mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m3 := mAny.(appModel)
	if strings.TrimSpace(m3.openItemID) != parentID {
		t.Fatalf("expected openItemID=%q, got %q", parentID, m3.openItemID)
	}
	if m3.view != viewItem {
		t.Fatalf("expected viewItem, got %v", m3.view)
	}
}

func TestItemView_Parent_EnterThenBack_ReturnsToChild(t *testing.T) {
	dir := t.TempDir()
	s := store.Store{Dir: dir}

	actorID := "act-human"
	now := time.Now().UTC()
	parentID := "item-a"
	childID := "item-b"
	parentPtr := func(s string) *string { return &s }

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
				ID:           parentID,
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				Rank:         "h0",
				Title:        "Parent",
				StatusID:     "todo",
				Archived:     false,
				OwnerActorID: actorID,
				CreatedBy:    actorID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           childID,
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				ParentID:     parentPtr(parentID),
				Rank:         "h1",
				Title:        "Child",
				StatusID:     "todo",
				Archived:     false,
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
	m.view = viewItem
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.openItemID = childID
	m.itemFocus = itemFocusTitle

	// tab to parent (title -> status -> assigned -> tags -> priority -> parent)
	var mAny tea.Model = m
	for i := 0; i < 5; i++ {
		mAny, _ = mAny.(appModel).Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	m2 := mAny.(appModel)
	if m2.itemFocus != itemFocusParent {
		t.Fatalf("expected focus=%v, got %v", itemFocusParent, m2.itemFocus)
	}

	// enter navigates to parent
	mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := mAny.(appModel)
	if strings.TrimSpace(m3.openItemID) != parentID {
		t.Fatalf("expected openItemID=%q, got %q", parentID, m3.openItemID)
	}
	if m3.itemFocus != itemFocusTitle {
		t.Fatalf("expected focus reset to title, got %v", m3.itemFocus)
	}

	// esc goes back to child and focuses Parent
	mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m4 := mAny.(appModel)
	if strings.TrimSpace(m4.openItemID) != childID {
		t.Fatalf("expected openItemID=%q, got %q", childID, m4.openItemID)
	}
	if m4.itemFocus != itemFocusParent {
		t.Fatalf("expected focus=%v, got %v", itemFocusParent, m4.itemFocus)
	}
}

func TestItemView_ChildrenFocus_PTargetsSelectedChild(t *testing.T) {
	dir := t.TempDir()
	s := store.Store{Dir: dir}

	actorID := "act-human"
	now := time.Now().UTC()
	parentID := "item-a"
	child1ID := "item-b"
	child2ID := "item-c"
	parentPtr := func(s string) *string { return &s }

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
				ID:           parentID,
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				Rank:         "h0",
				Title:        "Parent",
				StatusID:     "todo",
				Priority:     false,
				Archived:     false,
				OwnerActorID: actorID,
				CreatedBy:    actorID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           child1ID,
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				ParentID:     parentPtr(parentID),
				Rank:         "h1",
				Title:        "Child 1",
				StatusID:     "todo",
				Priority:     false,
				Archived:     false,
				OwnerActorID: actorID,
				CreatedBy:    actorID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           child2ID,
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				ParentID:     parentPtr(parentID),
				Rank:         "h2",
				Title:        "Child 2",
				StatusID:     "todo",
				Priority:     false,
				Archived:     false,
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
	m.view = viewItem
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.collapsed = map[string]bool{}
	m.openItemID = parentID
	m.itemFocus = itemFocusChildren
	m.itemChildIdx = 1 // select child2

	// p toggles priority for selected child (not the parent).
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	_ = mAny.(appModel)

	db2, err := s.Load()
	if err != nil {
		t.Fatalf("load db: %v", err)
	}
	parent, ok := db2.FindItem(parentID)
	if !ok {
		t.Fatalf("expected parent to exist")
	}
	c2, ok := db2.FindItem(child2ID)
	if !ok {
		t.Fatalf("expected child2 to exist")
	}
	if parent.Priority {
		t.Fatalf("expected parent priority to remain false")
	}
	if !c2.Priority {
		t.Fatalf("expected child2 priority=true after toggle; got false")
	}
}
