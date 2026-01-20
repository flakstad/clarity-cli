package tui

import (
	"sort"
	"strings"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func flattenOutline(db *store.DB, outline model.Outline, items []model.Item, collapsed map[string]bool) []outlineRow {
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
	var walk func(it model.Item, depth int, parentCheckbox bool)
	walk = func(it model.Item, depth int, parentCheckbox bool) {
		checkbox := parentCheckbox
		switch strings.TrimSpace(it.ItemKind) {
		case "checkbox":
			checkbox = true
		case "status":
			checkbox = false
		}
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
			checkbox:      checkbox,
			doneChildren:  doneChildren,
			totalChildren: totalChildren,
		})
		if collapsed[it.ID] {
			return
		}
		childrenCheckbox := strings.TrimSpace(it.ChildrenKind) == "checkbox"
		for _, ch := range children[it.ID] {
			walk(ch, depth+1, childrenCheckbox)
		}
	}
	for _, r := range roots {
		rootCheckbox := false
		if db != nil && r.ParentID != nil && strings.TrimSpace(*r.ParentID) != "" {
			if p, ok := db.FindItem(strings.TrimSpace(*r.ParentID)); ok && p != nil && strings.TrimSpace(p.ChildrenKind) == "checkbox" {
				rootCheckbox = true
			}
		}
		walk(r, 0, rootCheckbox)
	}
	return out
}

func computeChildProgress(outline model.Outline, children map[string][]model.Item) map[string][2]int {
	// Progress cookies are based on *direct children* only.
	//
	// This keeps parent nodes stable and predictable:
	// - If item A has a single child B, A's cookie is 0/1 until B itself is DONE,
	//   regardless of B's internal subtree.
	// - Deep hierarchies don't inflate denominators for ancestors.
	out := map[string][2]int{}
	for pid, ch := range children {
		done, total := countProgressChildren(outline, ch)
		out[pid] = [2]int{done, total}
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
