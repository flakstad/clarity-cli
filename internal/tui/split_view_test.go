package tui

import (
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/bubbles/list"
        xansi "github.com/charmbracelet/x/ansi"
)

func TestViewOutline_SplitPreview_PanesStayFixedWidthPerLine(t *testing.T) {
        db := &store.DB{
                CurrentActorID: "act-test",
                Actors:         []model.Actor{{ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"}},
                Projects: []model.Project{{
                        ID:        "proj-a",
                        Name:      "Project A",
                        CreatedBy: "act-test",
                        CreatedAt: time.Now().UTC(),
                }},
                Outlines: []model.Outline{{
                        ID:         "out-a",
                        ProjectID:  "proj-a",
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  "act-test",
                        CreatedAt:  time.Now().UTC(),
                }},
                Items: []model.Item{{
                        ID:           "item-a",
                        ProjectID:    "proj-a",
                        OutlineID:    "out-a",
                        Rank:         "h",
                        Title:        "Title",
                        Description:  strings.Repeat("X", 500), // ensure detail view has lots of wrapped content
                        StatusID:     "todo",
                        Priority:     false,
                        OnHold:       false,
                        Archived:     false,
                        OwnerActorID: "act-test",
                        CreatedBy:    "act-test",
                        CreatedAt:    time.Now().UTC(),
                        UpdatedAt:    time.Now().UTC(),
                }},
        }

        m := newAppModel(t.TempDir(), db)
        m.view = viewOutline
        m.showPreview = true
        m.modal = modalNone
        m.selectedProjectID = "proj-a"
        m.selectedOutlineID = "out-a"
        m.width = 40
        m.height = 30

        m.itemsList.SetItems([]list.Item{
                outlineRowItem{
                        row: outlineRow{
                                item:          db.Items[0],
                                depth:         0,
                                hasChildren:   false,
                                collapsed:     false,
                                doneChildren:  0,
                                totalChildren: 0,
                        },
                        outline: db.Outlines[0],
                },
        })

        out := m.viewOutline()
        lines := strings.Split(out, "\n")

        frameH := m.height - 6
        if frameH < 8 {
                frameH = 8
        }
        bodyHeight := frameH - (topPadLines + breadcrumbGap + 2)
        if bodyHeight < 6 {
                bodyHeight = 6
        }

        // The join block starts after:
        // - topPadLines leading blank lines
        // - 1 breadcrumb line
        // - breadcrumbGap blank lines
        start := topPadLines + 1 + breadcrumbGap

        if len(lines) < start+bodyHeight {
                t.Fatalf("unexpected outline view height: got %d lines, need at least %d", len(lines), start+bodyHeight)
        }

        for i := start; i < start+bodyHeight; i++ {
                if w := xansi.StringWidth(lines[i]); w != m.width {
                        t.Fatalf("expected split-view line %d to be exactly %d cols wide; got %d: %q", i, m.width, w, lines[i])
                }
        }
}
