package tui

import (
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func TestProjectsAndOutlinesHaveAddRow(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 12, 22, 0, 0, 0, 0, time.UTC)
	actorID := "act-a"
	projectID := "proj-a"
	outlineID := "out-a"

	db := &store.DB{
		Version:          1,
		CurrentActorID:   actorID,
		CurrentProjectID: projectID,
		NextIDs:          map[string]int{},
		Actors:           []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "A"}},
		Projects:         []model.Project{{ID: projectID, Name: "P", CreatedBy: actorID, CreatedAt: now}},
		Outlines:         []model.Outline{{ID: outlineID, ProjectID: projectID, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now}},
		Items:            []model.Item{},
		Deps:             []model.Dependency{},
		Comments:         []model.Comment{},
		Worklog:          []model.WorklogEntry{},
	}

	m := newAppModelWithWorkspace(t.TempDir(), db, "ws")
	m.refreshProjects()
	projects := m.projectsList.Items()
	if len(projects) == 0 {
		t.Fatalf("expected projects list items")
	}
	if _, ok := projects[len(projects)-1].(addProjectRow); !ok {
		t.Fatalf("expected last projects row to be addProjectRow; got %T", projects[len(projects)-1])
	}

	m.selectedProjectID = projectID
	m.view = viewOutlines
	m.refreshOutlines(projectID)
	outlines := m.outlinesList.Items()
	if len(outlines) == 0 {
		t.Fatalf("expected outlines list items")
	}
	if _, ok := outlines[len(outlines)-1].(addOutlineRow); !ok {
		t.Fatalf("expected last outlines row to be addOutlineRow; got %T", outlines[len(outlines)-1])
	}
	for _, it := range outlines {
		if _, ok := it.(projectUploadsRow); ok {
			t.Fatalf("did not expect projectUploadsRow when project has no attachments")
		}
	}
}

func TestAddRowEnterOpensSameModalAsN(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 12, 22, 0, 0, 0, 0, time.UTC)
	actorID := "act-a"
	projectID := "proj-a"

	db := &store.DB{
		Version:          1,
		CurrentActorID:   actorID,
		CurrentProjectID: projectID,
		NextIDs:          map[string]int{},
		Actors:           []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "A"}},
		Projects:         []model.Project{{ID: projectID, Name: "P", CreatedBy: actorID, CreatedAt: now}},
		Outlines:         []model.Outline{},
		Items:            []model.Item{},
		Deps:             []model.Dependency{},
		Comments:         []model.Comment{},
		Worklog:          []model.WorklogEntry{},
	}

	// Projects view: "n" opens modalNewProject.
	mp := newAppModelWithWorkspace(t.TempDir(), db, "ws")
	mp.view = viewProjects
	mp.refreshProjects()
	nm, _ := mp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	mp2 := nm.(appModel)
	if mp2.modal != modalNewProject {
		t.Fatalf("expected modalNewProject; got %v", mp2.modal)
	}

	// Projects view: enter on "+ New" opens modalNewProject.
	mp3 := newAppModelWithWorkspace(t.TempDir(), db, "ws")
	mp3.view = viewProjects
	mp3.refreshProjects()
	selectListItemByID(&mp3.projectsList, "__add__")
	em, _ := mp3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mp4 := em.(appModel)
	if mp4.modal != modalNewProject {
		t.Fatalf("expected modalNewProject from enter on add row; got %v", mp4.modal)
	}

	// Outlines view: "n" opens modalNewOutline.
	mo := newAppModelWithWorkspace(t.TempDir(), db, "ws")
	mo.view = viewOutlines
	mo.selectedProjectID = projectID
	mo.refreshOutlines(projectID)
	nm2, _ := mo.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	mo2 := nm2.(appModel)
	if mo2.modal != modalNewOutline {
		t.Fatalf("expected modalNewOutline; got %v", mo2.modal)
	}

	// Outlines view: enter on "+ New" opens modalNewOutline.
	mo3 := newAppModelWithWorkspace(t.TempDir(), db, "ws")
	mo3.view = viewOutlines
	mo3.selectedProjectID = projectID
	mo3.refreshOutlines(projectID)
	selectListItemByID(&mo3.outlinesList, "__add__")
	em2, _ := mo3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mo4 := em2.(appModel)
	if mo4.modal != modalNewOutline {
		t.Fatalf("expected modalNewOutline from enter on add row; got %v", mo4.modal)
	}
}

func TestUploadsRowOnlyShowsWhenProjectHasAttachments(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 12, 22, 0, 0, 0, 0, time.UTC)
	actorID := "act-a"
	projectID := "proj-a"
	outlineID := "out-a"
	itemID := "item-a"

	db := &store.DB{
		Version:          1,
		CurrentActorID:   actorID,
		CurrentProjectID: projectID,
		NextIDs:          map[string]int{},
		Actors:           []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "A"}},
		Projects:         []model.Project{{ID: projectID, Name: "P", CreatedBy: actorID, CreatedAt: now}},
		Outlines:         []model.Outline{{ID: outlineID, ProjectID: projectID, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now}},
		Items: []model.Item{{
			ID:        itemID,
			ProjectID: projectID,
			OutlineID: outlineID,
			Title:     "T",
			CreatedBy: actorID,
			CreatedAt: now,
		}},
		Deps:     []model.Dependency{},
		Comments: []model.Comment{},
		Worklog:  []model.WorklogEntry{},
		Attachments: []model.Attachment{{
			ID:           "att-a",
			EntityKind:   "item",
			EntityID:     itemID,
			OriginalName: "a.txt",
			Path:         "attachments/a.txt",
			CreatedBy:    actorID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}},
	}

	m := newAppModelWithWorkspace(t.TempDir(), db, "ws")
	m.view = viewOutlines
	m.selectedProjectID = projectID
	m.refreshOutlines(projectID)

	found := false
	for _, it := range m.outlinesList.Items() {
		if _, ok := it.(projectUploadsRow); ok {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected projectUploadsRow when project has attachments")
	}

	selectListItemByID(&m.outlinesList, "__uploads__")
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := nm.(appModel)
	if m2.view != viewProjectAttachments {
		t.Fatalf("expected viewProjectAttachments; got %v", m2.view)
	}
}
