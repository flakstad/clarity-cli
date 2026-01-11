package tui

import (
	"strings"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCapture_Draft_Esc_ShowsConfirmExitModal(t *testing.T) {
	m := captureModel{phase: capturePhaseEditDraft}
	mAny, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := mAny.(captureModel)
	if cmd != nil {
		t.Fatalf("expected no cmd when opening confirm modal")
	}
	if m2.modal != captureModalConfirmExit {
		t.Fatalf("expected captureModalConfirmExit, got %v", m2.modal)
	}
}

func TestCapture_Draft_ConfirmExit_Enter_CancelsCapture(t *testing.T) {
	m := captureModel{phase: capturePhaseEditDraft, modal: captureModalConfirmExit}
	mAny, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := mAny.(captureModel)
	if m2.modal != captureModalNone {
		t.Fatalf("expected modal to close, got %v", m2.modal)
	}
	if cmd == nil {
		t.Fatalf("expected finish cmd")
	}
	msg := cmd()
	fin, ok := msg.(captureFinishedMsg)
	if !ok {
		t.Fatalf("expected captureFinishedMsg, got %T", msg)
	}
	if !fin.canceled {
		t.Fatalf("expected canceled=true")
	}
}

func TestCapture_Draft_CtrlS_SubmitsDraft(t *testing.T) {
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
			Archived:  false,
		}},
		Outlines: []model.Outline{{
			ID:         "out-a",
			ProjectID:  "proj-a",
			StatusDefs: store.DefaultOutlineStatusDefs(),
			CreatedBy:  actorID,
			CreatedAt:  now,
			Archived:   false,
		}},
	}
	if err := s.Save(db); err != nil {
		t.Fatalf("save db: %v", err)
	}

	m := captureModel{
		phase:          capturePhaseEditDraft,
		workspace:      "ws",
		dir:            dir,
		st:             s,
		db:             db,
		actorOverride:  actorID,
		draftOutlineID: "out-a",
		draftItems: []model.Item{{
			ID:        "draft-1",
			ProjectID: "proj-a",
			OutlineID: "out-a",
			Rank:      "a",
			Title:     "Captured item",
			StatusID:  "todo",
		}},
	}

	mAny, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m2 := mAny.(captureModel)
	if cmd == nil {
		t.Fatalf("expected finish cmd")
	}
	msg := cmd()
	fin, ok := msg.(captureFinishedMsg)
	if !ok {
		t.Fatalf("expected captureFinishedMsg, got %T", msg)
	}
	if fin.canceled {
		t.Fatalf("expected canceled=false")
	}
	if strings.TrimSpace(fin.result.ItemID) == "" {
		t.Fatalf("expected result item id")
	}
	if strings.TrimSpace(m2.result.ItemID) == "" {
		t.Fatalf("expected model result item id")
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("load db: %v", err)
	}
	it, ok := loaded.FindItem(fin.result.ItemID)
	if !ok || it == nil {
		t.Fatalf("expected created item %q to exist", fin.result.ItemID)
	}
	if got := strings.TrimSpace(it.Title); got != "Captured item" {
		t.Fatalf("expected title=%q, got %q", "Captured item", got)
	}
}
