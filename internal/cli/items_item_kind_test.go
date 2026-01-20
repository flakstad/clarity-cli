package cli

import (
	"encoding/json"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func TestItemsSetItemKind_SetsAndClears(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)
	ownerID := "act-owner"
	projectID := "proj-test"
	outlineID := "out-test"
	itemID := "item-1"

	db := &store.DB{
		Version:        1,
		CurrentActorID: ownerID,
		NextIDs:        map[string]int{},
		Actors: []model.Actor{
			{ID: ownerID, Kind: model.ActorKindHuman, Name: "Owner"},
		},
		Projects: []model.Project{
			{ID: projectID, Name: "Test Project", CreatedBy: ownerID, CreatedAt: now},
		},
		Outlines: []model.Outline{
			{ID: outlineID, ProjectID: projectID, Name: nil, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: ownerID, CreatedAt: now},
		},
		Items: []model.Item{
			{
				ID:           itemID,
				ProjectID:    projectID,
				OutlineID:    outlineID,
				Rank:         "h",
				Title:        "Item",
				StatusID:     "todo",
				OwnerActorID: ownerID,
				CreatedBy:    ownerID,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
	}

	if err := (store.Store{Dir: dir}).Save(db); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	out, errOut, err := runCLI(t, []string{"--dir", dir, "--actor", ownerID, "items", "set-item-kind", itemID, "--kind", "status"})
	if err != nil {
		t.Fatalf("items set-item-kind error: %v\nstderr:\n%s", err, string(errOut))
	}
	var v map[string]any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("unmarshal output: %v\nstdout:\n%s", err, string(out))
	}
	data, ok := v["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object; got: %#v", v["data"])
	}
	if got, _ := data["itemKind"].(string); got != "status" {
		t.Fatalf("expected itemKind=status; got %q", got)
	}

	out, errOut, err = runCLI(t, []string{"--dir", dir, "--actor", ownerID, "items", "set-item-kind", itemID, "--kind", "inherit"})
	if err != nil {
		t.Fatalf("items set-item-kind clear error: %v\nstderr:\n%s", err, string(errOut))
	}
	v = map[string]any{}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("unmarshal output: %v\nstdout:\n%s", err, string(out))
	}
	data, ok = v["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object; got: %#v", v["data"])
	}
	if got, _ := data["itemKind"].(string); got != "" {
		t.Fatalf("expected itemKind cleared; got %#v", data["itemKind"])
	}
}
