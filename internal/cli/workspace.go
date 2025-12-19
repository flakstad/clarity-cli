package cli

import (
        "os"

        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newWorkspaceCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "workspace",
                Short: "Workspace management (workspace-first)",
        }

        cmd.AddCommand(newWorkspaceInitCmd(app))
        cmd.AddCommand(newWorkspaceUseCmd(app))
        cmd.AddCommand(newWorkspaceCurrentCmd(app))
        cmd.AddCommand(newWorkspaceRenameCmd(app))

        return cmd
}

func newWorkspaceInitCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "init <name>",
                Short: "Create a workspace and set it as current",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        name, err := store.NormalizeWorkspaceName(args[0])
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        dir, err := store.WorkspaceDir(name)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        s := store.Store{Dir: dir}
                        if err := s.Ensure(); err != nil {
                                return writeErr(cmd, err)
                        }
                        db, err := s.Load()
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }

                        cfg, err := store.LoadConfig()
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        cfg.CurrentWorkspace = name
                        if err := store.SaveConfig(cfg); err != nil {
                                return writeErr(cmd, err)
                        }

                        app.Workspace = name
                        app.Dir = dir
                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "workspace": name,
                                        "dir":       dir,
                                },
                        })
                },
        }
        return cmd
}

func newWorkspaceUseCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "use <name>",
                Short: "Set current workspace",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        name, err := store.NormalizeWorkspaceName(args[0])
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        dir, err := store.WorkspaceDir(name)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        s := store.Store{Dir: dir}
                        if err := s.Ensure(); err != nil {
                                return writeErr(cmd, err)
                        }

                        cfg, err := store.LoadConfig()
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        cfg.CurrentWorkspace = name
                        if err := store.SaveConfig(cfg); err != nil {
                                return writeErr(cmd, err)
                        }

                        app.Workspace = name
                        app.Dir = dir
                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "workspace": name,
                                        "dir":       dir,
                                },
                        })
                },
        }
        return cmd
}

func newWorkspaceCurrentCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "current",
                Short: "Show current workspace",
                RunE: func(cmd *cobra.Command, args []string) error {
                        cfg, err := store.LoadConfig()
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if cfg.CurrentWorkspace == "" {
                                cfg.CurrentWorkspace = "default"
                        }
                        var dir string
                        if cfg.CurrentWorkspace != "" {
                                d, err := store.WorkspaceDir(cfg.CurrentWorkspace)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                dir = d
                        }
                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "workspace": cfg.CurrentWorkspace,
                                        "dir":       dir,
                                },
                        })
                },
        }
        return cmd
}

func newWorkspaceRenameCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "rename <old> <new>",
                Short: "Rename a workspace (also updates currentWorkspace if needed)",
                Args:  cobra.ExactArgs(2),
                RunE: func(cmd *cobra.Command, args []string) error {
                        oldName, err := store.NormalizeWorkspaceName(args[0])
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        newName, err := store.NormalizeWorkspaceName(args[1])
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        oldDir, err := store.WorkspaceDir(oldName)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        newDir, err := store.WorkspaceDir(newName)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := os.Rename(oldDir, newDir); err != nil {
                                return writeErr(cmd, err)
                        }

                        cfg, err := store.LoadConfig()
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if cfg.CurrentWorkspace == "" {
                                cfg.CurrentWorkspace = "default"
                        }
                        if cfg.CurrentWorkspace == oldName {
                                cfg.CurrentWorkspace = newName
                                if err := store.SaveConfig(cfg); err != nil {
                                        return writeErr(cmd, err)
                                }
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "from": oldName,
                                        "to":   newName,
                                },
                        })
                },
        }
        return cmd
}
