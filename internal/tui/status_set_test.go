package tui

import (
	"strings"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

func TestStatusPicker_Enter_SetsStatus_WhenCurrentActorIsAgent(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()

	humanID := "act-human"
	agentID := "act-agent"

	db := &store.DB{
		CurrentActorID: agentID, // simulate being "stuck" on an agent actor
		Actors: []model.Actor{
			{ID: humanID, Kind: model.ActorKindHuman, Name: "human"},
			{ID: agentID, Kind: model.ActorKindAgent, Name: "agent", UserID: &humanID},
		},
		Projects: []model.Project{{ID: "proj-a", Name: "P", CreatedBy: humanID, CreatedAt: now}},
		Outlines: []model.Outline{{
			ID:        "out-a",
			ProjectID: "proj-a",
			StatusDefs: []model.OutlineStatusDef{
				{ID: "todo", Label: "TODO", IsEndState: false},
				{ID: "doing", Label: "DOING", IsEndState: false},
				{ID: "done", Label: "DONE", IsEndState: true},
			},
			CreatedBy: humanID,
			CreatedAt: now,
		}},
		Items: []model.Item{{
			ID:           "item-a",
			ProjectID:    "proj-a",
			OutlineID:    "out-a",
			Rank:         "h",
			Title:        "A",
			StatusID:     "todo",
			OwnerActorID: humanID, // owned by human
			CreatedBy:    humanID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}},
	}

	s := store.Store{Dir: dir}
	if err := s.Save(db); err != nil {
		t.Fatalf("save db: %v", err)
	}

	m := newAppModel(dir, db)
	m.view = viewOutline
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.selectedOutline = &db.Outlines[0]
	m.refreshItems(db.Outlines[0])
	selectListItemByID(&m.itemsList, "item-a")

	// Open the status picker (as SPACE does).
	it, _ := m.itemsList.SelectedItem().(outlineRowItem)
	m.openStatusPicker(it.outline, it.row.item.ID, it.row.item.StatusID)
	m.modal = modalPickStatus
	m.modalForID = it.row.item.ID

	// Select "DOING" option.
	// opts are: (no status), TODO, DOING, DONE
	m.statusList.Select(2)

	mm, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := mm.(appModel)

	// Reload from disk to ensure persisted change.
	db2, err := s.Load()
	if err != nil {
		t.Fatalf("load db: %v", err)
	}
	item2, ok := db2.FindItem("item-a")
	if !ok {
		t.Fatalf("expected item-a to exist")
	}
	if item2.StatusID != "doing" {
		t.Fatalf("expected status to be set to doing; got %q (minibuffer=%q)", item2.StatusID, m2.minibufferText)
	}
	if got := strings.TrimSpace(m2.minibufferText); got == "" || !strings.Contains(got, "Status:") {
		t.Fatalf("expected minibuffer status confirmation; got %q", m2.minibufferText)
	}

	// Ensure the status transition was recorded in the event log with from/to.
	evs, err := store.ReadEventsForEntity(dir, "item-a", 0)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	found := false
	for _, ev := range evs {
		if ev.Type != "item.set_status" {
			continue
		}
		payload, ok := ev.Payload.(map[string]any)
		if !ok {
			t.Fatalf("expected payload object; got %#v", ev.Payload)
		}
		if got, _ := payload["from"].(string); got != "todo" {
			t.Fatalf("expected from=todo; got %q (payload=%#v)", got, payload)
		}
		if got, _ := payload["to"].(string); got != "doing" {
			t.Fatalf("expected to=doing; got %q (payload=%#v)", got, payload)
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("expected an item.set_status event for item-a; got %d events", len(evs))
	}

	// Ensure history is available via the History modal ("H", discoverable via action panel).
	m2.view = viewItem
	m2.selectedProjectID = "proj-a"
	m2.selectedOutlineID = "out-a"
	m2.openItemID = "item-a"
	m2.width = 120
	m2.height = 80
	m2.pane = paneOutline
	m2.refreshItemSubtree(db.Outlines[0], "item-a")
	selectListItemByID(&m2.itemsList, "item-a")
	mAny, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("H")})
	m3 := mAny.(appModel)
	if m3.modal != modalActivityList {
		t.Fatalf("expected modalActivityList after H, got %v", m3.modal)
	}
	if m3.activityModalKind != activityModalKindHistory {
		t.Fatalf("expected activityModalKindHistory, got %v", m3.activityModalKind)
	}

	historyFound := false
	for _, it := range m3.activityModalList.Items() {
		row, ok := it.(activityModalRowItem)
		if !ok {
			continue
		}
		if strings.Contains(row.title, "set status:") && strings.Contains(row.title, "todo") && strings.Contains(row.title, "doing") {
			historyFound = true
			break
		}
	}
	if !historyFound {
		t.Fatalf("expected status transition in history list; got %d rows", len(m3.activityModalList.Items()))
	}

	// Enter opens the selected history entry and ESC returns to the list.
	mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m4 := mAny.(appModel)
	if m4.modal != modalViewEntry {
		t.Fatalf("expected modalViewEntry after enter in history list, got %v", m4.modal)
	}
	if m4.viewModalReturn != modalActivityList {
		t.Fatalf("expected viewModalReturn to be modalActivityList, got %v", m4.viewModalReturn)
	}
	mAny, _ = m4.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m5 := mAny.(appModel)
	if m5.modal != modalActivityList {
		t.Fatalf("expected modalActivityList after esc in view entry, got %v", m5.modal)
	}
}
