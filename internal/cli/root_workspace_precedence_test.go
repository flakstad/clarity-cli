package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func TestWorkspaceFlagOverridesEnvDir(t *testing.T) {
	wsName := "Flakstad Software"
	itemID := "item-6pm"

	cfgDir := t.TempDir()
	wsDir := filepath.Join(cfgDir, "workspaces", wsName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace dir: %v", err)
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	actorID := "act-testhuman"
	projectID := "proj-test"
	outlineID := "out-test"

	// Seed the selected workspace with the target item.
	wsDB := &store.DB{
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
				Title:        "Workspace Item",
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
	if err := (store.Store{Dir: wsDir}).Save(wsDB); err != nil {
		t.Fatalf("seed workspace store: %v", err)
	}

	// Seed a different store dir (CLARITY_DIR) that *does not* contain the target item.
	envDir := t.TempDir()
	envDB := &store.DB{
		Version:        1,
		CurrentActorID: actorID,
		NextIDs:        map[string]int{},
		Actors: []model.Actor{
			{ID: actorID, Kind: model.ActorKindHuman, Name: "Test Human"},
		},
		Projects: []model.Project{
			{ID: projectID, Name: "Env Project", CreatedBy: actorID, CreatedAt: now},
		},
		Outlines: []model.Outline{
			{ID: outlineID, ProjectID: projectID, Name: nil, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now},
		},
		Items: []model.Item{
			{
				ID:           "item-other",
				ProjectID:    projectID,
				OutlineID:    outlineID,
				ParentID:     nil,
				Rank:         "h",
				Title:        "Other",
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
	if err := (store.Store{Dir: envDir}).Save(envDB); err != nil {
		t.Fatalf("seed env store: %v", err)
	}

	setenv(t, "CLARITY_CONFIG_DIR", cfgDir)
	setenv(t, "CLARITY_DIR", envDir)

	cmd := NewRootCmd()
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"items", "show", itemID, "--workspace", wsName})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute error: %v\nstderr:\n%s", err, errBuf.String())
	}

	var env struct {
		Data struct {
			Item model.Item `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal(outBuf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal output: %v\nstdout:\n%s", err, outBuf.String())
	}
	if env.Data.Item.ID != itemID {
		t.Fatalf("unexpected item id: got %q want %q\nstdout:\n%s", env.Data.Item.ID, itemID, outBuf.String())
	}
}

func setenv(t *testing.T, k, v string) {
	t.Helper()
	prev, had := os.LookupEnv(k)
	if err := os.Setenv(k, v); err != nil {
		t.Fatalf("setenv %s: %v", k, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(k, prev)
		} else {
			_ = os.Unsetenv(k)
		}
	})
}
