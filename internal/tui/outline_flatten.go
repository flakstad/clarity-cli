package tui

import (
        "sort"
        "strings"

        "clarity-cli/internal/model"
)

func flattenOutline(outline model.Outline, items []model.Item, collapsed map[string]bool) []outlineRow {
        // Build parent -> children map (siblings sorted by Order).
        children := map[string][]model.Item{}
        hasChildren := map[string]bool{}
        var roots []model.Item
        present := map[string]bool{}
        for _, it := range items {
                present[it.ID] = true
        }
        for _, it := range items {
                if it.ParentID == nil || strings.TrimSpace(*it.ParentID) == "" {
                        roots = append(roots, it)
                        continue
                }
                // If a parent is missing (e.g. archived), treat this as a root to avoid "orphaning"
                // the subtree in the outline view.
                if !present[*it.ParentID] {
                        roots = append(roots, it)
                        continue
                }
                children[*it.ParentID] = append(children[*it.ParentID], it)
        }
        for pid, ch := range children {
                if len(ch) > 0 {
                        hasChildren[pid] = true
                }
        }

        sort.Slice(roots, func(i, j int) bool { return compareOutlineItems(roots[i], roots[j]) < 0 })
        for pid := range children {
                sibs := children[pid]
                sort.Slice(sibs, func(i, j int) bool { return compareOutlineItems(sibs[i], sibs[j]) < 0 })
                children[pid] = sibs
        }

        progress := computeChildProgress(outline, children)

        var out []outlineRow
        var walk func(it model.Item, depth int)
        walk = func(it model.Item, depth int) {
                doneChildren := 0
                totalChildren := 0
                if p, ok := progress[it.ID]; ok {
                        doneChildren = p[0]
                        totalChildren = p[1]
                }
                out = append(out, outlineRow{
                        item:          it,
                        depth:         depth,
                        hasChildren:   hasChildren[it.ID],
                        collapsed:     collapsed[it.ID],
                        doneChildren:  doneChildren,
                        totalChildren: totalChildren,
                })
                if collapsed[it.ID] {
                        return
                }
                for _, ch := range children[it.ID] {
                        walk(ch, depth+1)
                }
        }
        for _, r := range roots {
                walk(r, 0)
        }
        return out
}

func computeChildProgress(outline model.Outline, children map[string][]model.Item) map[string][2]int {
        isDone := func(statusID string) bool {
                for _, def := range outline.StatusDefs {
                        if def.ID == statusID {
                                return def.IsEndState
                        }
                }
                return strings.ToLower(strings.TrimSpace(statusID)) == "done"
        }

        memo := map[string][2]int{}
        visiting := map[string]bool{}

        var rec func(id string) (int, int)
        rec = func(id string) (int, int) {
                if v, ok := memo[id]; ok {
                        return v[0], v[1]
                }
                if visiting[id] {
                        return 0, 0
                }
                visiting[id] = true

                done := 0
                total := 0
                for _, ch := range children[id] {
                        total++
                        if isDone(ch.StatusID) {
                                done++
                        }
                        d2, t2 := rec(ch.ID)
                        done += d2
                        total += t2
                }

                visiting[id] = false
                memo[id] = [2]int{done, total}
                return done, total
        }

        // Ensure every node in the adjacency list has an entry.
        for pid := range children {
                _, _ = rec(pid)
        }
        return memo
}

func compareOutlineItems(a, b model.Item) int {
        ra := strings.TrimSpace(a.Rank)
        rb := strings.TrimSpace(b.Rank)
        if ra != "" && rb != "" {
                if ra < rb {
                        return -1
                }
                if ra > rb {
                        return 1
                }
                // Deterministic tie-break: equal ranks must still produce a stable ordering,
                // otherwise sort.Slice may reshuffle equal elements between renders (causing
                // "jumps" when moving items).
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
