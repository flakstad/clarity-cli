package cli

import (
        "encoding/json"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestItemsSetAssign_DoesNotShadowGlobalActorFlag(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()

        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)
        ownerID := "act-owner"
        assigneeID := "act-assignee"
        projectID := "proj-test"
        outlineID := "out-test"
        itemID := "item-test"

        db := &store.DB{
                Version:        1,
                CurrentActorID: ownerID,
                NextIDs:        map[string]int{},
                Actors: []model.Actor{
                        {ID: ownerID, Kind: model.ActorKindHuman, Name: "Owner"},
                        {ID: assigneeID, Kind: model.ActorKindHuman, Name: "Assignee"},
                },
                Projects: []model.Project{
                        {ID: projectID, Name: "Test Project", CreatedBy: ownerID, CreatedAt: now},
                },
                Outlines: []model.Outline{
                        {ID: outlineID, ProjectID: projectID, Name: nil, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: ownerID, CreatedAt: now},
                },
                Items: []model.Item{
                        {
                                ID:              itemID,
                                ProjectID:       projectID,
                                OutlineID:       outlineID,
                                ParentID:        nil,
                                Rank:            "h",
                                Title:           "Test Item",
                                Description:     "Hello",
                                StatusID:        "todo",
                                Priority:        false,
                                OnHold:          false,
                                Archived:        false,
                                OwnerActorID:    ownerID,
                                AssignedActorID: nil,
                                CreatedBy:       ownerID,
                                CreatedAt:       now,
                                UpdatedAt:       now,
                        },
                },
                Deps:     []model.Dependency{},
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        if err := (store.Store{Dir: dir}).Save(db); err != nil {
                t.Fatalf("seed store: %v", err)
        }

        out, errOut, err := runCLI(t, []string{"--dir", dir, "--actor", ownerID, "items", "set-assign", itemID, "--assignee", assigneeID})
        if err != nil {
                t.Fatalf("items set-assign error: %v\nstderr:\n%s", err, string(errOut))
        }

        var v map[string]any
        if err := json.Unmarshal(out, &v); err != nil {
                t.Fatalf("unmarshal output: %v\nstdout:\n%s", err, string(out))
        }
        data, ok := v["data"].(map[string]any)
        if !ok {
                t.Fatalf("expected data object; got: %#v", v["data"])
        }
        if got, _ := data["assignedActorId"].(string); got != assigneeID {
                t.Fatalf("expected assignedActorId=%q; got %q", assigneeID, got)
        }
        if got, _ := data["ownerActorId"].(string); got != assigneeID {
                t.Fatalf("expected ownerActorId to transfer to %q; got %q", assigneeID, got)
        }
}
