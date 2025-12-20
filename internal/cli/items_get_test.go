package cli

import (
        "bytes"
        "encoding/json"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestItemsGet_IsAliasForShow(t *testing.T) {
        t.Parallel()

        dir := t.TempDir()

        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)
        actorID := "act-testhuman"
        projectID := "proj-test"
        outlineID := "out-test"
        itemID := "item-test"

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
                Items: []model.Item{
                        {
                                ID:           itemID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                ParentID:     nil,
                                Rank:         "h",
                                Title:        "Test Item",
                                Description:  "Hello",
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

        if err := (store.Store{Dir: dir}).Save(db); err != nil {
                t.Fatalf("seed store: %v", err)
        }

        showOut, showErr, err := runCLI(t, []string{"--dir", dir, "--actor", actorID, "items", "show", itemID})
        if err != nil {
                t.Fatalf("items show error: %v\nstderr:\n%s", err, string(showErr))
        }

        getOut, getErr, err := runCLI(t, []string{"--dir", dir, "--actor", actorID, "items", "get", itemID})
        if err != nil {
                t.Fatalf("items get error: %v\nstderr:\n%s", err, string(getErr))
        }

        var showV any
        if err := json.Unmarshal(showOut, &showV); err != nil {
                t.Fatalf("unmarshal show output: %v\nstdout:\n%s", err, string(showOut))
        }
        var getV any
        if err := json.Unmarshal(getOut, &getV); err != nil {
                t.Fatalf("unmarshal get output: %v\nstdout:\n%s", err, string(getOut))
        }

        if !jsonDeepEqual(showV, getV) {
                t.Fatalf("items get output differs from items show\nshow:\n%s\nget:\n%s", string(showOut), string(getOut))
        }
}

func runCLI(t *testing.T, args []string) (stdout []byte, stderr []byte, err error) {
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

func jsonDeepEqual(a, b any) bool {
        ab, err := json.Marshal(a)
        if err != nil {
                return false
        }
        bb, err := json.Marshal(b)
        if err != nil {
                return false
        }
        return bytes.Equal(ab, bb)
}
