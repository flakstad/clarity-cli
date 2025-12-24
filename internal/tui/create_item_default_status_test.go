package tui

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestCreateItemFromModal_DefaultStatusIsFirstStatusDef(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Now().UTC()

        actorID := "act-human"
        projectID := "proj-a"
        outlineID := "out-a"

        db := &store.DB{
                Version:          1,
                CurrentActorID:   actorID,
                CurrentProjectID: projectID,
                NextIDs:          map[string]int{},
                Actors:           []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "Human"}},
                Projects:         []model.Project{{ID: projectID, Name: "P", CreatedBy: actorID, CreatedAt: now}},
                Outlines: []model.Outline{{
                        ID:        outlineID,
                        ProjectID: projectID,
                        Name:      nil,
                        StatusDefs: []model.OutlineStatusDef{
                                {ID: "backlog", Label: "Backlog", IsEndState: false},
                                {ID: "todo", Label: "Todo", IsEndState: false},
                        },
                        CreatedBy: actorID,
                        CreatedAt: now,
                }},
                Items:    []model.Item{},
                Deps:     []model.Dependency{},
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        s := store.Store{Dir: dir}
        if err := s.Save(db); err != nil {
                t.Fatalf("save db: %v", err)
        }

        m := newAppModel(dir, db)
        m.selectedOutline = &db.Outlines[0]

        if err := m.createItemFromModal("X"); err != nil {
                t.Fatalf("create item: %v", err)
        }

        db2, err := s.Load()
        if err != nil {
                t.Fatalf("load db: %v", err)
        }
        if len(db2.Items) != 1 {
                t.Fatalf("expected 1 item, got %d", len(db2.Items))
        }
        if got := db2.Items[0].StatusID; got != "backlog" {
                t.Fatalf("unexpected status id. want %q, got %q", "backlog", got)
        }
}
