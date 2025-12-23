package store

import (
        "errors"
        "sort"
        "strings"

        "clarity-cli/internal/model"
)

// ReorderResult describes the rank updates needed to realize an index-based reorder.
// RankByID includes only items whose ranks should change.
type ReorderResult struct {
        RankByID     map[string]string
        WindowIDs    []string // IDs whose ranks were (re)assigned in the fallback path (in final order)
        UsedFallback bool
}

// SortItemsByRankOrder sorts items in place using the same ordering as the TUI outline view:
// rank (lexicographic), then CreatedAt, then ID.
func SortItemsByRankOrder(items []*model.Item) {
        sort.SliceStable(items, func(i, j int) bool {
                return compareItemsByRankCreatedID(*items[i], *items[j]) < 0
        })
}

func compareItemsByRankCreatedID(a, b model.Item) int {
        ra := strings.TrimSpace(a.Rank)
        rb := strings.TrimSpace(b.Rank)
        if ra != "" && rb != "" {
                if ra < rb {
                        return -1
                }
                if ra > rb {
                        return 1
                }
                if a.CreatedAt.Before(b.CreatedAt) {
                        return -1
                }
                if a.CreatedAt.After(b.CreatedAt) {
                        return 1
                }
                if a.ID < b.ID {
                        return -1
                }
                if a.ID > b.ID {
                        return 1
                }
                return 0
        }
        if a.CreatedAt.Before(b.CreatedAt) {
                return -1
        }
        if a.CreatedAt.After(b.CreatedAt) {
                return 1
        }
        if a.ID < b.ID {
                return -1
        }
        if a.ID > b.ID {
                return 1
        }
        return 0
}

// PlanReorderRanks plans rank updates for reordering a sibling set.
//
// Inputs:
// - sibs: the current sibling set (including the moved item)
// - movedID: the item being moved
// - insertAt: the index to insert the moved item into the sibling list *after removing it*
//
// Behavior:
//   - Prefer changing only the moved item's rank (fast path).
//   - If the immediate neighbor bounds are not usable (e.g. duplicate ranks),
//     rebalance ranks for the smallest contiguous window around the insertion point that
//     yields valid outer bounds.
func PlanReorderRanks(sibs []*model.Item, movedID string, insertAt int) (ReorderResult, error) {
        movedID = strings.TrimSpace(movedID)
        if movedID == "" {
                return ReorderResult{}, errors.New("missing movedID")
        }
        if len(sibs) == 0 {
                return ReorderResult{RankByID: map[string]string{}}, nil
        }

        // Work on a copy so callers don't get their slice reordered.
        cur := append([]*model.Item{}, sibs...)
        SortItemsByRankOrder(cur)

        // Locate moved item.
        movedIdx := -1
        for i := range cur {
                if strings.TrimSpace(cur[i].ID) == movedID {
                        movedIdx = i
                        break
                }
        }
        if movedIdx < 0 {
                return ReorderResult{}, errors.New("moved item not found in sibling set")
        }
        moved := cur[movedIdx]

        // Build list without moved item.
        rest := make([]*model.Item, 0, len(cur)-1)
        for i := range cur {
                if i == movedIdx {
                        continue
                }
                rest = append(rest, cur[i])
        }

        if insertAt < 0 {
                insertAt = 0
        }
        if insertAt > len(rest) {
                insertAt = len(rest)
        }

        // If the move is a no-op (same position), return empty updates.
        // Compute current index of moved in the "after removal" coordinate system:
        curInsertAt := movedIdx
        if movedIdx > len(rest) {
                curInsertAt = len(rest)
        }
        if insertAt == curInsertAt {
                return ReorderResult{RankByID: map[string]string{}}, nil
        }
        // Window-selection preference: when moving earlier (up), prefer to rebalance to the right
        // (touching the displaced neighbor(s)) rather than pulling in earlier siblings.
        preferRight := insertAt < curInsertAt

        // Build final order.
        final := make([]*model.Item, 0, len(cur))
        final = append(final, rest[:insertAt]...)
        final = append(final, moved)
        final = append(final, rest[insertAt:]...)

        // Fast path: only update moved item's rank if we have usable immediate bounds.
        existing := existingRanksExcluding(final, map[string]bool{movedID: true})
        if r, ok, err := rankBetweenNeighbors(existing, final, insertAt); err == nil && ok {
                if strings.TrimSpace(moved.Rank) != r {
                        return ReorderResult{
                                RankByID: map[string]string{movedID: r},
                        }, nil
                }
                return ReorderResult{RankByID: map[string]string{}}, nil
        } else if err != nil {
                return ReorderResult{}, err
        }

        // Fallback: rebalance a minimal contiguous window around insertion point.
        lo, hi := minimalValidWindow(final, insertAt, preferRight)

        // Outer bounds.
        lower := ""
        upper := ""
        if lo > 0 {
                lower = strings.TrimSpace(final[lo-1].Rank)
        }
        if hi+1 < len(final) {
                upper = strings.TrimSpace(final[hi+1].Rank)
        }

        // Build existing ranks excluding window items (we're about to overwrite them).
        excl := map[string]bool{}
        for i := lo; i <= hi; i++ {
                excl[strings.TrimSpace(final[i].ID)] = true
        }
        existing = existingRanksExcluding(final, excl)

        res := ReorderResult{
                RankByID:     map[string]string{},
                WindowIDs:    make([]string, 0, hi-lo+1),
                UsedFallback: true,
        }
        curLower := lower
        for i := lo; i <= hi; i++ {
                id := strings.TrimSpace(final[i].ID)
                if id == "" {
                        continue
                }
                r, err := RankBetweenUnique(existing, curLower, upper)
                if err != nil {
                        return ReorderResult{}, err
                }
                existing[strings.ToLower(strings.TrimSpace(r))] = true
                res.RankByID[id] = r
                res.WindowIDs = append(res.WindowIDs, id)
                curLower = r
        }
        return res, nil
}

func existingRanksExcluding(items []*model.Item, excludeIDs map[string]bool) map[string]bool {
        existing := map[string]bool{}
        for _, it := range items {
                if it == nil {
                        continue
                }
                id := strings.TrimSpace(it.ID)
                if excludeIDs != nil && excludeIDs[id] {
                        continue
                }
                rn := strings.ToLower(strings.TrimSpace(it.Rank))
                if rn != "" {
                        existing[rn] = true
                }
        }
        return existing
}

// rankBetweenNeighbors attempts to compute a new rank for the moved item using its immediate
// neighbors in the final order. Returns ok=false when bounds are unusable (e.g. lower>=upper).
func rankBetweenNeighbors(existing map[string]bool, final []*model.Item, movedIdx int) (rank string, ok bool, err error) {
        lower := ""
        upper := ""
        if movedIdx > 0 {
                lower = strings.TrimSpace(final[movedIdx-1].Rank)
        }
        if movedIdx+1 < len(final) {
                upper = strings.TrimSpace(final[movedIdx+1].Rank)
        }
        if strings.TrimSpace(lower) != "" && strings.TrimSpace(upper) != "" && !(lower < upper) {
                return "", false, nil
        }
        r, err := RankBetweenUnique(existing, lower, upper)
        if err != nil {
                return "", false, nil
        }
        return r, true, nil
}

// minimalValidWindow finds the smallest contiguous window [lo, hi] containing movedIdx such that
// outer bounds (rank before lo, rank after hi) are either open-ended or strictly increasing.
//
// When multiple windows of the same minimal size are valid, preferRight influences tie-breaking:
// - preferRight=true: prefer windows that expand to the right of movedIdx first
// - preferRight=false: prefer windows that expand to the left of movedIdx first
func minimalValidWindow(final []*model.Item, movedIdx int, preferRight bool) (lo, hi int) {
        if movedIdx < 0 {
                return 0, len(final) - 1
        }
        if movedIdx >= len(final) {
                return 0, len(final) - 1
        }

        valid := func(lo, hi int) bool {
                lower := ""
                upper := ""
                if lo > 0 {
                        lower = strings.TrimSpace(final[lo-1].Rank)
                }
                if hi+1 < len(final) {
                        upper = strings.TrimSpace(final[hi+1].Rank)
                }
                if lower == "" || upper == "" {
                        return true
                }
                return lower < upper
        }

        for size := 1; size <= len(final); size++ {
                startMin := movedIdx - (size - 1)
                if startMin < 0 {
                        startMin = 0
                }
                startMax := movedIdx
                if startMax+size > len(final) {
                        startMax = len(final) - size
                }
                if preferRight {
                        for lo := startMax; lo >= startMin; lo-- {
                                hi := lo + size - 1
                                if !(lo <= movedIdx && movedIdx <= hi) {
                                        continue
                                }
                                if valid(lo, hi) {
                                        return lo, hi
                                }
                        }
                } else {
                        for lo := startMin; lo <= startMax; lo++ {
                                hi := lo + size - 1
                                if !(lo <= movedIdx && movedIdx <= hi) {
                                        continue
                                }
                                if valid(lo, hi) {
                                        return lo, hi
                                }
                        }
                }
        }
        return 0, len(final) - 1
}
