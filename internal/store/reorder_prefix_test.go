package store

import (
	"testing"
	"time"

	"clarity-cli/internal/model"
)

func TestPlanReorderRanks_PrefixAdjacentBounds_DoesNotJump(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// "y" < "y0" is a prefix-adjacent pair with no in-between rank available.
	// Reordering an item into that gap must not produce a rank that sorts after "y0"
	// (which would manifest as a "jump" past the intended position).
	a := &model.Item{ID: "a", Rank: "y", CreatedAt: now}
	b := &model.Item{ID: "b", Rank: "y0", CreatedAt: now.Add(time.Second)}
	x := &model.Item{ID: "x", Rank: "h", CreatedAt: now.Add(2 * time.Second)}

	sibs := []*model.Item{a, b, x}

	// After removing x, siblings are [a, b]. Insert x after a => insertAt=1.
	res, err := PlanReorderRanks(sibs, "x", 1)
	if err != nil {
		t.Fatalf("PlanReorderRanks unexpected err: %v", err)
	}
	for id, r := range res.RankByID {
		switch id {
		case "a":
			a.Rank = r
		case "b":
			b.Rank = r
		case "x":
			x.Rank = r
		}
	}

	final := []*model.Item{a, b, x}
	SortItemsByRankOrder(final)
	if got, want := []string{final[0].ID, final[1].ID, final[2].ID}, []string{"a", "x", "b"}; got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("expected order %v; got %v (ranks: a=%q x=%q b=%q)", want, got, a.Rank, x.Rank, b.Rank)
	}
}
