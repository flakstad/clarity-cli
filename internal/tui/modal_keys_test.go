package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        tea "github.com/charmbracelet/bubbletea"
)

func TestEditTitle_CtrlS_SavesAndEmitsEvent(t *testing.T) {
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
                        Title:        "Old title",
                        Description:  "",
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

        // Press e => open edit title modal.
        mAny, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
        m2 := mAny.(appModel)
        if m2.modal != modalEditTitle {
                t.Fatalf("expected modalEditTitle, got %v", m2.modal)
        }

        want := "New title"
        m2.input.SetValue(want)
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
        if got := strings.TrimSpace(it.Title); got != want {
                t.Fatalf("expected title=%q, got %q", want, got)
        }

        evs, err := store.ReadEvents(dir, 0)
        if err != nil {
                t.Fatalf("read events: %v", err)
        }
        found := false
        for _, ev := range evs {
                if ev.Type == "item.set_title" && ev.EntityID == "item-a" {
                        found = true
                        if strings.TrimSpace(ev.ActorID) != actorID {
                                t.Fatalf("expected event actor=%q, got %q", actorID, ev.ActorID)
                        }
                }
        }
        if !found {
                t.Fatalf("expected to find item.set_title event for item-a")
        }
}

func TestEditTitle_CtrlG_CancelsWithoutSaving(t *testing.T) {
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
                        Title:        "Old title",
                        Description:  "",
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

        // Open modal.
        mAny, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
        m2 := mAny.(appModel)
        if m2.modal != modalEditTitle {
                t.Fatalf("expected modalEditTitle, got %v", m2.modal)
        }

        m2.input.SetValue("Changed title")
        mAny, _ = m2.updateOutline(tea.KeyMsg{Type: tea.KeyCtrlG})
        m3 := mAny.(appModel)
        if m3.modal != modalNone {
                t.Fatalf("expected modal to close after ctrl+g, got %v", m3.modal)
        }

        loaded, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        it, ok := loaded.FindItem("item-a")
        if !ok {
                t.Fatalf("expected item-a to exist")
        }
        if got := strings.TrimSpace(it.Title); got != "Old title" {
                t.Fatalf("expected title to remain unchanged, got %q", got)
        }

        evs, err := store.ReadEvents(dir, 0)
        if err != nil {
                t.Fatalf("read events: %v", err)
        }
        for _, ev := range evs {
                if ev.Type == "item.set_title" && ev.EntityID == "item-a" {
                        t.Fatalf("did not expect item.set_title event after ctrl+g cancel")
                }
        }
}

func TestAddComment_CtrlG_CancelsWithoutSaving(t *testing.T) {
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
                        Description:  "",
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

        // Press Shift+C => open add comment modal.
        mAny, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
        m2 := mAny.(appModel)
        if m2.modal != modalAddComment {
                t.Fatalf("expected modalAddComment, got %v", m2.modal)
        }

        m2.textarea.SetValue("Hello")
        mAny, _ = m2.updateOutline(tea.KeyMsg{Type: tea.KeyCtrlG})
        m3 := mAny.(appModel)
        if m3.modal != modalNone {
                t.Fatalf("expected modal to close after ctrl+g, got %v", m3.modal)
        }

        loaded, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        if got := len(loaded.Comments); got != 0 {
                t.Fatalf("expected no comments after ctrl+g cancel, got %d", got)
        }
}
