package cli

import (
	"encoding/json"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func TestItemsCopy_CopiesContentAndInsertsAfterSource(t *testing.T) {
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
		Items: []model.Item{
			{
				ID:           "item-a",
				ProjectID:    projectID,
				OutlineID:    outlineID,
				Rank:         "h",
				Title:        "Title A",
				Description:  "Desc A",
				StatusID:     "doing",
				Priority:     true,
				OnHold:       true,
				Tags:         []string{"x", "y"},
				OwnerActorID: actorID,
				CreatedBy:    actorID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           "item-b",
				ProjectID:    projectID,
				OutlineID:    outlineID,
				Rank:         "i",
				Title:        "Title B",
				StatusID:     "todo",
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

	out, errOut, err := runCLI(t, []string{"--dir", dir, "--actor", actorID, "items", "copy", "item-a"})
	if err != nil {
		t.Fatalf("items copy error: %v\nstderr:\n%s", err, string(errOut))
	}

	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("unmarshal copy output: %v\nstdout:\n%s", err, string(out))
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object; got: %#v", env["data"])
	}
	newID, _ := data["id"].(string)
	if newID == "" || newID == "item-a" || newID == "item-b" {
		t.Fatalf("unexpected new item id: %q", newID)
	}

	loaded, err := (store.Store{Dir: dir}).Load()
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	it, ok := loaded.FindItem(newID)
	if !ok || it == nil {
		t.Fatalf("expected item %q to exist", newID)
	}
	if it.Title != "Title A" || it.Description != "Desc A" {
		t.Fatalf("unexpected copied content: %#v", it)
	}
	if it.StatusID != "todo" || it.Priority || it.OnHold {
		t.Fatalf("expected copied item to reset status/flags; got status=%q priority=%v onHold=%v", it.StatusID, it.Priority, it.OnHold)
	}
	if it.Rank <= "h" || it.Rank >= "i" {
		t.Fatalf("expected copied rank between %q and %q; got %q", "h", "i", it.Rank)
	}
}
