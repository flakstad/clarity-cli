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

func TestItemView_RendersActivityRowsInList_NoSplitPane(t *testing.T) {
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
		Comments: []model.Comment{{
			ID:        "c1",
			ItemID:    "item-a",
			AuthorID:  actorID,
			Body:      "Comment body",
			CreatedAt: now.Add(1 * time.Minute),
		}},
		Worklog: []model.WorklogEntry{{
			ID:        "w1",
			ItemID:    "item-a",
			AuthorID:  actorID,
			Body:      "Worked on it",
			CreatedAt: now.Add(2 * time.Minute),
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
	// Expand the root and activity roots so rows are visible.
	m.itemCollapsed = map[string]bool{
		"item-a":                         false,
		activityCommentsRootID("item-a"): false,
		activityWorklogRootID("item-a"):  false,
	}
	m.refreshItemSubtree(db.Outlines[0], "item-a")
	selectListItemByID(&m.itemsList, "item-a")

	out := m.viewItem()
	if strings.Contains(out, "Comments   My worklog") {
		t.Fatalf("expected no split-pane activity tabs; got: %q", out)
	}
	if !strings.Contains(out, "Comments (1)") || !strings.Contains(out, "My worklog (1)") {
		t.Fatalf("expected item activity tabs in view; got: %q", out)
	}
}

func TestItemView_TabCollapse_IsIsolatedFromOutlineCollapse(t *testing.T) {
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

	// Outline default: parent collapsed (so child is hidden).
	if m.collapsed == nil || !m.collapsed["item-root"] {
		t.Fatalf("expected outline collapsed[item-root]=true by default")
	}

	// Enter item view (copies collapse state).
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := mAny.(appModel)
	if m2.view != viewItem {
		t.Fatalf("expected viewItem, got %v", m2.view)
	}

	// Toggle collapse in item view (should not affect outline's collapsed map).
	mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3 := mAny.(appModel)
	if m3.itemCollapsed == nil {
		t.Fatalf("expected itemCollapsed to exist")
	}
	if m3.itemCollapsed["item-root"] == m3.collapsed["item-root"] {
		t.Fatalf("expected itemCollapsed[item-root] to diverge from outline collapsed[item-root]")
	}

	// Back to outline: outline collapsed state should remain unchanged.
	mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m4 := mAny.(appModel)
	if m4.view != viewOutline {
		t.Fatalf("expected viewOutline after backspace, got %v", m4.view)
	}
	if !m4.collapsed["item-root"] {
		t.Fatalf("expected outline collapsed[item-root] to remain true after item view tab toggle")
	}
}
