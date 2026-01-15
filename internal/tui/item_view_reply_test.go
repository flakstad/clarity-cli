package tui

import (
	"os"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func TestItemView_ReplyOpensReplyModalAndSetsReplyTo(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	actorID := "act-human"

	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	db := &store.DB{
		CurrentActorID: actorID,
		Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "A"}},
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
			Body:      "Root comment",
			CreatedAt: now.Add(1 * time.Minute),
		}},
	}

	s := store.Store{Dir: dir}
	if err := s.Save(db); err != nil {
		t.Fatalf("save db: %v", err)
	}

	m := newAppModel(dir, db)
	m.view = viewItem
	m.pane = paneDetail
	if m.itemsListActive != nil {
		*m.itemsListActive = false
	}
	m.itemFocus = itemFocusComments
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.openItemID = "item-a"
	m.refreshItemSubtree(db.Outlines[0], "item-a")

	// "R" should open the reply modal with modalForKey set to the parent comment id.
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m2 := mAny.(appModel)
	if m2.modal != modalReplyComment {
		t.Fatalf("expected modalReplyComment, got %v", m2.modal)
	}
	if m2.modalForKey != "c1" {
		t.Fatalf("expected modalForKey=c1, got %q", m2.modalForKey)
	}

	// Saving should create a reply comment (ReplyToCommentID set).
	m2.textFocus = textFocusBody
	m2.textarea.SetValue("Reply body")
	mAny, _ = m2.updateOutline(tea.KeyMsg{Type: tea.KeyCtrlS})
	m3 := mAny.(appModel)
	if m3.modal != modalNone {
		t.Fatalf("expected modal closed after save, got %v", m3.modal)
	}
	if m3.db == nil {
		t.Fatalf("expected db loaded")
	}
	comments := m3.db.CommentsForItem("item-a")
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}
	var reply *model.Comment
	for i := range comments {
		if comments[i].ID != "c1" {
			reply = &comments[i]
			break
		}
	}
	if reply == nil || reply.ReplyToCommentID == nil || *reply.ReplyToCommentID != "c1" {
		t.Fatalf("expected reply to c1; got %+v", reply)
	}
}
