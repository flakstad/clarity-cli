package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

var sgrRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripSGR(s string) string {
	return sgrRE.ReplaceAllString(s, "")
}

func TestViewOutline_SplitPreview_RendersNoDetailPane(t *testing.T) {
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
	// Preview mode has been removed: outline should never render a detail pane.
	m.showPreview = true
	m.modal = modalNone
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.width = 100
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
	if strings.Contains(out, "Description") || strings.Contains(out, "Created by: ") {
		t.Fatalf("did not expect detail pane content in outline view; got: %q", out)
	}
}

func TestViewOutline_SinglePane_IsLeftAlignedWithOuterMargin(t *testing.T) {
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
			StatusID:     "todo",
			OwnerActorID: "act-test",
			CreatedBy:    "act-test",
			CreatedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		}},
	}

	m := newAppModel(t.TempDir(), db)
	m.view = viewOutline
	m.showPreview = false // force single-pane
	m.modal = modalNone
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.width = 120
	m.height = 30

	m.itemsList.SetItems([]list.Item{
		outlineRowItem{row: outlineRow{item: db.Items[0]}, outline: db.Outlines[0]},
	})

	out := m.viewOutline()
	wantListW := m.width - 2*splitOuterMargin
	if wantListW < 10 {
		wantListW = m.width
	}
	if got := m.itemsList.Width(); got != wantListW {
		t.Fatalf("expected outline list width=%d, got %d", wantListW, got)
	}
	lines := strings.Split(out, "\n")
	if len(lines) <= topPadLines {
		t.Fatalf("expected output to include top padding + content; got %d lines", len(lines))
	}

	headerLine := stripSGR(lines[topPadLines])
	idx := strings.Index(headerLine, m.breadcrumbText())
	if idx < 0 {
		t.Fatalf("expected breadcrumb row to contain breadcrumb; got: %q", headerLine)
	}
	if idx != splitOuterMargin {
		t.Fatalf("expected breadcrumb to start at column=%d (outer margin), got %d (line=%q)", splitOuterMargin, idx, headerLine)
	}
}

func TestViewProjects_IsLeftAlignedWithOuterMarginAndFullWidth(t *testing.T) {
	db := &store.DB{
		CurrentActorID: "act-test",
		Actors:         []model.Actor{{ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"}},
		Projects: []model.Project{{
			ID:        "proj-a",
			Name:      "Project A",
			CreatedBy: "act-test",
			CreatedAt: time.Now().UTC(),
		}},
	}
	m := newAppModel(t.TempDir(), db)
	m.view = viewProjects
	m.modal = modalNone
	m.width = 120
	m.height = 30

	out := m.viewProjects()
	wantW := m.width - 2*splitOuterMargin
	if wantW < 10 {
		wantW = m.width
	}
	if got := m.projectsList.Width(); got != wantW {
		t.Fatalf("expected projects list width=%d, got %d", wantW, got)
	}
	lines := strings.Split(out, "\n")
	if len(lines) <= topPadLines {
		t.Fatalf("expected output to include top padding + content; got %d lines", len(lines))
	}
	headerLine := stripSGR(lines[topPadLines])
	idx := strings.Index(headerLine, m.breadcrumbText())
	if idx < 0 {
		t.Fatalf("expected breadcrumb row to contain breadcrumb; got: %q", headerLine)
	}
	if idx != splitOuterMargin {
		t.Fatalf("expected breadcrumb to start at column=%d (outer margin), got %d (line=%q)", splitOuterMargin, idx, headerLine)
	}
}

func TestViewOutlines_IsLeftAlignedWithOuterMarginAndFullWidth(t *testing.T) {
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
	}
	m := newAppModel(t.TempDir(), db)
	m.view = viewOutlines
	m.modal = modalNone
	m.selectedProjectID = "proj-a"
	m.width = 120
	m.height = 30

	out := m.viewOutlines()
	wantW := m.width - 2*splitOuterMargin
	if wantW < 10 {
		wantW = m.width
	}
	if got := m.outlinesList.Width(); got != wantW {
		t.Fatalf("expected outlines list width=%d, got %d", wantW, got)
	}
	lines := strings.Split(out, "\n")
	if len(lines) <= topPadLines {
		t.Fatalf("expected output to include top padding + content; got %d lines", len(lines))
	}
	headerLine := stripSGR(lines[topPadLines])
	idx := strings.Index(headerLine, m.breadcrumbText())
	if idx < 0 {
		t.Fatalf("expected breadcrumb row to contain breadcrumb; got: %q", headerLine)
	}
	if idx != splitOuterMargin {
		t.Fatalf("expected breadcrumb to start at column=%d (outer margin), got %d (line=%q)", splitOuterMargin, idx, headerLine)
	}
}

func TestUpdate_PreviewComputeMsg_IsDebouncedBySeqAndSelection(t *testing.T) {
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
		Items: []model.Item{
			{
				ID:           "item-a",
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				Rank:         "h",
				Title:        "A",
				StatusID:     "todo",
				OwnerActorID: "act-test",
				CreatedBy:    "act-test",
				CreatedAt:    time.Now().UTC(),
				UpdatedAt:    time.Now().UTC(),
			},
			{
				ID:           "item-b",
				ProjectID:    "proj-a",
				OutlineID:    "out-a",
				Rank:         "i",
				Title:        "B",
				StatusID:     "todo",
				OwnerActorID: "act-test",
				CreatedBy:    "act-test",
				CreatedAt:    time.Now().UTC(),
				UpdatedAt:    time.Now().UTC(),
			},
		},
	}

	m := newAppModel(t.TempDir(), db)
	m.view = viewOutline
	m.modal = modalNone
	m.selectedProjectID = "proj-a"
	m.selectedOutlineID = "out-a"
	m.width = 100
	m.height = 30

	// Preview mode has been removed: preview compute messages should be no-ops.
	m.previewCacheForID = ""
	m.previewCache = ""

	mm, _ := m.Update(previewComputeMsg{seq: 1, itemID: "item-a", w: 40, h: 10})
	m = mm.(appModel)
	if m.previewCacheForID != "" || strings.TrimSpace(m.previewCache) != "" {
		t.Fatalf("expected previewComputeMsg to be a no-op; got cacheForID=%q cacheLen=%d", m.previewCacheForID, len(m.previewCache))
	}
}

func TestView_ShowsResizingOverlay(t *testing.T) {
	db := &store.DB{
		CurrentActorID: "act-test",
		Actors:         []model.Actor{{ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"}},
	}
	m := newAppModel(t.TempDir(), db)
	m.view = viewProjects
	m.width = 60
	m.height = 20
	m.resizing = true

	out := m.View()
	if !strings.Contains(out, "Resizing") {
		t.Fatalf("expected resizing overlay to contain 'Resizing'; got: %q", out)
	}
	// Ensure we render "Resizing" only once (centered), not repeated on every row.
	if n := strings.Count(out, "Resizing"); n != 1 {
		t.Fatalf("expected resizing overlay to contain 'Resizing' exactly once; got %d occurrences", n)
	}
	// Ensure we render a full-height block (stable during resize).
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Fatalf("expected resizing overlay to be exactly %d lines tall; got %d", m.height, len(lines))
	}
}

func TestUpdate_WindowSizeMsg_DebouncesResizingFlag(t *testing.T) {
	db := &store.DB{
		CurrentActorID: "act-test",
		Actors:         []model.Actor{{ID: "act-test", Kind: model.ActorKindHuman, Name: "tester"}},
	}
	m := newAppModel(t.TempDir(), db)

	// First WindowSizeMsg is treated as initial sizing (no resizing overlay).
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = mm.(appModel)
	if !m.resizing {
		// ok
	} else {
		t.Fatalf("expected resizing=false after initial WindowSizeMsg")
	}
	if m.resizeSeq != 0 {
		t.Fatalf("expected resizeSeq=0 after initial WindowSizeMsg, got %d", m.resizeSeq)
	}

	// Second WindowSizeMsg -> enter resizing mode, seq=1
	mm, _ = m.Update(tea.WindowSizeMsg{Width: 81, Height: 25})
	m = mm.(appModel)
	if !m.resizing {
		t.Fatalf("expected resizing=true after subsequent WindowSizeMsg")
	}
	if m.resizeSeq != 1 {
		t.Fatalf("expected resizeSeq=1 after subsequent WindowSizeMsg, got %d", m.resizeSeq)
	}

	// Stale done message should NOT clear.
	mm, _ = m.Update(resizeDoneMsg{seq: 0})
	m = mm.(appModel)
	if !m.resizing {
		t.Fatalf("expected resizing to remain true for stale resizeDoneMsg")
	}

	// Current done message clears.
	mm, _ = m.Update(resizeDoneMsg{seq: 1})
	m = mm.(appModel)
	if m.resizing {
		t.Fatalf("expected resizing=false after latest resizeDoneMsg")
	}
}
