package cli

import (
        "strings"
        "time"

        "clarity-cli/internal/model"

        "github.com/spf13/cobra"
)

func newProjectsCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "projects",
                Short: "Project commands",
        }
        cmd.AddCommand(newProjectsCreateCmd(app))
        cmd.AddCommand(newProjectsListCmd(app))
        cmd.AddCommand(newProjectsUseCmd(app))
        cmd.AddCommand(newProjectsCurrentCmd(app))
        return cmd
}

func newProjectsCreateCmd(app *App) *cobra.Command {
        var name string
        var use bool

        cmd := &cobra.Command{
                Use:   "create",
                Short: "Create a project",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        actorID, err := currentActorID(app, db)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if _, ok := db.FindActor(actorID); !ok {
                                return writeErr(cmd, errNotFound("actor", actorID))
                        }

                        p := model.Project{
                                ID:        s.NextID(db, "proj"),
                                Name:      strings.TrimSpace(name),
                                CreatedBy: actorID,
                                CreatedAt: time.Now().UTC(),
                        }
                        db.Projects = append(db.Projects, p)
                        if use {
                                db.CurrentProjectID = p.ID
                        }
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "project.create", p.ID, p)
                        return writeOut(cmd, app, map[string]any{"data": p})
                },
        }

        cmd.Flags().StringVar(&name, "name", "", "Project name")
        cmd.Flags().BoolVar(&use, "use", false, "Set as current project")
        _ = cmd.MarkFlagRequired("name")
        return cmd
}

func newProjectsListCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "list",
                Short: "List projects",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{"data": db.Projects})
                },
        }
        return cmd
}

func newProjectsUseCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "use <project-id>",
                Short: "Set current project (workspace-scoped)",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        pid := strings.TrimSpace(args[0])
                        p, ok := db.FindProject(pid)
                        if !ok {
                                return writeErr(cmd, errNotFound("project", pid))
                        }
                        db.CurrentProjectID = pid
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "projectId": pid,
                                        "project":   p,
                                },
                        })
                },
        }
        return cmd
}

func newProjectsCurrentCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "current",
                Short: "Show current project (workspace-scoped)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        pid := strings.TrimSpace(db.CurrentProjectID)
                        var p *model.Project
                        if pid != "" {
                                if pp, ok := db.FindProject(pid); ok {
                                        p = pp
                                }
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "projectId": pid,
                                        "project":   p,
                                },
                        })
                },
        }
        return cmd
}
