package cli

import (
        "bytes"
        "encoding/json"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestItemsReady_ExcludesAssignedByDefault(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)

        humanID := "act-human"
        agent1 := "act-agent1"
        projectID := "proj-a"
        outlineID := "out-a"

        assigned := "item-assigned"
        unassigned := "item-unassigned"

        db := &store.DB{
                Version:        1,
                CurrentActorID: humanID,
                NextIDs:        map[string]int{},
                Actors: []model.Actor{
                        {ID: humanID, Kind: model.ActorKindHuman, Name: "Human"},
                        {ID: agent1, Kind: model.ActorKindAgent, Name: "s1 Agent1", UserID: ptr(humanID)},
                },
                Projects: []model.Project{
                        {ID: projectID, Name: "P", CreatedBy: humanID, CreatedAt: now},
                },
                Outlines: []model.Outline{
                        {ID: outlineID, ProjectID: projectID, Name: nil, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: humanID, CreatedAt: now},
                },
                Items: []model.Item{
                        {
                                ID:              assigned,
                                ProjectID:       projectID,
                                OutlineID:       outlineID,
                                Rank:            "h",
                                Title:           "Assigned",
                                StatusID:        "todo",
                                AssignedActorID: ptr(agent1),
                                OwnerActorID:    humanID,
                                CreatedBy:       humanID,
                                CreatedAt:       now,
                                UpdatedAt:       now,
                        },
                        {
                                ID:           unassigned,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                Rank:         "h0",
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

        out, errOut, err := runCLI2(t, []string{"--dir", dir, "--actor", humanID, "items", "ready"})
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
        if containsItemID(items, assigned) {
                t.Fatalf("expected assigned item to be excluded by default")
        }
        if !containsItemID(items, unassigned) {
                t.Fatalf("expected unassigned item to be included")
        }

        out2, errOut2, err := runCLI2(t, []string{"--dir", dir, "--actor", humanID, "items", "ready", "--include-assigned"})
        if err != nil {
                t.Fatalf("items ready --include-assigned: %v\nstderr:\n%s", err, string(errOut2))
        }
        var env2 map[string]any
        _ = json.Unmarshal(out2, &env2)
        items2, _ := env2["data"].([]any)
        if !containsItemID(items2, assigned) {
                t.Fatalf("expected assigned item to be included with --include-assigned")
        }
}

func TestItemsClaim_RequiresTakeAssignedToSteal(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)

        humanID := "act-human"
        agent1 := "act-agent1"
        agent2 := "act-agent2"
        projectID := "proj-a"
        outlineID := "out-a"
        itemID := "item-a"

        db := &store.DB{
                Version:        1,
                CurrentActorID: humanID,
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
                                ID:              itemID,
                                ProjectID:       projectID,
                                OutlineID:       outlineID,
                                Rank:            "h",
                                Title:           "Task",
                                StatusID:        "todo",
                                AssignedActorID: ptr(agent1),
                                OwnerActorID:    agent1,
                                CreatedBy:       humanID,
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

        _, errOut, err := runCLI2(t, []string{"--dir", dir, "--actor", agent2, "items", "claim", itemID})
        if err == nil {
                t.Fatalf("expected claim to fail without --take-assigned")
        }
        if !bytes.Contains(errOut, []byte("take-assigned")) {
                t.Fatalf("expected error to mention --take-assigned; stderr:\n%s", string(errOut))
        }

        out, errOut2, err := runCLI2(t, []string{"--dir", dir, "--actor", agent2, "items", "claim", itemID, "--take-assigned"})
        if err != nil {
                t.Fatalf("claim with --take-assigned failed: %v\nstderr:\n%s", err, string(errOut2))
        }
        var env map[string]any
        _ = json.Unmarshal(out, &env)
        item, ok := env["data"].(map[string]any)
        if !ok {
                t.Fatalf("expected data object; got: %#v", env["data"])
        }
        assignedActor, _ := item["assignedActorId"].(string)
        if assignedActor != agent2 {
                t.Fatalf("expected assignedActorId=%q; got %q", agent2, assignedActor)
        }
}

func runCLI2(t *testing.T, args []string) (stdout []byte, stderr []byte, err error) {
        t.Helper()
        cmd := NewRootCmd()
        var outBuf bytes.Buffer
        var errBuf bytes.Buffer
        cmd.SetOut(&outBuf)
        cmd.SetErr(&errBuf)
        cmd.SetArgs(args)
        e := cmd.Execute()
        return outBuf.Bytes(), errBuf.Bytes(), e
}

func containsItemID(items []any, id string) bool {
        for _, it := range items {
                m, ok := it.(map[string]any)
                if !ok {
                        continue
                }
                if s, _ := m["id"].(string); s == id {
                        return true
                }
        }
        return false
}

func ptr(s string) *string { return &s }
