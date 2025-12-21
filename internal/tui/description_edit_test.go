package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        tea "github.com/charmbracelet/bubbletea"
)

func TestEditDescription_D_CtrlS_SavesAndEmitsEvent(t *testing.T) {
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
        m.view = viewOutline
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.selectedOutline = &db.Outlines[0]
        m.refreshItems(db.Outlines[0])
        selectListItemByID(&m.itemsList, "item-a")

        // Press Shift+D => open edit description modal.
        mAny, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
        m2 := mAny.(appModel)
        if m2.modal != modalEditDescription {
                t.Fatalf("expected modalEditDescription, got %v", m2.modal)
        }
        if got := strings.TrimSpace(m2.textarea.Value()); got != "Old description" {
                t.Fatalf("expected textarea to be seeded with existing description; got %q", got)
        }

        // Update the textarea value then save via ctrl+s.
        want := "New description\n\n- bullet 1\n- bullet 2"
        m2.textarea.SetValue(want)
        mAny, _ = m2.updateOutline(tea.KeyMsg{Type: tea.KeyCtrlS})
        m3 := mAny.(appModel)
        if m3.modal != modalNone {
                t.Fatalf("expected modal to close after save, got %v", m3.modal)
        }

        loaded, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        it, ok := loaded.FindItem("item-a")
        if !ok {
                t.Fatalf("expected item-a to exist")
        }
        if got := strings.TrimSpace(it.Description); got != want {
                t.Fatalf("expected description=%q, got %q", want, got)
        }

        evs, err := store.ReadEvents(dir, 0)
        if err != nil {
                t.Fatalf("read events: %v", err)
        }
        found := false
        for _, ev := range evs {
                if ev.Type == "item.set_description" && ev.EntityID == "item-a" {
                        found = true
                        if strings.TrimSpace(ev.ActorID) != actorID {
                                t.Fatalf("expected event actor=%q, got %q", actorID, ev.ActorID)
                        }
                }
        }
        if !found {
                t.Fatalf("expected to find item.set_description event for item-a")
        }
}
