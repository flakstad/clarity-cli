package web

import (
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func TestAgendaRowsWeb_ExcludesItemsWithEmptyStatus(t *testing.T) {
	actorID := "act-human"
	now := time.Now().UTC()

	db := &store.DB{
		CurrentActorID: actorID,
		Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "human"}},
		Projects:       []model.Project{{ID: "proj-a", Name: "Alpha", CreatedBy: actorID, CreatedAt: now}},
		Outlines:       []model.Outline{{ID: "out-a", ProjectID: "proj-a", StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now}},
		Items: []model.Item{
			{ID: "item-todo", ProjectID: "proj-a", OutlineID: "out-a", Rank: "h", Title: "Keep me", StatusID: "todo", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
			{ID: "item-empty", ProjectID: "proj-a", OutlineID: "out-a", Rank: "i", Title: "Hide me", StatusID: "", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
			{ID: "item-done", ProjectID: "proj-a", OutlineID: "out-a", Rank: "j", Title: "Also hide", StatusID: "done", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
		},
	}

	rows := agendaRowsWeb(db, actorID)
	var itemIDs []string
	for _, r := range rows {
		if r.Kind != "item" {
			continue
		}
		itemIDs = append(itemIDs, r.ItemID)
	}

	if len(itemIDs) != 1 || itemIDs[0] != "item-todo" {
		t.Fatalf("expected only item-todo in agenda rows, got %v", itemIDs)
	}
}
