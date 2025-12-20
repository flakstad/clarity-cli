package cli

import (
        "errors"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newOutlinesCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "outlines",
                Short: "Outline commands (containers for items)",
        }
        cmd.AddCommand(newOutlinesCreateCmd(app))
        cmd.AddCommand(newOutlinesListCmd(app))
        cmd.AddCommand(newOutlinesShowCmd(app))
        cmd.AddCommand(newOutlinesStatusCmd(app))
        return cmd
}

func newOutlinesCreateCmd(app *App) *cobra.Command {
        var projectID string
        var name string

        cmd := &cobra.Command{
                Use:   "create",
                Short: "Create an outline in a project",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        actorID, err := currentActorID(app, db)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        pid := strings.TrimSpace(projectID)
                        if pid == "" {
                                pid = strings.TrimSpace(db.CurrentProjectID)
                                if pid == "" {
                                        return writeErr(cmd, errors.New("missing --project (or set a current project with `clarity projects use <project-id>`)"))
                                }
                        }
                        if _, ok := db.FindProject(pid); !ok {
                                return writeErr(cmd, errNotFound("project", pid))
                        }
                        var namePtr *string
                        n := strings.TrimSpace(name)
                        if n != "" {
                                namePtr = &n
                        }
                        o := model.Outline{
                                ID:         s.NextID(db, "out"),
                                ProjectID:  pid,
                                Name:       namePtr,
                                StatusDefs: store.DefaultOutlineStatusDefs(),
                                CreatedBy:  actorID,
                                CreatedAt:  time.Now().UTC(),
                        }
                        db.Outlines = append(db.Outlines, o)
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "outline.create", o.ID, o)
                        return writeOut(cmd, app, map[string]any{"data": o})
                },
        }

        cmd.Flags().StringVar(&projectID, "project", "", "Project id (optional if a current project is set)")
        cmd.Flags().StringVar(&name, "name", "", "Optional outline name")
        return cmd
}

func newOutlinesListCmd(app *App) *cobra.Command {
        var projectID string
        cmd := &cobra.Command{
                Use:   "list",
                Short: "List outlines",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if projectID == "" {
                                out := db.Outlines
                                if out == nil {
                                        out = []model.Outline{}
                                }
                                return writeOut(cmd, app, map[string]any{"data": out})
                        }
                        out := make([]model.Outline, 0)
                        for _, o := range db.Outlines {
                                if o.ProjectID == projectID {
                                        out = append(out, o)
                                }
                        }
                        return writeOut(cmd, app, map[string]any{"data": out})
                },
        }
        cmd.Flags().StringVar(&projectID, "project", "", "Project id (optional)")
        return cmd
}

func newOutlinesShowCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "show <outline-id>",
                Short: "Show an outline",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        id := args[0]
                        o, ok := db.FindOutline(id)
                        if !ok {
                                return writeErr(cmd, errNotFound("outline", id))
                        }
                        return writeOut(cmd, app, map[string]any{"data": o})
                },
        }
        return cmd
}
