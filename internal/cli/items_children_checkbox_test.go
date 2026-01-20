package cli

import (
	"encoding/json"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func TestItemsSetChildrenCheckbox_TogglesChildrenKind(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)
	ownerID := "act-owner"
	projectID := "proj-test"
	outlineID := "out-test"
	itemID := "item-parent"

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
				Title:        "Parent",
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

	out, errOut, err := runCLI(t, []string{"--dir", dir, "--actor", ownerID, "items", "set-children-checkbox", itemID, "--on"})
	if err != nil {
		t.Fatalf("items set-children-checkbox --on error: %v\nstderr:\n%s", err, string(errOut))
	}
	var v map[string]any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("unmarshal output: %v\nstdout:\n%s", err, string(out))
	}
	data, ok := v["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object; got: %#v", v["data"])
	}
	if got, _ := data["childrenKind"].(string); got != "checkbox" {
		t.Fatalf("expected childrenKind=%q; got %q", "checkbox", got)
	}

	out, errOut, err = runCLI(t, []string{"--dir", dir, "--actor", ownerID, "items", "set-children-checkbox", itemID, "--off"})
	if err != nil {
		t.Fatalf("items set-children-checkbox --off error: %v\nstderr:\n%s", err, string(errOut))
	}
	v = map[string]any{}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("unmarshal output: %v\nstdout:\n%s", err, string(out))
	}
	data, ok = v["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object; got: %#v", v["data"])
	}
	if got, _ := data["childrenKind"].(string); got != "" {
		t.Fatalf("expected childrenKind cleared; got %#v", data["childrenKind"])
	}
}
