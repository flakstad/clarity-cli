package cli

import (
        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newDoctorCmd(app *App) *cobra.Command {
        var fail bool

        cmd := &cobra.Command{
                Use:   "doctor",
                Short: "Validate workspace event logs and invariants",
                RunE: func(cmd *cobra.Command, args []string) error {
                        dir, err := resolveDir(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        report := store.DoctorEventsV1(dir)

                        meta := map[string]any{
                                "issues":    len(report.Issues),
                                "hasErrors": report.HasErrors(),
                        }
                        hints := []string{
                                "clarity status",
                        }

                        if err := writeOut(cmd, app, map[string]any{
                                "data":   report,
                                "meta":   meta,
                                "_hints": hints,
                        }); err != nil {
                                return err
                        }

                        if fail && report.HasErrors() {
                                return store.ErrDoctorIssuesFound
                        }
                        return nil
                },
        }

        cmd.Flags().BoolVar(&fail, "fail", false, "Exit with non-zero status if errors are found")
        return cmd
}

func resolveDir(app *App) (string, error) {
        if app.Dir != "" {
                return app.Dir, nil
        }

        // Workspace-first:
        // 1) --workspace
        // 2) ~/.clarity/config.json currentWorkspace
        // 3) default workspace ("default")
        // 4) project-local discovery fallback (legacy)
        if app.Workspace != "" {
                d, err := store.WorkspaceDir(app.Workspace)
                if err != nil {
                        return "", err
                }
                app.Dir = d
                return d, nil
        }
        if cfg, err := store.LoadConfig(); err == nil && cfg.CurrentWorkspace != "" {
                d, err := store.WorkspaceDir(cfg.CurrentWorkspace)
                if err != nil {
                        return "", err
                }
                app.Workspace = cfg.CurrentWorkspace
                app.Dir = d
                return d, nil
        }

        app.Workspace = "default"
        d, err := store.WorkspaceDir(app.Workspace)
        if err != nil {
                return "", err
        }
        app.Dir = d
        return d, nil
}
