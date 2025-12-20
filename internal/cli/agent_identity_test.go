package cli

import (
        "encoding/json"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestIdentityAgentEnsure_CreatesOrReusesSessionIdentity(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)

        humanID := "act-human"
        db := &store.DB{
                Version:        1,
                CurrentActorID: humanID,
                NextIDs:        map[string]int{},
                Actors: []model.Actor{
                        {ID: humanID, Kind: model.ActorKindHuman, Name: "Human"},
                },
                Projects: []model.Project{},
                Outlines: []model.Outline{},
                Items:    []model.Item{},
                Deps:     []model.Dependency{},
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }
        _ = now

        if err := (store.Store{Dir: dir}).Save(db); err != nil {
                t.Fatalf("seed store: %v", err)
        }

        session := "sess-123"

        out1, err1, err := runCLI(t, []string{"--dir", dir, "--actor", humanID, "identity", "agent", "ensure", "--session", session, "--name", "Cursor Agent", "--use"})
        if err != nil {
                t.Fatalf("ensure 1 error: %v\nstderr:\n%s", err, string(err1))
        }

        var env1 map[string]any
        if err := json.Unmarshal(out1, &env1); err != nil {
                t.Fatalf("unmarshal ensure 1: %v\nstdout:\n%s", err, string(out1))
        }

        data1, ok := env1["data"].(map[string]any)
        if !ok {
                t.Fatalf("expected data object; got: %#v", env1["data"])
        }
        actorID1, _ := data1["id"].(string)
        if actorID1 == "" {
                t.Fatalf("expected actor id in data; got: %#v", data1)
        }

        out2, err2, err := runCLI(t, []string{"--dir", dir, "--actor", humanID, "identity", "agent", "ensure", "--session", session, "--name", "Cursor Agent", "--use"})
        if err != nil {
                t.Fatalf("ensure 2 error: %v\nstderr:\n%s", err, string(err2))
        }
        var env2 map[string]any
        if err := json.Unmarshal(out2, &env2); err != nil {
                t.Fatalf("unmarshal ensure 2: %v\nstdout:\n%s", err, string(out2))
        }
        data2, ok := env2["data"].(map[string]any)
        if !ok {
                t.Fatalf("expected data object; got: %#v", env2["data"])
        }
        actorID2, _ := data2["id"].(string)
        if actorID2 != actorID1 {
                t.Fatalf("expected same actor id for same session; got %q then %q", actorID1, actorID2)
        }
}

func TestAgentStart_EnsuresIdentityAndClaimsItem(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()
        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)

        humanID := "act-human"
        projectID := "proj-a"
        outlineID := "out-a"
        itemID := "item-a"

        db := &store.DB{
                Version:        1,
                CurrentActorID: humanID,
                NextIDs:        map[string]int{},
                Actors: []model.Actor{
                        {ID: humanID, Kind: model.ActorKindHuman, Name: "Human"},
                },
                Projects: []model.Project{
                        {ID: projectID, Name: "P", CreatedBy: humanID, CreatedAt: now},
                },
                Outlines: []model.Outline{
                        {ID: outlineID, ProjectID: projectID, Name: nil, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: humanID, CreatedAt: now},
                },
                Items: []model.Item{
                        {
                                ID:           itemID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                Rank:         "h",
                                Title:        "Task",
                                Description:  "",
                                StatusID:     "todo",
                                Priority:     false,
                                OnHold:       false,
                                Archived:     false,
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

        session := "sess-claim"
        out, errOut, err := runCLI(t, []string{"--dir", dir, "--actor", humanID, "agent", "start", itemID, "--session", session, "--name", "Cursor Agent"})
        if err != nil {
                t.Fatalf("agent start error: %v\nstderr:\n%s", err, string(errOut))
        }

        var env map[string]any
        if err := json.Unmarshal(out, &env); err != nil {
                t.Fatalf("unmarshal agent start: %v\nstdout:\n%s", err, string(out))
        }
        data, ok := env["data"].(map[string]any)
        if !ok {
                t.Fatalf("expected data object; got: %#v", env["data"])
        }
        actor, ok := data["actor"].(map[string]any)
        if !ok {
                t.Fatalf("expected data.actor object; got: %#v", data["actor"])
        }
        agentID, _ := actor["id"].(string)
        if agentID == "" {
                t.Fatalf("expected agent id; got: %#v", actor)
        }

        item, ok := data["item"].(map[string]any)
        if !ok {
                t.Fatalf("expected data.item object; got: %#v", data["item"])
        }

        assigned, _ := item["assignedActorId"].(string)
        if assigned != agentID {
                t.Fatalf("expected item assignedActorId=%q; got %q", agentID, assigned)
        }
        owner, _ := item["ownerActorId"].(string)
        if owner != agentID {
                t.Fatalf("expected item ownerActorId=%q; got %q", agentID, owner)
        }
}
