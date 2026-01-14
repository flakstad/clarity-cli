package tui

import (
	"strings"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func newKeybindingContractModel(t *testing.T) (dir string, m appModel) {
	t.Helper()

	dir = t.TempDir()
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

	m = newAppModel(dir, db)
	m.width = 120
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	if o, ok := m.db.FindOutline("out-a"); ok && o != nil {
		m.selectedOutline = o
		m.refreshItems(*o)
	}
	selectListItemByID(&m.itemsList, "item-a")

	return dir, m
}

func TestKeybindingContract_OutlineView_A_Assigns_And_a_OpensAgenda(t *testing.T) {
	_, m := newKeybindingContractModel(t)
	m.view = viewOutline
	m.pane = paneOutline
	m.modal = modalNone

	// A = assign (direct shortcut).
	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m2 := mAny.(appModel)
	if m2.modal != modalPickAssignee {
		t.Fatalf("expected modalPickAssignee after A; got %v", m2.modal)
	}

	// a = agenda (global opener).
	m3 := m
	mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m4 := mAny.(appModel)
	if m4.modal != modalActionPanel {
		t.Fatalf("expected modalActionPanel after a; got %v", m4.modal)
	}
	if got := m4.curActionPanelKind(); got != actionPanelAgenda {
		t.Fatalf("expected actionPanelAgenda after a; got %v", got)
	}
}

func TestKeybindingContract_ItemView_A_Assigns_And_a_OpensAgenda(t *testing.T) {
	_, m := newKeybindingContractModel(t)
	m.view = viewItem
	m.openItemID = "item-a"
	m.modal = modalNone

	mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m2 := mAny.(appModel)
	if m2.modal != modalPickAssignee {
		t.Fatalf("expected modalPickAssignee after A; got %v", m2.modal)
	}

	m3 := m
	mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m4 := mAny.(appModel)
	if m4.modal != modalActionPanel {
		t.Fatalf("expected modalActionPanel after a; got %v", m4.modal)
	}
	if got := m4.curActionPanelKind(); got != actionPanelAgenda {
		t.Fatalf("expected actionPanelAgenda after a; got %v", got)
	}
}

func TestKeybindingContract_ActionPanel_Shows_A_Assign_And_a_Agenda(t *testing.T) {
	_, m := newKeybindingContractModel(t)
	m.view = viewOutline
	m.pane = paneOutline

	m.openActionPanel(actionPanelContext)
	out := m.renderActionPanel()

	// Agenda is a global opener in the action panel root.
	if !strings.Contains(out, "a            Agenda Commands") {
		t.Fatalf("expected action panel to show lowercase 'a' for agenda; got:\n%s", out)
	}
	// Assign is uppercase A in item contexts.
	if !strings.Contains(out, "A            Assign") {
		t.Fatalf("expected action panel to show uppercase 'A' for assign; got:\n%s", out)
	}
}

