package publish

import (
        "os"
        "path/filepath"
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"
)

func TestRenderItemMarkdown_IncludesDescriptionAndComments(t *testing.T) {
        t.Parallel()

        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)
        actorID := "act-human"
        projectID := "proj-test"
        outlineID := "out-test"
        itemID := "item-test"

        db := &store.DB{
                Version:        1,
                CurrentActorID: actorID,
                NextIDs:        map[string]int{},
                Actors: []model.Actor{
                        {ID: actorID, Kind: model.ActorKindHuman, Name: "Human"},
                },
                Projects: []model.Project{
                        {ID: projectID, Name: "Test Project", CreatedBy: actorID, CreatedAt: now},
                },
                Outlines: []model.Outline{
                        {ID: outlineID, ProjectID: projectID, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now},
                },
                Items: []model.Item{
                        {
                                ID:           itemID,
                                ProjectID:    projectID,
                                OutlineID:    outlineID,
                                Rank:         "h",
                                Title:        "Hello",
                                Description:  "Some **markdown**.",
                                StatusID:     "todo",
                                OwnerActorID: actorID,
                                CreatedBy:    actorID,
                                CreatedAt:    now,
                                UpdatedAt:    now,
                        },
                },
                Comments: []model.Comment{
                        {ID: "cmt-1", ItemID: itemID, AuthorID: actorID, Body: "Comment body", CreatedAt: now.Add(1 * time.Hour)},
                },
        }

        md, err := RenderItemMarkdown(db, itemID, RenderOptions{ActorID: actorID})
        if err != nil {
                t.Fatalf("RenderItemMarkdown: %v", err)
        }
        if !strings.Contains(md, "# Hello") {
                t.Fatalf("expected title header, got:\n%s", md)
        }
        if !strings.Contains(md, "## Description") || !strings.Contains(md, "Some **markdown**.") {
                t.Fatalf("expected description section, got:\n%s", md)
        }
        if !strings.Contains(md, "## Comments") || !strings.Contains(md, "Comment body") {
                t.Fatalf("expected comments section, got:\n%s", md)
        }
}

func TestWriteOutline_WritesIndexAndItems(t *testing.T) {
        t.Parallel()

        now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)
        actorID := "act-human"
        projectID := "proj-test"
        outlineID := "out-test"

        db := &store.DB{
                Version:        1,
                CurrentActorID: actorID,
                NextIDs:        map[string]int{},
                Actors: []model.Actor{
                        {ID: actorID, Kind: model.ActorKindHuman, Name: "Human"},
                },
                Projects: []model.Project{
                        {ID: projectID, Name: "Test Project", CreatedBy: actorID, CreatedAt: now},
                },
                Outlines: []model.Outline{
                        {ID: outlineID, ProjectID: projectID, StatusDefs: store.DefaultOutlineStatusDefs(), CreatedBy: actorID, CreatedAt: now},
                },
                Items: []model.Item{
                        {ID: "item-a", ProjectID: projectID, OutlineID: outlineID, Rank: "h", Title: "A", StatusID: "todo", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                        {ID: "item-b", ProjectID: projectID, OutlineID: outlineID, Rank: "h0", Title: "B", StatusID: "doing", OwnerActorID: actorID, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now},
                },
        }

        to := t.TempDir()
        res, err := WriteOutline(db, outlineID, to, WriteOptions{ActorID: actorID, Overwrite: true})
        if err != nil {
                t.Fatalf("WriteOutline: %v", err)
        }
        if len(res.Written) < 3 {
                t.Fatalf("expected at least 3 written files; got %d (%v)", len(res.Written), res.Written)
        }
        indexPath := filepath.Join(to, "outlines", outlineID, "index.md")
        if _, err := os.Stat(indexPath); err != nil {
                t.Fatalf("stat index.md: %v", err)
        }
        itemPath := filepath.Join(to, "outlines", outlineID, "items", "item-a.md")
        if _, err := os.Stat(itemPath); err != nil {
                t.Fatalf("stat item-a.md: %v", err)
        }
}
