package tui

import (
	"path/filepath"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func TestCaptureTemplateTargetLabel_UsesOutlineDisplayName(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CLARITY_CONFIG_DIR", cfgDir)

	ws := "Test Workspace"
	dir := filepath.Join(cfgDir, "workspaces", ws)
	s := store.Store{Dir: dir}

	actorID := "act-a"
	now := time.Now().UTC()
	outlineName := "Inbox"

	db := &store.DB{
		Version:        1,
		CurrentActorID: actorID,
		Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "Human"}},
		Projects: []model.Project{{
			ID:        "proj-a",
			Name:      "Project A",
			CreatedBy: actorID,
			CreatedAt: now,
		}},
		Outlines: []model.Outline{{
			ID:         "out-a",
			ProjectID:  "proj-a",
			Name:       &outlineName,
			StatusDefs: store.DefaultOutlineStatusDefs(),
			CreatedBy:  actorID,
			CreatedAt:  now,
		}},
	}
	if err := s.Save(db); err != nil {
		t.Fatalf("seed workspace db: %v", err)
	}

	got := captureTemplateTargetLabel(ws, "out-a")
	want := "Test Workspace / Project A / Inbox"
	if got != want {
		t.Fatalf("unexpected label: got %q, want %q", got, want)
	}
}
