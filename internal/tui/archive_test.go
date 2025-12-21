package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/bubbles/list"
        tea "github.com/charmbracelet/bubbletea"
)

func TestArchiveItemTree_UsesEditActorID_AllowsArchivingHumanOwnedItemWhenCurrentActorIsAgent(t *testing.T) {
        dir := t.TempDir()
        s := store.Store{Dir: dir}

        humanID := "act-human"
        agentID := "act-agent"

        now := time.Now().UTC()
        db := &store.DB{
                CurrentActorID: humanID, // will override below after actors exist
                Actors: []model.Actor{
                        {ID: humanID, Kind: model.ActorKindHuman, Name: "human"},
                        {ID: agentID, Kind: model.ActorKindAgent, Name: "agent", UserID: &humanID},
                },
                Projects: []model.Project{{
                        ID:        "proj-a",
                        Name:      "Project A",
                        CreatedBy: humanID,
                        CreatedAt: now,
                        Archived:  false,
                }},
                Outlines: []model.Outline{{
                        ID:         "out-a",
                        ProjectID:  "proj-a",
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  humanID,
                        CreatedAt:  now,
                        Archived:   false,
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
                        Archived:     false,
                        OwnerActorID: humanID,
                        CreatedBy:    humanID,
                        CreatedAt:    now,
                        UpdatedAt:    now,
                }},
        }
        // Simulate "current actor is agent" (common after `clarity agent start ...`).
        db.CurrentActorID = agentID

        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        n, err := m.archiveItemTree("item-a")
        if err != nil {
                t.Fatalf("archiveItemTree: %v", err)
        }
        if n != 1 {
                t.Fatalf("expected archived count=1, got %d", n)
        }

        loaded, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        it, ok := loaded.FindItem("item-a")
        if !ok {
                t.Fatalf("expected item to exist")
        }
        if !it.Archived {
                t.Fatalf("expected item archived=true")
        }

        // Ensure we attribute the archive event to the human actor (editActorID behavior).
        evs, err := store.ReadEvents(dir, 0)
        if err != nil {
                t.Fatalf("read events: %v", err)
        }
        found := false
        for _, ev := range evs {
                if ev.Type == "item.archive" && ev.EntityID == "item-a" {
                        found = true
                        if strings.TrimSpace(ev.ActorID) != humanID {
                                t.Fatalf("expected archive event actor=%q, got %q", humanID, ev.ActorID)
                        }
                }
        }
        if !found {
                t.Fatalf("expected to find item.archive event for item-a")
        }
}

func TestUpdate_R_FromItemView_ArchivesAndReturnsToOutline(t *testing.T) {
        dir := t.TempDir()
        s := store.Store{Dir: dir}

        humanID := "act-human"
        now := time.Now().UTC()
        db := &store.DB{
                CurrentActorID: humanID,
                Actors:         []model.Actor{{ID: humanID, Kind: model.ActorKindHuman, Name: "human"}},
                Projects: []model.Project{{
                        ID:        "proj-a",
                        Name:      "Project A",
                        CreatedBy: humanID,
                        CreatedAt: now,
                        Archived:  false,
                }},
                Outlines: []model.Outline{{
                        ID:         "out-a",
                        ProjectID:  "proj-a",
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  humanID,
                        CreatedAt:  now,
                        Archived:   false,
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
                        Archived:     false,
                        OwnerActorID: humanID,
                        CreatedBy:    humanID,
                        CreatedAt:    now,
                        UpdatedAt:    now,
                }},
        }
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.view = viewItem
        m.openItemID = "item-a"
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.selectedOutline = &db.Outlines[0]

        // Seed itemsList so archive confirmation can compute "nearest selectable" without panicking.
        m.itemsList.SetItems([]list.Item{
                outlineRowItem{row: outlineRow{item: db.Items[0]}, outline: db.Outlines[0]},
                addItemRow{},
        })
        m.itemsList.Select(0)

        // Press 'r' => opens confirm modal for the open item.
        m2Any, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
        m2 := m2Any.(appModel)
        if m2.modal != modalConfirmArchive {
                t.Fatalf("expected modalConfirmArchive, got %v", m2.modal)
        }
        if m2.archiveFor != archiveTargetItem {
                t.Fatalf("expected archiveTargetItem, got %v", m2.archiveFor)
        }
        if strings.TrimSpace(m2.modalForID) != "item-a" {
                t.Fatalf("expected modalForID=item-a, got %q", m2.modalForID)
        }

        // Confirm => archives and returns to outline view.
        m3Any, _ := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
        m3 := m3Any.(appModel)
        if m3.view != viewOutline {
                t.Fatalf("expected viewOutline, got %v", m3.view)
        }
        if strings.TrimSpace(m3.openItemID) != "" {
                t.Fatalf("expected openItemID cleared, got %q", m3.openItemID)
        }

        loaded, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        it, ok := loaded.FindItem("item-a")
        if !ok {
                t.Fatalf("expected item to exist")
        }
        if !it.Archived {
                t.Fatalf("expected item archived=true")
        }
}
