package tui

import (
        "testing"
        "time"

        "clarity-cli/internal/model"
)

func TestComputeChildProgress_DeepHierarchyCountsLeavesOnly(t *testing.T) {
        now := time.Date(2025, 12, 21, 0, 0, 0, 0, time.UTC)
        outline := model.Outline{
                ID:        "out-a",
                ProjectID: "proj-a",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "TODO", IsEndState: false},
                        {ID: "done", Label: "DONE", IsEndState: true},
                },
                CreatedBy: "act-a",
                CreatedAt: now,
        }

        // Chain: a -> b -> c (leaf)
        a := model.Item{ID: "item-a", OutlineID: outline.ID, Title: "A", StatusID: "todo", CreatedAt: now, UpdatedAt: now}
        bParent := "item-a"
        b := model.Item{ID: "item-b", OutlineID: outline.ID, ParentID: &bParent, Title: "B", StatusID: "todo", CreatedAt: now, UpdatedAt: now}
        cParent := "item-b"
        c := model.Item{ID: "item-c", OutlineID: outline.ID, ParentID: &cParent, Title: "C", StatusID: "done", CreatedAt: now, UpdatedAt: now}

        children := map[string][]model.Item{
                "item-a": {b},
                "item-b": {c},
        }

        got := computeChildProgress(outline, children)

        // Direct children only: item-a tracks item-b (not item-c).
        if p := got["item-a"]; p != [2]int{0, 1} {
                t.Fatalf("expected item-a progress [done,total]=[0,1], got %v", p)
        }
        if p := got["item-b"]; p != [2]int{1, 1} {
                t.Fatalf("expected item-b progress [done,total]=[1,1], got %v", p)
        }

        _ = a // a is not referenced directly in the adjacency map; included for clarity.
}

func TestComputeChildProgress_BranchingCountsLeafDescendants(t *testing.T) {
        now := time.Date(2025, 12, 21, 0, 0, 0, 0, time.UTC)
        outline := model.Outline{
                ID:        "out-a",
                ProjectID: "proj-a",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "TODO", IsEndState: false},
                        {ID: "done", Label: "DONE", IsEndState: true},
                },
                CreatedBy: "act-a",
                CreatedAt: now,
        }

        // Tree:
        //   root
        //     group (has child leaf1)
        //     leaf2
        root := model.Item{ID: "item-root", OutlineID: outline.ID, Title: "root", StatusID: "todo", CreatedAt: now, UpdatedAt: now}
        groupParent := "item-root"
        group := model.Item{ID: "item-group", OutlineID: outline.ID, ParentID: &groupParent, Title: "group", StatusID: "todo", CreatedAt: now, UpdatedAt: now}
        leaf1Parent := "item-group"
        leaf1 := model.Item{ID: "item-leaf1", OutlineID: outline.ID, ParentID: &leaf1Parent, Title: "leaf1", StatusID: "done", CreatedAt: now, UpdatedAt: now}
        leaf2Parent := "item-root"
        leaf2 := model.Item{ID: "item-leaf2", OutlineID: outline.ID, ParentID: &leaf2Parent, Title: "leaf2", StatusID: "todo", CreatedAt: now, UpdatedAt: now}

        children := map[string][]model.Item{
                "item-root":  {group, leaf2},
                "item-group": {leaf1},
        }

        got := computeChildProgress(outline, children)

        // Direct children only: root tracks group + leaf2 (both todo) => 0/2
        if p := got["item-root"]; p != [2]int{0, 2} {
                t.Fatalf("expected item-root progress [done,total]=[0,2], got %v", p)
        }
        // group has leaf1 => 1/1
        if p := got["item-group"]; p != [2]int{1, 1} {
                t.Fatalf("expected item-group progress [done,total]=[1,1], got %v", p)
        }

        _ = root
}

func TestComputeChildProgress_DirectChildDoneDoesNotPropagate(t *testing.T) {
        now := time.Date(2025, 12, 21, 0, 0, 0, 0, time.UTC)
        outline := model.Outline{
                ID:        "out-a",
                ProjectID: "proj-a",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "TODO", IsEndState: false},
                        {ID: "done", Label: "DONE", IsEndState: true},
                },
                CreatedBy: "act-a",
                CreatedAt: now,
        }

        // Shape:
        // 1
        // - 2
        //   - 3 (done)
        //   - 4 (todo)
        one := model.Item{ID: "item-1", OutlineID: outline.ID, Title: "1", StatusID: "todo", CreatedAt: now, UpdatedAt: now}
        twoParent := "item-1"
        two := model.Item{ID: "item-2", OutlineID: outline.ID, ParentID: &twoParent, Title: "2", StatusID: "todo", CreatedAt: now, UpdatedAt: now}
        threeParent := "item-2"
        three := model.Item{ID: "item-3", OutlineID: outline.ID, ParentID: &threeParent, Title: "3", StatusID: "done", CreatedAt: now, UpdatedAt: now}
        fourParent := "item-2"
        four := model.Item{ID: "item-4", OutlineID: outline.ID, ParentID: &fourParent, Title: "4", StatusID: "todo", CreatedAt: now, UpdatedAt: now}

        children := map[string][]model.Item{
                "item-1": {two},
                "item-2": {three, four},
        }

        got := computeChildProgress(outline, children)

        // Only 2's direct children affect 2; 1 only tracks 2.
        if p := got["item-2"]; p != [2]int{1, 2} {
                t.Fatalf("expected item-2 progress [done,total]=[1,2], got %v", p)
        }
        if p := got["item-1"]; p != [2]int{0, 1} {
                t.Fatalf("expected item-1 progress [done,total]=[0,1], got %v", p)
        }

        _ = one
}

func TestComputeChildProgress_IgnoresChildrenWithoutStatus(t *testing.T) {
        now := time.Date(2025, 12, 21, 0, 0, 0, 0, time.UTC)
        outline := model.Outline{
                ID:        "out-a",
                ProjectID: "proj-a",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "todo", Label: "TODO", IsEndState: false},
                        {ID: "done", Label: "DONE", IsEndState: true},
                },
                CreatedBy: "act-a",
                CreatedAt: now,
        }

        parent := model.Item{ID: "item-parent", OutlineID: outline.ID, Title: "parent", StatusID: "todo", CreatedAt: now, UpdatedAt: now}
        parentID := parent.ID

        noStatus := model.Item{ID: "item-nostatus", OutlineID: outline.ID, ParentID: &parentID, Title: "no status", StatusID: "", CreatedAt: now, UpdatedAt: now}
        todo := model.Item{ID: "item-todo", OutlineID: outline.ID, ParentID: &parentID, Title: "todo", StatusID: "todo", CreatedAt: now, UpdatedAt: now}
        done := model.Item{ID: "item-done", OutlineID: outline.ID, ParentID: &parentID, Title: "done", StatusID: "done", CreatedAt: now, UpdatedAt: now}

        children := map[string][]model.Item{
                parent.ID: {noStatus, todo, done},
        }

        got := computeChildProgress(outline, children)
        // Only children with explicit status are counted: todo + done => total=2, done=1.
        if p := got[parent.ID]; p != [2]int{1, 2} {
                t.Fatalf("expected parent progress [done,total]=[1,2] (excluding no-status child), got %v", p)
        }
}
