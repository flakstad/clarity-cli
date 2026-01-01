package cli

import (
        "errors"
        "fmt"
        "net/http"
        "strings"
        "time"

        "clarity-cli/internal/store"
        "clarity-cli/internal/web"

        "github.com/spf13/cobra"
)

func newWebCmd(app *App) *cobra.Command {
        var addr string
        var readOnly bool
        var authMode string
        var componentsDir string

        cmd := &cobra.Command{
                Use:   "web",
                Short: "Run a local web UI server for the current workspace (v1, experimental)",
                Long: strings.TrimSpace(`
Run a self-hosted web UI for Clarity.

V1 notes:
- No hosted SaaS; the server runs on your machine.
- This is experimental and currently prioritizes read-only views.
`),
                Example: strings.TrimSpace(`
# Serve the current workspace on localhost
clarity web --addr 127.0.0.1:3333

# Serve a specific workspace
clarity --workspace "Flakstad Software" web --addr :3333
`),
                RunE: func(cmd *cobra.Command, args []string) error {
                        dir, err := resolveDir(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        // This is a long-running command. Write a minimal "started" envelope for scripts,
                        // and log the friendly URL to stderr.
                        actorID := strings.TrimSpace(app.ActorID)
                        if actorID == "" {
                                if db, err := (store.Store{Dir: dir}).Load(); err == nil && db != nil {
                                        actorID = strings.TrimSpace(db.CurrentActorID)
                                }
                        }
                        srv, err := web.NewServer(web.ServerConfig{
                                Addr:          strings.TrimSpace(addr),
                                Dir:           dir,
                                Workspace:     strings.TrimSpace(app.Workspace),
                                ActorID:       actorID,
                                ReadOnly:      readOnly,
                                AuthMode:      strings.TrimSpace(authMode),
                                ComponentsDir: strings.TrimSpace(componentsDir),
                        })
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        listenAddr := srv.Addr()
                        if listenAddr == "" {
                                return writeErr(cmd, errors.New("web: missing --addr"))
                        }

                        _ = writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "addr":      listenAddr,
                                        "workspace": strings.TrimSpace(app.Workspace),
                                        "dir":       dir,
                                        "actor":     actorID,
                                        "readOnly":  readOnly,
                                        "auth":      strings.TrimSpace(authMode),
                                        "startedAt": time.Now().UTC().Format(time.RFC3339Nano),
                                },
                                "_hints": []string{
                                        "open http://" + listenAddr,
                                        "clarity sync status",
                                },
                        })

                        fmt.Fprintf(cmd.ErrOrStderr(), "Clarity web running at http://%s (workspace=%s)\n", listenAddr, strings.TrimSpace(app.Workspace))
                        return http.ListenAndServe(listenAddr, srv.Handler())
                },
        }

        cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:3333", "Bind address (host:port or :port)")
        cmd.Flags().BoolVar(&readOnly, "read-only", true, "Disable mutating operations (recommended for v1)")
        cmd.Flags().StringVar(&authMode, "auth", "none", "Auth mode: none|dev|magic (dev mode = actor picker; magic = email link via meta/users.json)")
        cmd.Flags().StringVar(&componentsDir, "components-dir", envOr("CLARITY_COMPONENTS_DIR", ""), "Path to a local `clarity-components` checkout (serves outline.js)")

        return cmd
}
