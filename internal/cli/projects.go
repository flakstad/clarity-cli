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
        return cmd
}

func newProjectsCreateCmd(app *App) *cobra.Command {
        var name string

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
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        _ = s.AppendEvent(actorID, "project.create", p.ID, p)
                        return writeOut(cmd, app, map[string]any{"data": p})
                },
        }

        cmd.Flags().StringVar(&name, "name", "", "Project name")
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
