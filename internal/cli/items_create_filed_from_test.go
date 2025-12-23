package cli

import (
        "encoding/json"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestItemsCreate_FiledFromPrefixesDescription(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()

        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)
        actorID := "act-testhuman"
        projectID := "proj-test"
        outlineID := "out-test"

        db := &store.DB{
                Version:        1,
                CurrentActorID: actorID,
                NextIDs:        map[string]int{},
                Actors: []model.Actor{
                        {ID: actorID, Kind: model.ActorKindHuman, Name: "Test Human"},
                },
                Projects: []model.Project{
                        {ID: projectID, Name: "Test Project", CreatedBy: actorID, CreatedAt: now},
                },
                Outlines: []model.Outline{
                        {ID: outlineID, ProjectID: projectID, Name: nil, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now},
                },
                Items:    []model.Item{},
                Deps:     []model.Dependency{},
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        if err := (store.Store{Dir: dir}).Save(db); err != nil {
                t.Fatalf("seed store: %v", err)
        }

        out, errOut, err := runCLI(t, []string{
                "--dir", dir,
                "--actor", actorID,
                "items", "create",
                "--project", projectID,
                "--title", "Unrelated issue",
                "--description", "Details",
                "--filed-from", "item-vth",
        })
        if err != nil {
                t.Fatalf("items create error: %v\nstderr:\n%s", err, string(errOut))
        }

        var env map[string]any
        if err := json.Unmarshal(out, &env); err != nil {
                t.Fatalf("unmarshal create output: %v\nstdout:\n%s", err, string(out))
        }

        data, ok := env["data"].(map[string]any)
        if !ok {
                t.Fatalf("expected data object; got: %#v", env["data"])
        }
        desc, _ := data["description"].(string)
        if want := "Filed from: item-vth\n\nDetails"; desc != want {
                t.Fatalf("unexpected description.\nwant:\n%q\ngot:\n%q", want, desc)
        }
}
