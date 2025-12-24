package tui

import (
        "fmt"
        "strings"
        "testing"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/bubbles/list"
)

func TestListBodyWithOverflowHint_ShowsMoreIndicator(t *testing.T) {
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
        m.width = 80
        m.height = 18

        // Seed outlines list with enough rows to overflow a small body height.
        items := make([]list.Item, 0, 40)
        for i := 0; i < 40; i++ {
                oid := fmt.Sprintf("out-%02d", i)
                o := model.Outline{
                        ID:         oid,
                        ProjectID:  "proj-a",
                        StatusDefs: store.DefaultOutlineStatusDefs(),
                        CreatedBy:  "act-test",
                        CreatedAt:  time.Now().UTC(),
                }
                items = append(items, outlineItem{outline: o})
        }
        m.outlinesList.SetItems(items)

        out := m.viewOutlines()
        plain := stripANSIEscapes(out)
        if !strings.Contains(plain, "↓ more") {
                t.Fatalf("expected overflow hint to be shown, got:\n%s", plain)
        }
}
