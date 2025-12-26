package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        tea "github.com/charmbracelet/bubbletea"
)

func TestOutlineView_D_SetsDueDate_AndEmitsEvent(t *testing.T) {
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
                        Priority:     false,
                        OnHold:       false,
                        Due:          nil,
                        Schedule:     nil,
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

        // d => open due modal
        mAny, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
        m2 := mAny.(appModel)
        if m2.modal != modalSetDue {
                t.Fatalf("expected modalSetDue, got %v", m2.modal)
        }

        // Set date-only and save.
        m2.yearInput.SetValue("2025")
        m2.monthInput.SetValue("12")
        m2.dayInput.SetValue("31")
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
        if it.Due == nil || strings.TrimSpace(it.Due.Date) != "2025-12-31" || it.Due.Time != nil {
                t.Fatalf("expected due=2025-12-31 (date-only), got %#v", it.Due)
        }

        evs, err := store.ReadEvents(dir, 0)
        if err != nil {
                t.Fatalf("read events: %v", err)
        }
        found := false
        for _, ev := range evs {
                if ev.Type == "item.set_due" && ev.EntityID == "item-a" {
                        found = true
                        if strings.TrimSpace(ev.ActorID) != actorID {
                                t.Fatalf("expected event actor=%q, got %q", actorID, ev.ActorID)
                        }
                }
        }
        if !found {
                t.Fatalf("expected to find item.set_due event for item-a")
        }
}

func TestOutlineView_S_SetsScheduleDateTime_AndC_Clears(t *testing.T) {
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
                        Priority:     false,
                        OnHold:       false,
                        Due:          nil,
                        Schedule:     nil,
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

        // s => open schedule modal
        mAny, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
        m2 := mAny.(appModel)
        if m2.modal != modalSetSchedule {
                t.Fatalf("expected modalSetSchedule, got %v", m2.modal)
        }

        // Enter date + time and save.
        m2.yearInput.SetValue("2025")
        m2.monthInput.SetValue("12")
        m2.dayInput.SetValue("20")
        m2.timeEnabled = true
        m2.hourInput.SetValue("09")
        m2.minuteInput.SetValue("00")
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
        if it.Schedule == nil || strings.TrimSpace(it.Schedule.Date) != "2025-12-20" || it.Schedule.Time == nil || strings.TrimSpace(*it.Schedule.Time) != "09:00" {
                t.Fatalf("expected schedule=2025-12-20 09:00, got %#v", it.Schedule)
        }

        // Re-open and clear via ctrl+c.
        m2 = newAppModel(dir, loaded)
        m2.view = viewOutline
        m2.selectedProjectID = "proj-a"
        m2.selectedOutlineID = "out-a"
        m2.selectedOutline = &loaded.Outlines[0]
        m2.refreshItems(loaded.Outlines[0])
        selectListItemByID(&m2.itemsList, "item-a")
        mAny, _ = m2.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
        m4 := mAny.(appModel)
        if m4.modal != modalSetSchedule {
                t.Fatalf("expected modalSetSchedule, got %v", m4.modal)
        }
        mAny, _ = m4.updateOutline(tea.KeyMsg{Type: tea.KeyCtrlC})
        m5 := mAny.(appModel)
        if m5.modal != modalNone {
                t.Fatalf("expected modal to close after clear, got %v", m5.modal)
        }

        loaded2, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        it2, ok := loaded2.FindItem("item-a")
        if !ok {
                t.Fatalf("expected item-a to exist")
        }
        if it2.Schedule != nil {
                t.Fatalf("expected schedule cleared (nil), got %#v", it2.Schedule)
        }
}

func TestDueModal_JK_AdjustsDay(t *testing.T) {
        dir := t.TempDir()
        s := store.Store{Dir: dir}

        actorID := "act-human"
        now := time.Now().UTC()
        db := &store.DB{
                CurrentActorID: actorID,
                Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "human"}},
                Projects:       []model.Project{{ID: "proj-a", Name: "Project A", CreatedBy: actorID, CreatedAt: now}},
                Outlines:       []model.Outline{{ID: "out-a", ProjectID: "proj-a", StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now}},
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

        m := newAppModel(dir, db)
        m.view = viewOutline
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.selectedOutline = &db.Outlines[0]
        m.refreshItems(db.Outlines[0])
        selectListItemByID(&m.itemsList, "item-a")

        // d => open due modal
        mAny, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
        m2 := mAny.(appModel)
        if m2.modal != modalSetDue {
                t.Fatalf("expected modalSetDue, got %v", m2.modal)
        }
        // Seed a date and then bump day with k (default segment is day).
        m2.yearInput.SetValue("2025")
        m2.monthInput.SetValue("12")
        m2.dayInput.SetValue("31")
        // Focus day field (modal opens focused on year).
        for i := 0; i < 2; i++ {
                mAny, _ = m2.updateOutline(tea.KeyMsg{Type: tea.KeyTab})
                m2 = mAny.(appModel)
        }
        mAny, _ = m2.updateOutline(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
        m3 := mAny.(appModel)
        if gotY, gotM, gotD := strings.TrimSpace(m3.yearInput.Value()), strings.TrimSpace(m3.monthInput.Value()), strings.TrimSpace(m3.dayInput.Value()); gotY != "2026" || gotM != "01" || gotD != "01" {
                t.Fatalf("expected day increment to roll over year; got y=%q m=%q d=%q", gotY, gotM, gotD)
        }
}
