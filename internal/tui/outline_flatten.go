package tui

import (
        "sort"
        "strings"

        "clarity-cli/internal/model"
)

func flattenOutline(items []model.Item) []outlineRow {
        // Build parent -> children map (siblings sorted by Order).
        children := map[string][]model.Item{}
        var roots []model.Item
        for _, it := range items {
                if it.ParentID == nil || *it.ParentID == "" {
                        roots = append(roots, it)
                        continue
                }
                children[*it.ParentID] = append(children[*it.ParentID], it)
        }

        sort.Slice(roots, func(i, j int) bool { return compareOutlineItems(roots[i], roots[j]) < 0 })
        for pid := range children {
                sibs := children[pid]
                sort.Slice(sibs, func(i, j int) bool { return compareOutlineItems(sibs[i], sibs[j]) < 0 })
                children[pid] = sibs
        }

        var out []outlineRow
        var walk func(it model.Item, depth int)
        walk = func(it model.Item, depth int) {
                out = append(out, outlineRow{item: it, depth: depth})
                for _, ch := range children[it.ID] {
                        walk(ch, depth+1)
                }
        }
        for _, r := range roots {
                walk(r, 0)
        }
        return out
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
                return 0
        }
        if a.CreatedAt.Before(b.CreatedAt) {
                return -1
        }
        if a.CreatedAt.After(b.CreatedAt) {
                return 1
        }
        return 0
}
