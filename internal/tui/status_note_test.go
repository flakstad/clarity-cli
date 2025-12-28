package tui

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        tea "github.com/charmbracelet/bubbletea"
)

func TestStatusPicker_Enter_OpensNoteModal_WhenStatusRequiresNote(t *testing.T) {
        dir := t.TempDir()
        now := time.Now().UTC()

        actorID := "act-human"

        db := &store.DB{
                CurrentActorID: actorID,
                Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "human"}},
                Projects:       []model.Project{{ID: "proj-a", Name: "P", CreatedBy: actorID, CreatedAt: now}},
                Outlines: []model.Outline{{
                        ID:        "out-a",
                        ProjectID: "proj-a",
                        StatusDefs: []model.OutlineStatusDef{
                                {ID: "todo", Label: "TODO"},
                                {ID: "doing", Label: "DOING", RequiresNote: true},
                                {ID: "done", Label: "DONE", IsEndState: true},
                        },
                        CreatedBy: actorID,
                        CreatedAt: now,
                }},
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj-a",
                        OutlineID:    "out-a",
                        Rank:         "h",
                        Title:        "A",
                        StatusID:     "todo",
                        OwnerActorID: actorID,
                        CreatedBy:    actorID,
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

        // Open status picker and select "doing" (requires note).
        it, _ := m.itemsList.SelectedItem().(outlineRowItem)
        m.openStatusPicker(it.outline, it.row.item.ID, it.row.item.StatusID)
        m.modal = modalPickStatus
        m.modalForID = it.row.item.ID
        // opts are: (no status), TODO, DOING, DONE
        m.statusList.Select(2)

        mAny, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyEnter})
        m2 := mAny.(appModel)
        if m2.modal != modalStatusNote {
                t.Fatalf("expected modalStatusNote, got %v", m2.modal)
        }
        if got := m2.modalForKey; got != "doing" {
                t.Fatalf("expected modalForKey=doing, got %q", got)
        }

        // Status should not be persisted yet.
        db2, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        item2, _ := db2.FindItem("item-a")
        if got := item2.StatusID; got != "todo" {
                t.Fatalf("expected status still todo before note save; got %q", got)
        }

        // Save note => applies status change.
        m2.textarea.SetValue("Because…")
        mAny, _ = m2.updateOutline(tea.KeyMsg{Type: tea.KeyCtrlS})
        m3 := mAny.(appModel)
        if m3.modal != modalNone {
                t.Fatalf("expected modal to close after save, got %v", m3.modal)
        }

        db3, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        item3, _ := db3.FindItem("item-a")
        if got := item3.StatusID; got != "doing" {
                t.Fatalf("expected status doing after note save; got %q", got)
        }

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
                if got, _ := payload["note"].(string); got != "Because…" {
                        t.Fatalf("expected payload.note to be saved; got %#v", payload)
                }
                found = true
                break
        }
        if !found {
                t.Fatalf("expected item.set_status event with note")
        }
}
