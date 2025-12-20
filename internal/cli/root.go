package cli

import (
        "errors"
        "fmt"
        "os"
        "strings"

        "clarity-cli/internal/format"
        "clarity-cli/internal/store"
        "clarity-cli/internal/tui"

        "github.com/spf13/cobra"
)

type App struct {
        Dir        string
        Workspace  string
        ActorID    string
        PrettyJSON bool
        Format     string
}

func NewRootCmd() *cobra.Command {
        app := &App{}

        cmd := &cobra.Command{
                Use:          "clarity",
                Short:        "Clarity (local-first) CLI + TUI",
                SilenceUsage: true,
                Example: strings.TrimSpace(`
  # Start the interactive TUI
  clarity

  # Scriptable commands
  clarity items list

  # Find the next thing to work on
  clarity items ready

  # Direct item lookup (shortcut for: clarity items show <item-id>)
  clarity item-abc123
`),
                RunE: func(cmd *cobra.Command, args []string) error {
                        // No subcommand => interactive TUI.
                        if cmd.HasSubCommands() && len(args) == 0 {
                                return runTUI(app)
                        }
                        return cmd.Help()
                },
        }

        cmd.PersistentFlags().StringVar(&app.Dir, "dir", envOr("CLARITY_DIR", ""), "Path to store dir (overrides workspace resolution)")
        cmd.PersistentFlags().StringVar(&app.Workspace, "workspace", envOr("CLARITY_WORKSPACE", ""), "Workspace name (stored under ~/.clarity/workspaces/<name>)")
        cmd.PersistentFlags().StringVar(&app.ActorID, "actor", envOr("CLARITY_ACTOR", ""), "Actor id (overrides currentActorId in db.json)")
        cmd.PersistentFlags().BoolVar(&app.PrettyJSON, "pretty", false, "Pretty-print JSON output")
        cmd.PersistentFlags().StringVar(&app.Format, "format", envOr("CLARITY_FORMAT", "json"), "Output format (json|edn)")

        cmd.AddCommand(newInitCmd(app))
        cmd.AddCommand(newDocsCmd(app))
        cmd.AddCommand(newStatusCmd(app))
        cmd.AddCommand(newWorkspaceCmd(app))
        cmd.AddCommand(newIdentityCmd(app))
        cmd.AddCommand(newProjectsCmd(app))
        cmd.AddCommand(newOutlinesCmd(app))
        cmd.AddCommand(newItemsCmd(app))
        cmd.AddCommand(newDepsCmd(app))
        cmd.AddCommand(newCommentsCmd(app))
        cmd.AddCommand(newEventsCmd(app))
        cmd.AddCommand(newWorklogCmd(app))

        return cmd
}

func runTUI(app *App) error {
        st, _, err := loadDB(app)
        if err != nil {
                return err
        }
        return tui.Run(app.Dir, st)
}

func loadDB(app *App) (*store.DB, store.Store, error) {
        dir := app.Dir
        if dir == "" {
                // Workspace-first:
                // 1) --workspace
                // 2) ~/.clarity/config.json currentWorkspace
                // 3) default workspace ("default")
                // 4) project-local discovery fallback (legacy)
                if app.Workspace != "" {
                        d, err := store.WorkspaceDir(app.Workspace)
                        if err != nil {
                                return nil, store.Store{}, err
                        }
                        dir = d
                } else if cfg, err := store.LoadConfig(); err == nil && cfg.CurrentWorkspace != "" {
                        d, err := store.WorkspaceDir(cfg.CurrentWorkspace)
                        if err != nil {
                                return nil, store.Store{}, err
                        }
                        app.Workspace = cfg.CurrentWorkspace
                        dir = d
                } else {
                        // Create/use the implicit default workspace.
                        app.Workspace = "default"
                        d, err := store.WorkspaceDir(app.Workspace)
                        if err != nil {
                                return nil, store.Store{}, err
                        }
                        dir = d
                }
                app.Dir = dir
        }

        s := store.Store{Dir: dir}
        db, err := s.Load()
        if err != nil {
                return nil, s, err
        }
        return db, s, nil
}

func currentActorID(app *App, db *store.DB) (string, error) {
        if app.ActorID != "" {
                return app.ActorID, nil
        }
        if db.CurrentActorID != "" {
                return db.CurrentActorID, nil
        }
        return "", errors.New("no current actor; run `clarity identity create ... --use` or `clarity identity use <actor-id>` (or pass --actor)")
}

func envOr(k, d string) string {
        if v := os.Getenv(k); v != "" {
                return v
        }
        return d
}

func writeOut(cmd *cobra.Command, app *App, v any) error {
        return format.Write(cmd.OutOrStdout(), v, app.Format, app.PrettyJSON)
}

func writeErr(cmd *cobra.Command, err error) error {
        fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
        return err
}
