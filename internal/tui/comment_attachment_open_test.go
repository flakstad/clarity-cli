package tui

import (
	"strings"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func TestItemComment_OpenLinksPicker_IncludesCommentAttachments(t *testing.T) {
	dir := t.TempDir()

	actorID := "act-human"
	now := time.Now().UTC()

	commentID := "c1"
	attID := "att-abc123"

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
			ID:        commentID,
			ItemID:    "item-a",
			AuthorID:  actorID,
			Body:      "Comment body",
			CreatedAt: now.Add(1 * time.Minute),
		}},
		Attachments: []model.Attachment{{
			ID:           attID,
			EntityKind:   "comment",
			EntityID:     commentID,
			OriginalName: "file.txt",
			SizeBytes:    1,
			Path:         "attachments/file.txt",
			CreatedBy:    actorID,
			CreatedAt:    now.Add(2 * time.Minute),
			UpdatedAt:    now.Add(2 * time.Minute),
		}},
	}

	m := newAppModel(dir, db)
	m.view = viewItem
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.openItemID = "item-a"
	m.pane = paneDetail
	m.itemFocus = itemFocusComments
	m.itemCommentIdx = 0
	m.refreshItemSubtree(db.Outlines[0], "item-a")
	selectListItemByID(&m.itemsList, "item-a")

	// enter => open comment modal (view entry) with attachments included.
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := mAny.(appModel)
	if m2.modal != modalViewEntry {
		t.Fatalf("expected modalViewEntry, got %v", m2.modal)
	}
	if !strings.Contains(m2.viewModalBody, attID) {
		t.Fatalf("expected view modal body to include attachment id %q; got %q", attID, m2.viewModalBody)
	}

	// l => open link/attachment picker.
	mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m3 := mAny.(appModel)
	if m3.modal != modalPickTargets {
		t.Fatalf("expected modalPickTargets, got %v", m3.modal)
	}

	found := false
	for _, t0 := range m3.targetPickTargets {
		if t0.Kind == targetPickTargetAttachment && strings.TrimSpace(t0.Target) == attID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected target picker to include attachment %q; got %+v", attID, m3.targetPickTargets)
	}
}
