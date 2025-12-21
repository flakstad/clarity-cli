package tui

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        tea "github.com/charmbracelet/bubbletea"
)

func TestAgendaView_ShowsAllNonDoneItemsAcrossWorkspace(t *testing.T) {
        dir := t.TempDir()
        s := store.Store{Dir: dir}

        actorID := "act-human"
        now := time.Now().UTC()

        db := &store.DB{
                CurrentActorID: actorID,
                Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "human"}},
                Projects: []model.Project{
                        {ID: "proj-a", Name: "Alpha", CreatedBy: actorID, CreatedAt: now},
                        {ID: "proj-b", Name: "Beta", CreatedBy: actorID, CreatedAt: now},
                },
                Outlines: []model.Outline{
                        {ID: "out-a", ProjectID: "proj-a", StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now},
                        {ID: "out-b", ProjectID: "proj-b", StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now},
                },
                Items: []model.Item{
                        {ID: "item-todo", ProjectID: "proj-a", OutlineID: "out-a", Rank: "h", Title: "Keep me", StatusID: "todo", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                        {ID: "item-done", ProjectID: "proj-a", OutlineID: "out-a", Rank: "i", Title: "Hide me", StatusID: "done", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                        {ID: "item-empty", ProjectID: "proj-b", OutlineID: "out-b", Rank: "h", Title: "Also keep", StatusID: "", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                },
        }
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        if m.view != viewProjects {
                t.Fatalf("expected start viewProjects, got %v", m.view)
        }

        // Open agenda commands, then run 't' (list all TODO entries).
        mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
        m2 := mAny.(appModel)
        if m2.modal != modalActionPanel || m2.curActionPanelKind() != actionPanelAgenda {
                t.Fatalf("expected agenda commands panel, got modal=%v kind=%v", m2.modal, m2.curActionPanelKind())
        }
        mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
        m2 = mAny.(appModel)
        if m2.view != viewAgenda {
                t.Fatalf("expected viewAgenda after a then t, got %v", m2.view)
        }

        all := m2.agendaList.Items()
        rows := 0
        for _, it := range all {
                r, ok := it.(agendaRowItem)
                if !ok {
                        continue
                }
                rows++
                if r.row.item.ID == "item-done" {
                        t.Fatalf("did not expect done item in agenda")
                }
        }
        if rows != 2 {
                t.Fatalf("expected 2 agenda rows (non-done), got %d (total list items=%d)", rows, len(all))
        }
}

func TestAgendaView_EnterOpensItem_BackReturnsToAgenda_AndAgendaBackReturnsPreviousView(t *testing.T) {
        dir := t.TempDir()
        s := store.Store{Dir: dir}

        actorID := "act-human"
        now := time.Now().UTC()

        db := &store.DB{
                CurrentActorID: actorID,
                Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "human"}},
                Projects:       []model.Project{{ID: "proj-a", Name: "Alpha", CreatedBy: actorID, CreatedAt: now}},
                Outlines:       []model.Outline{{ID: "out-a", ProjectID: "proj-a", StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now}},
                Items:          []model.Item{{ID: "item-todo", ProjectID: "proj-a", OutlineID: "out-a", Rank: "h", Title: "Keep me", StatusID: "todo", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now}},
        }
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)

        // Open agenda from projects view; should remember return view.
        mAny, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
        m2 := mAny.(appModel)
        mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
        m2 = mAny.(appModel)
        if m2.view != viewAgenda {
                t.Fatalf("expected viewAgenda after a then t, got %v", m2.view)
        }
        if !m2.hasAgendaReturnView || m2.agendaReturnView != viewProjects {
                t.Fatalf("expected agendaReturnView=viewProjects, got has=%v return=%v", m2.hasAgendaReturnView, m2.agendaReturnView)
        }

        // Enter opens item.
        mAny, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
        m3 := mAny.(appModel)
        if m3.view != viewItem {
                t.Fatalf("expected viewItem, got %v", m3.view)
        }
        if m3.openItemID != "item-todo" {
                t.Fatalf("expected openItemID=item-todo, got %q", m3.openItemID)
        }
        if !m3.hasReturnView || m3.returnView != viewAgenda {
                t.Fatalf("expected returnView=viewAgenda, got has=%v return=%v", m3.hasReturnView, m3.returnView)
        }

        // Back from item returns to agenda.
        mAny, _ = m3.Update(tea.KeyMsg{Type: tea.KeyEsc})
        m4 := mAny.(appModel)
        if m4.view != viewAgenda {
                t.Fatalf("expected viewAgenda after back from item, got %v", m4.view)
        }

        // Back from agenda returns to previous view (projects).
        mAny, _ = m4.Update(tea.KeyMsg{Type: tea.KeyBackspace})
        m5 := mAny.(appModel)
        if m5.view != viewProjects {
                t.Fatalf("expected viewProjects after back from agenda, got %v", m5.view)
        }
}
