package cli

import (
        "encoding/json"
        "errors"
        "os"
        "path/filepath"
        "strings"
        "time"

        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newWorkspaceCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "workspace",
                Short: "Workspace management (default workspace is recommended unless explicitly told otherwise)",
        }

        cmd.AddCommand(newWorkspaceInitCmd(app))
        cmd.AddCommand(newWorkspaceUseCmd(app))
        cmd.AddCommand(newWorkspaceCurrentCmd(app))
        cmd.AddCommand(newWorkspaceListCmd(app))
        cmd.AddCommand(newWorkspaceRenameCmd(app))
        cmd.AddCommand(newWorkspaceExportCmd(app))
        cmd.AddCommand(newWorkspaceImportCmd(app))

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

func newWorkspaceListCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "list",
                Short: "List all workspaces",
                RunE: func(cmd *cobra.Command, args []string) error {
                        cfg, err := store.LoadConfig()
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if cfg.CurrentWorkspace == "" {
                                cfg.CurrentWorkspace = "default"
                        }

                        ws, err := store.ListWorkspaces()
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "workspaces":       ws,
                                        "currentWorkspace": cfg.CurrentWorkspace,
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

func newWorkspaceExportCmd(app *App) *cobra.Command {
        var to string
        var includeEvents bool
        var force bool

        cmd := &cobra.Command{
                Use:   "export",
                Short: "Export a portable backup (state.json + events.jsonl) for offline storage",
                RunE: func(cmd *cobra.Command, args []string) error {
                        to = strings.TrimSpace(to)
                        if to == "" {
                                return writeErr(cmd, errors.New("missing --to (target directory)"))
                        }

                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        if err := os.MkdirAll(to, 0o755); err != nil {
                                return writeErr(cmd, err)
                        }
                        if !force {
                                if ents, err := os.ReadDir(to); err == nil && len(ents) > 0 {
                                        return writeErr(cmd, errors.New("target directory is not empty (pass --force to overwrite)"))
                                }
                        }

                        // state.json (pretty, stable)
                        statePath := filepath.Join(to, "state.json")
                        b, err := json.MarshalIndent(db, "", "  ")
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := os.WriteFile(statePath, b, 0o644); err != nil {
                                return writeErr(cmd, err)
                        }

                        eventsPath := ""
                        evCount := 0
                        if includeEvents {
                                evs, err := s.ReadEventsV1(cmd.Context(), 0)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                                eventsPath = filepath.Join(to, "events.jsonl")
                                if err := store.WriteEventsJSONL(eventsPath, evs); err != nil {
                                        return writeErr(cmd, err)
                                }
                                evCount = len(evs)
                        }

                        // Minimal manifest for humans.
                        manifest := map[string]any{
                                "version":          1,
                                "exportedAtUnixMs": time.Now().UTC().UnixMilli(),
                                "workspace": map[string]any{
                                        "name": app.Workspace,
                                        "dir":  app.Dir,
                                },
                                "files": map[string]any{
                                        "state": "state.json",
                                        "events": func() any {
                                                if includeEvents {
                                                        return "events.jsonl"
                                                }
                                                return nil
                                        }(),
                                },
                        }
                        // Best-effort: avoid writing null for events when disabled.
                        if !includeEvents {
                                if files, ok := manifest["files"].(map[string]any); ok {
                                        delete(files, "events")
                                }
                        }
                        if mb, err := json.MarshalIndent(manifest, "", "  "); err == nil {
                                _ = os.WriteFile(filepath.Join(to, "manifest.json"), mb, 0o644)
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "to":           to,
                                        "statePath":    statePath,
                                        "eventsPath":   eventsPath,
                                        "eventsCount":  evCount,
                                        "workspace":    app.Workspace,
                                        "workspaceDir": app.Dir,
                                },
                        })
                },
        }

        cmd.Flags().StringVar(&to, "to", "", "Target directory to write backup files into")
        cmd.Flags().BoolVar(&includeEvents, "events", true, "Include events.jsonl (recommended; useful for future sync/debugging)")
        cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing files in the target directory")

        return cmd
}

func newWorkspaceImportCmd(app *App) *cobra.Command {
        var from string
        var nameOpt string
        var force bool
        var use bool
        var withEvents bool

        cmd := &cobra.Command{
                Use:   "import [name]",
                Short: "Import a portable backup into a new workspace",
                Args:  cobra.MaximumNArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        // Accept either positional [name] or --name (preferred for scripts).
                        rawName := strings.TrimSpace(nameOpt)
                        if rawName == "" && len(args) > 0 {
                                rawName = args[0]
                        }
                        if rawName == "" {
                                return writeErr(cmd, errors.New("missing workspace name (pass [name] or --name)"))
                        }

                        name, err := store.NormalizeWorkspaceName(rawName)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        from = strings.TrimSpace(from)
                        if from == "" {
                                return writeErr(cmd, errors.New("missing --from (backup directory)"))
                        }

                        dir, err := store.WorkspaceDir(name)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        if st, err := os.Stat(dir); err == nil && st.IsDir() {
                                if !force {
                                        return writeErr(cmd, errors.New("workspace already exists (pass --force to replace it)"))
                                }
                                if err := os.RemoveAll(dir); err != nil {
                                        return writeErr(cmd, err)
                                }
                        }
                        if err := os.MkdirAll(dir, 0o755); err != nil {
                                return writeErr(cmd, err)
                        }

                        // Load state.json
                        statePath := filepath.Join(from, "state.json")
                        sb, err := os.ReadFile(statePath)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        var stDB store.DB
                        if err := json.Unmarshal(sb, &stDB); err != nil {
                                return writeErr(cmd, err)
                        }

                        s := store.Store{Dir: dir}
                        if err := s.Save(&stDB); err != nil {
                                return writeErr(cmd, err)
                        }

                        evCount := 0
                        if withEvents {
                                eventsPath := filepath.Join(from, "events.jsonl")
                                if _, err := os.Stat(eventsPath); err == nil {
                                        evs, err := store.ReadEventsJSONL(eventsPath)
                                        if err != nil {
                                                return writeErr(cmd, err)
                                        }
                                        wsID := ""
                                        if len(evs) > 0 {
                                                wsID = strings.TrimSpace(evs[0].WorkspaceID)
                                        }
                                        if err := s.ReplaceEventsV1(cmd.Context(), wsID, evs); err != nil {
                                                return writeErr(cmd, err)
                                        }
                                        evCount = len(evs)
                                }
                        }

                        if use {
                                cfg, err := store.LoadConfig()
                                if err == nil {
                                        cfg.CurrentWorkspace = name
                                        _ = store.SaveConfig(cfg)
                                }
                                app.Workspace = name
                                app.Dir = dir
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "workspace":   name,
                                        "dir":         dir,
                                        "from":        from,
                                        "eventsCount": evCount,
                                        "used":        use,
                                },
                        })
                },
        }

        cmd.Flags().StringVar(&from, "from", "", "Backup directory containing state.json (and optionally events.jsonl)")
        cmd.Flags().StringVar(&nameOpt, "name", "", "Workspace name for the imported backup (overrides positional [name])")
        cmd.Flags().BoolVar(&withEvents, "events", true, "Import events.jsonl if present")
        cmd.Flags().BoolVar(&force, "force", false, "Replace existing workspace if it already exists")
        cmd.Flags().BoolVar(&use, "use", false, "Set the imported workspace as current")

        return cmd
}
