package tui

import (
        "strings"
        "testing"

        "clarity-cli/internal/model"
)

func TestRenderOutlineColumns_OnlyTopLevelItems(t *testing.T) {
        ol := model.Outline{
                ID: "out-1",
                StatusDefs: []model.OutlineStatusDef{
                        {ID: "s1", Label: "Todo"},
                },
        }

        items := []model.Item{
                {ID: "a", OutlineID: ol.ID, Title: "Top", StatusID: "s1"},
                {ID: "b", OutlineID: ol.ID, Title: "Child", ParentID: strPtr("a"), StatusID: "s1"},
        }

        board := buildOutlineColumnsBoard(nil, ol, items)
        out := renderOutlineColumns(ol, board, outlineColumnsSelection{}, 80, 10)
        if strings.Contains(out, "Child") {
                t.Fatalf("expected nested item title to be excluded from columns output, got=%q", out)
        }
        if !strings.Contains(out, "Top") {
                t.Fatalf("expected top-level item title to be present in columns output, got=%q", out)
        }
        if strings.Contains(out, "- Top") {
                t.Fatalf("expected columns view to not render list-style prefixes, got=%q", out)
        }
        // Count in the column header should reflect only top-level items.
        if !strings.Contains(out, "Todo (1)") {
                t.Fatalf("expected header count to be 1, got=%q", out)
        }
}
