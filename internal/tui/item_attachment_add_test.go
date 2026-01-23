package tui

import (
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func TestItemView_AttachFileKey_OpensAttachmentPickerForItem(t *testing.T) {
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
	m.attachmentFilePickerLastDir = dir

	if m.collapsed == nil {
		m.collapsed = map[string]bool{}
	}
	m.itemCollapsed = map[string]bool{
		"item-a":                         false,
		activityCommentsRootID("item-a"): false,
	}
	m.refreshItemSubtree(db.Outlines[0], "item-a")
	selectListItemByID(&m.itemsList, "item-a")

	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m2 := mAny.(appModel)

	if m2.modal != modalPickAttachmentFile {
		t.Fatalf("expected modalPickAttachmentFile, got %v", m2.modal)
	}
	if m2.attachmentAddKind != "item" {
		t.Fatalf("expected attachmentAddKind %q, got %q", "item", m2.attachmentAddKind)
	}
	if m2.attachmentAddEntityID != "item-a" {
		t.Fatalf("expected attachmentAddEntityID %q, got %q", "item-a", m2.attachmentAddEntityID)
	}
}
