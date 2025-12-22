package store

import (
        "encoding/json"
        "os"
        "testing"
        "time"

        "clarity-cli/internal/model"
)

func TestSQLiteStateStore_SaveLoad_RoundTrip(t *testing.T) {
        withEnv(t, envEventLogBackend, string(EventLogBackendSQLite), func() {
                withEnv(t, "CLARITY_CONFIG_DIR", t.TempDir(), func() {
                        dir := t.TempDir()
                        s := Store{Dir: dir}

                        now := time.Now().UTC()
                        db := &DB{
                                Version:          1,
                                CurrentActorID:   "act-a",
                                CurrentProjectID: "proj-a",
                                NextIDs:          map[string]int{},
                                Actors:           []model.Actor{{ID: "act-a", Kind: model.ActorKindHuman, Name: "A"}},
                                Projects:         []model.Project{{ID: "proj-a", Name: "P", CreatedBy: "act-a", CreatedAt: now}},
                                Outlines:         []model.Outline{{ID: "out-a", ProjectID: "proj-a", StatusDefs: DefaultOutlineStatusDefs(), CreatedBy: "act-a", CreatedAt: now}},
                                Items:            []model.Item{{ID: "item-a", ProjectID: "proj-a", OutlineID: "out-a", Rank: "h", Title: "T", StatusID: "todo", OwnerActorID: "act-a", CreatedBy: "act-a", CreatedAt: now, UpdatedAt: now}},
                                Deps:             []model.Dependency{},
                                Comments:         []model.Comment{},
                                Worklog:          []model.WorklogEntry{},
                        }

                        if err := s.Save(db); err != nil {
                                t.Fatalf("save sqlite: %v", err)
                        }

                        got, err := s.Load()
                        if err != nil {
                                t.Fatalf("load sqlite: %v", err)
                        }
                        if got.CurrentActorID != "act-a" || got.CurrentProjectID != "proj-a" {
                                t.Fatalf("unexpected meta: actor=%q project=%q", got.CurrentActorID, got.CurrentProjectID)
                        }
                        if len(got.Items) != 1 || got.Items[0].ID != "item-a" {
                                t.Fatalf("unexpected items: %+v", got.Items)
                        }
                })
        })
}

func TestSQLiteStateStore_ImportsLegacyDBJSONOnce(t *testing.T) {
        withEnv(t, envEventLogBackend, string(EventLogBackendSQLite), func() {
                withEnv(t, "CLARITY_CONFIG_DIR", t.TempDir(), func() {
                        dir := t.TempDir()

                        // Create a legacy db.json by writing the wire format directly.
                        now := time.Now().UTC()
                        legacy := &DB{
                                Version:          1,
                                CurrentActorID:   "act-a",
                                CurrentProjectID: "proj-a",
                                NextIDs:          map[string]int{},
                                Actors:           []model.Actor{{ID: "act-a", Kind: model.ActorKindHuman, Name: "A"}},
                                Projects:         []model.Project{{ID: "proj-a", Name: "P", CreatedBy: "act-a", CreatedAt: now}},
                                Outlines:         []model.Outline{{ID: "out-a", ProjectID: "proj-a", StatusDefs: DefaultOutlineStatusDefs(), CreatedBy: "act-a", CreatedAt: now}},
                                Items:            []model.Item{{ID: "item-a", ProjectID: "proj-a", OutlineID: "out-a", Rank: "h", Title: "T", StatusID: "todo", OwnerActorID: "act-a", CreatedBy: "act-a", CreatedAt: now, UpdatedAt: now}},
                        }
                        b, err := json.MarshalIndent(legacy, "", "  ")
                        if err != nil {
                                t.Fatalf("marshal legacy: %v", err)
                        }
                        if err := os.WriteFile(filepathJoin(dir, "db.json"), b, 0o644); err != nil {
                                t.Fatalf("write db.json: %v", err)
                        }
                        if _, err := os.Stat(filepathJoin(dir, "db.json")); err != nil {
                                t.Fatalf("expected db.json: %v", err)
                        }

                        // Now load (SQLite-only); should import.
                        s2 := Store{Dir: dir}
                        got, err := s2.Load()
                        if err != nil {
                                t.Fatalf("load sqlite (import): %v", err)
                        }
                        if len(got.Items) != 1 || got.Items[0].ID != "item-a" {
                                t.Fatalf("unexpected imported items: %+v", got.Items)
                        }
                })
        })
}

func filepathJoin(a, b string) string { return a + string(os.PathSeparator) + b }
