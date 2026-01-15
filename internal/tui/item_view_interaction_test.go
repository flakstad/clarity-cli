package tui

import (
	"strings"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func hasOutlineRowItemID(l list.Model, id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, it := range l.Items() {
		row, ok := it.(outlineRowItem)
		if !ok {
			continue
		}
		if strings.TrimSpace(row.row.item.ID) == id {
			return true
		}
	}
	return false
}

func TestItemView_EnterFromOutline_NarrowsToSubtree(t *testing.T) {
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
				ID:           "item-root",
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				Rank:         "h",
				Title:        "Root",
				StatusID:     "todo",
				OwnerActorID: actorID,
				CreatedBy:    actorID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           "item-child",
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				ParentID:     ptr("item-root"),
				Rank:         "i",
				Title:        "Child",
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
	m.width = 120
	m.height = 40
	m.view = viewOutline
	m.pane = paneOutline
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.refreshItems(db.Outlines[0])
	selectListItemByID(&m.itemsList, "item-root")
	// Ensure root is expanded so the narrowed view also shows the child.
	m.collapsed["item-root"] = false

	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := mAny.(appModel)
	if m2.view != viewItem {
		t.Fatalf("expected viewItem, got %v", m2.view)
	}
	if m2.pane != paneOutline {
		t.Fatalf("expected paneOutline, got %v", m2.pane)
	}
	if got := strings.TrimSpace(m2.openItemID); got != "item-root" {
		t.Fatalf("expected openItemID=item-root, got %q", got)
	}
	if !hasOutlineRowItemID(m2.itemsList, "item-child") {
		t.Fatalf("expected subtree list to include item-child")
	}
}

func TestItemView_EnterNarrowsFurther_BackspaceWidens(t *testing.T) {
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
				ID:           "item-root",
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				Rank:         "h",
				Title:        "Root",
				StatusID:     "todo",
				OwnerActorID: actorID,
				CreatedBy:    actorID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           "item-child",
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				ParentID:     ptr("item-root"),
				Rank:         "i",
				Title:        "Child",
				StatusID:     "todo",
				OwnerActorID: actorID,
				CreatedBy:    actorID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           "item-grand",
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				ParentID:     ptr("item-child"),
				Rank:         "j",
				Title:        "Grandchild",
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
	m.width = 120
	m.height = 40
	m.view = viewOutline
	m.pane = paneOutline
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.refreshItems(db.Outlines[0])
	selectListItemByID(&m.itemsList, "item-root")
	// Ensure root is expanded so we can select the child before narrowing further.
	m.collapsed["item-root"] = false

	// Enter: open narrowed item view.
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := mAny.(appModel)

	// Move selection down to child.
	for i := 0; i < 10; i++ {
		if strings.TrimSpace(selectedOutlineListItemID(&m2.itemsList)) == "item-child" {
			break
		}
		mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyDown})
		m2 = mAny.(appModel)
	}
	if got := strings.TrimSpace(selectedOutlineListItemID(&m2.itemsList)); got != "item-child" {
		t.Fatalf("expected selection on item-child, got %q", got)
	}

	// Enter again: narrow to child subtree.
	mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := mAny.(appModel)
	if got := strings.TrimSpace(m3.openItemID); got != "item-child" {
		t.Fatalf("expected openItemID=item-child after narrow, got %q", got)
	}
	if len(m3.itemNavStack) != 1 {
		t.Fatalf("expected nav stack size 1, got %d", len(m3.itemNavStack))
	}

	// Backspace: widen back to root, selecting the child we came from.
	mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m4 := mAny.(appModel)
	if got := strings.TrimSpace(m4.openItemID); got != "item-root" {
		t.Fatalf("expected openItemID=item-root after widen, got %q", got)
	}
	if got := strings.TrimSpace(selectedOutlineListItemID(&m4.itemsList)); got != "item-child" {
		t.Fatalf("expected selection restored to item-child, got %q", got)
	}
}

func TestItemView_CtrlO_TogglesActivityPane(t *testing.T) {
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
	m.view = viewItem
	m.pane = paneOutline
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.openItemID = "item-a"
	m.refreshItemSubtree(db.Outlines[0], "item-a")
	selectListItemByID(&m.itemsList, "item-a")

	// ctrl+x then o switches focus to the activity panel ("other window").
	m2 := m
	mAny, _ := m2.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	mAny, _ = mAny.(appModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m3 := mAny.(appModel)
	if m3.pane != paneDetail {
		t.Fatalf("expected paneDetail after ctrl+x o, got %v", m3.pane)
	}
	if m3.itemFocus != itemFocusComments {
		t.Fatalf("expected focus=comments after ctrl+x o, got %v", m3.itemFocus)
	}

	// tab cycles focus to Worklog.
	mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyTab})
	m4 := mAny.(appModel)
	if m4.itemFocus != itemFocusWorklog {
		t.Fatalf("expected focus=worklog after tab, got %v", m4.itemFocus)
	}
	out := m4.viewItem()
	if !strings.Contains(out, "Comments") || !strings.Contains(out, "My worklog") || !strings.Contains(out, "History") {
		t.Fatalf("expected item activity tabs in view; got: %q", out)
	}
}
