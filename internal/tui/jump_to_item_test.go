package tui

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestJumpToItemByID_OpensItemViewAndSelectsItem(t *testing.T) {
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
                Outlines: []model.Outline{
                        {
                                ID:         "out-a",
                                ProjectID:  "proj-a",
                                StatusDefs: store.DefaultOutlineStatusDefs(),
                                CreatedBy:  actorID,
                                CreatedAt:  now,
                        },
                        {
                                ID:         "out-b",
                                ProjectID:  "proj-a",
                                StatusDefs: store.DefaultOutlineStatusDefs(),
                                CreatedBy:  actorID,
                                CreatedAt:  now,
                        },
                },
                Items: []model.Item{
                        {
                                ID:           "item-a",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-a",
                                Rank:         "h",
                                Title:        "Item A",
                                StatusID:     "todo",
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                        {
                                ID:           "item-b",
                                ProjectID:    "proj-a",
                                OutlineID:    "out-b",
                                Rank:         "h",
                                Title:        "Item B",
                                StatusID:     "todo",
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                },
        }
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.view = viewOutline

        // Should accept both "item-..." and bare "..." ids.
        if err := (&m).jumpToItemByID("b"); err != nil {
                t.Fatalf("jump: %v", err)
        }

        if m.view != viewItem {
                t.Fatalf("expected viewItem, got %v", m.view)
        }
        if m.openItemID != "item-b" {
                t.Fatalf("expected openItemID=item-b, got %q", m.openItemID)
        }
        if m.selectedOutlineID != "out-b" {
                t.Fatalf("expected selectedOutlineID=out-b, got %q", m.selectedOutlineID)
        }

        // Ensure outline selection is also updated so "back" lands on the item.
        if it, ok := m.itemsList.SelectedItem().(outlineRowItem); !ok || it.row.item.ID != "item-b" {
                t.Fatalf("expected selected outline row to be item-b, got %#v", m.itemsList.SelectedItem())
        }
}

func TestJumpToItemByID_FromAgenda_PreservesAgendaReturnView(t *testing.T) {
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
                        Title:        "Item A",
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
        m.view = viewAgenda

        if err := (&m).jumpToItemByID("item-a"); err != nil {
                t.Fatalf("jump: %v", err)
        }
        if m.view != viewItem || m.openItemID != "item-a" {
                t.Fatalf("expected to open item-a in item view, got view=%v openItemID=%q", m.view, m.openItemID)
        }
        if !m.hasReturnView || m.returnView != viewAgenda {
                t.Fatalf("expected returnView=viewAgenda, got hasReturnView=%v returnView=%v", m.hasReturnView, m.returnView)
        }
}
