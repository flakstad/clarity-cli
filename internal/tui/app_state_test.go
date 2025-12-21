package tui

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestNewAppModel_RestoresLastTUIState_ItemView(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 21, 0, 0, 0, 0, time.UTC)

        actorID := "act-test"
        projectID := "proj-test"
        outlineID := "out-test"
        itemID := "item-test"

        db := &store.DB{
                Version:          1,
                CurrentActorID:   actorID,
                CurrentProjectID: projectID,
                NextIDs:          map[string]int{},
                Actors:           []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "Test"}},
                Projects:         []model.Project{{ID: projectID, Name: "P", CreatedBy: actorID, CreatedAt: now}},
                Outlines:         []model.Outline{{ID: outlineID, ProjectID: projectID, Name: nil, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now}},
                Items: []model.Item{
                        {
                                ID:           itemID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                ParentID:     nil,
                                Rank:         "h",
                                Title:        "T",
                                Description:  "",
                                StatusID:     "todo",
                                Priority:     false,
                                OnHold:       false,
                                Archived:     false,
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                },
                Deps:     []model.Dependency{},
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        // Seed saved TUI state in the workspace/store dir.
        s := store.Store{Dir: dir}
        if err := s.SaveTUIState(&store.TUIState{
                View:              "item",
                SelectedProjectID: projectID,
                SelectedOutlineID: outlineID,
                OpenItemID:        itemID,
                ReturnView:        "outline",
        }); err != nil {
                t.Fatalf("seed SaveTUIState: %v", err)
        }

        m := newAppModel(dir, db)
        if m.view != viewItem {
                t.Fatalf("expected viewItem; got %v", m.view)
        }
        if m.openItemID != itemID {
                t.Fatalf("expected openItemID %q; got %q", itemID, m.openItemID)
        }
        if m.selectedProjectID != projectID {
                t.Fatalf("expected selectedProjectID %q; got %q", projectID, m.selectedProjectID)
        }
        if m.selectedOutlineID != outlineID {
                t.Fatalf("expected selectedOutlineID %q; got %q", outlineID, m.selectedOutlineID)
        }
        if !m.hasReturnView || m.returnView != viewOutline {
                t.Fatalf("expected return view outline; has=%v return=%v", m.hasReturnView, m.returnView)
        }
}
