package store

import "testing"

func TestRankBetweenUnique_AvoidsCollisionByTighteningLowerBound(t *testing.T) {
        existing := map[string]bool{
                "p": true,
        }
        // RankBetween("m","t") commonly yields "p". Ensure we don't return an existing rank.
        r, err := RankBetweenUnique(existing, "m", "t")
        if err != nil {
                t.Fatalf("unexpected err: %v", err)
        }
        if r == "p" {
                t.Fatalf("expected unique rank (not p)")
        }
        if existing[r] {
                t.Fatalf("expected returned rank to be unique; got existing rank %q", r)
        }
}

func TestRankBetweenUnique_OpenEndedUpper_IsUnique(t *testing.T) {
        existing := map[string]bool{
                "h0":  true,
                "h00": true,
        }
        r, err := RankBetweenUnique(existing, "h", "")
        if err != nil {
                t.Fatalf("unexpected err: %v", err)
        }
        if existing[r] {
                t.Fatalf("expected returned rank to be unique; got existing rank %q", r)
        }
}
