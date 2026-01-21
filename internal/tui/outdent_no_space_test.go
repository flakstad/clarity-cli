package tui

import (
	"sort"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func TestOutdentSelected_NoSpaceBetweenRanks_AppendsToBottom(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	act := "act-a"
	proj := "proj-a"
	out := "out-a"

	parentID := "p"
	sibID := "s"
	childID := "c"

	// "y" < "y0" is a prefix-adjacent pair; there is no rank strictly between them.
	db := &store.DB{
		CurrentActorID: act,
		Actors:         []model.Actor{{ID: act, Kind: model.ActorKindHuman, Name: "tester"}},
		Projects:       []model.Project{{ID: proj, Name: "P", CreatedBy: act, CreatedAt: now}},
		Outlines:       []model.Outline{{ID: out, ProjectID: proj, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: act, CreatedAt: now}},
		Items: []model.Item{
			{ID: parentID, ProjectID: proj, OutlineID: out, Rank: "y", Title: "P", OwnerActorID: act, CreatedBy: act, CreatedAt: now, UpdatedAt: now},
			{ID: sibID, ProjectID: proj, OutlineID: out, Rank: "y0", Title: "S", OwnerActorID: act, CreatedBy: act, CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
			{ID: childID, ProjectID: proj, OutlineID: out, ParentID: &parentID, Rank: "h", Title: "C", OwnerActorID: act, CreatedBy: act, CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second)},
		},
	}
	s := store.Store{Dir: dir}
	if err := s.Save(db); err != nil {
		t.Fatalf("save db: %v", err)
	}

	m := newAppModel(dir, db)
	m.view = viewOutline
	m.selectedProjectID = proj
	m.selectedOutlineID = out
	m.selectedOutline = &db.Outlines[0]
	m.collapsed = map[string]bool{parentID: false}
	m.refreshItems(db.Outlines[0])
	selectListItemByID(&m.itemsList, childID)

	if err := m.outdentSelected(); err != nil {
		t.Fatalf("expected outdent to succeed; got err: %v", err)
	}

	db2, err := s.Load()
	if err != nil {
		t.Fatalf("load db: %v", err)
	}
	p1, _ := db2.FindItem(parentID)
	s1, _ := db2.FindItem(sibID)
	c1, _ := db2.FindItem(childID)

	if c1.ParentID != nil && *c1.ParentID != "" {
		t.Fatalf("expected child to be outdented to root; got parentID=%v", c1.ParentID)
	}

	// Verify it ended up at the bottom of the outline (after all root siblings).
	roots := []model.Item{*p1, *s1, *c1}
	sort.Slice(roots, func(i, j int) bool { return compareOutlineItems(roots[i], roots[j]) < 0 })
	got := []string{roots[0].ID, roots[1].ID, roots[2].ID}
	want := []string{parentID, sibID, childID}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected root order %v; got %v (ranks: p=%q s=%q c=%q)", want, got, p1.Rank, s1.Rank, c1.Rank)
		}
	}
}
