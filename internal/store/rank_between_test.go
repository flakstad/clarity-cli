package store

import "testing"

func TestRankBetween_PrefixAdjacent_NoSpace(t *testing.T) {
	// "y" < "y0" but there is no lexicographic string strictly between them in our alphabet,
	// since '0' is the minimal digit and end-of-string sorts before any digit.
	if _, err := RankBetween("y", "y0"); err == nil {
		t.Fatalf("expected error for prefix-adjacent bounds (no space), got nil")
	}
}
