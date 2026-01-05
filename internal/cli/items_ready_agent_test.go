package cli

import (
        "encoding/json"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestItemsReady_IncludesAssignedToCurrentAgent_First(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)

        humanID := "act-human"
        agent1 := "act-agent1"
        agent2 := "act-agent2"
        projectID := "proj-a"
        outlineID := "out-a"

        assignedToMe := "item-assigned-me"
        assignedToOther := "item-assigned-other"
        unassigned := "item-unassigned"

        db := &store.DB{
                Version:        1,
                CurrentActorID: agent1,
                NextIDs:        map[string]int{},
                Actors: []model.Actor{
                        {ID: humanID, Kind: model.ActorKindHuman, Name: "Human"},
                        {ID: agent1, Kind: model.ActorKindAgent, Name: "s1 Agent1", UserID: ptr(humanID)},
                        {ID: agent2, Kind: model.ActorKindAgent, Name: "s2 Agent2", UserID: ptr(humanID)},
                },
                Projects: []model.Project{
                        {ID: projectID, Name: "P", CreatedBy: humanID, CreatedAt: now},
                },
                Outlines: []model.Outline{
                        {ID: outlineID, ProjectID: projectID, Name: nil, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: humanID, CreatedAt: now},
                },
                Items: []model.Item{
                        {
                                ID:              assignedToMe,
                                ProjectID:       projectID,
                                OutlineID:       outlineID,
                                Rank:            "h",
                                Title:           "Assigned to me",
                                StatusID:        "todo",
                                AssignedActorID: ptr(agent1),
                                OwnerActorID:    humanID,
                                CreatedBy:       humanID,
                                CreatedAt:       now,
                                UpdatedAt:       now,
                        },
                        {
                                ID:              assignedToOther,
                                ProjectID:       projectID,
                                OutlineID:       outlineID,
                                Rank:            "h0",
                                Title:           "Assigned to other",
                                StatusID:        "todo",
                                AssignedActorID: ptr(agent2),
                                OwnerActorID:    humanID,
                                CreatedBy:       humanID,
                                CreatedAt:       now,
                                UpdatedAt:       now,
                        },
                        {
                                ID:           unassigned,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                Rank:         "h00",
                                Title:        "Unassigned",
                                StatusID:     "todo",
                                OwnerActorID: humanID,
                                CreatedBy:    humanID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                },
                Deps:     []model.Dependency{},
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        if err := (store.Store{Dir: dir}).Save(db); err != nil {
                t.Fatalf("seed store: %v", err)
        }

        out, errOut, err := runCLI2(t, []string{"--dir", dir, "--actor", agent1, "items", "ready"})
        if err != nil {
                t.Fatalf("items ready: %v\nstderr:\n%s", err, string(errOut))
        }

        var env map[string]any
        if err := json.Unmarshal(out, &env); err != nil {
                t.Fatalf("unmarshal: %v\nstdout:\n%s", err, string(out))
        }
        items, ok := env["data"].([]any)
        if !ok {
                t.Fatalf("expected data array; got: %#v", env["data"])
        }
        if len(items) < 2 {
                t.Fatalf("expected at least 2 items; got %d", len(items))
        }

        // Should include the item assigned to the current agent and omit the item assigned to another agent.
        if !containsItemID(items, assignedToMe) {
                t.Fatalf("expected item assigned to current agent to be included")
        }
        if containsItemID(items, assignedToOther) {
                t.Fatalf("expected item assigned to another agent to be excluded by default")
        }
        if !containsItemID(items, unassigned) {
                t.Fatalf("expected unassigned item to be included")
        }

        // Assigned-to-me should be first in the list.
        first, _ := items[0].(map[string]any)
        if id, _ := first["id"].(string); id != assignedToMe {
                t.Fatalf("expected first item to be %q; got %q", assignedToMe, id)
        }
}
