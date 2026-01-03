package cli

import (
        "errors"
        "fmt"
        "net/http"
        "strings"
        "time"

        "clarity-cli/internal/store"
        "clarity-cli/internal/webtui"

        "github.com/spf13/cobra"
)

func newWebTUICmd(app *App) *cobra.Command {
        var addr string

        cmd := &cobra.Command{
                Use:   "webtui",
                Short: "Run the Bubble Tea TUI in your browser (PTY + WebSocket demo, experimental)",
                Long: strings.TrimSpace(`
Run the existing Bubble Tea TUI over the web via a server-side PTY and a browser terminal emulator.

Notes:
- Experimental demo mode (no auth yet).
- Each browser tab starts a TUI subprocess on the server.
`),
                Example: strings.TrimSpace(`
# Serve the current workspace on localhost
clarity webtui --addr 127.0.0.1:3334

# Serve a specific workspace
clarity --workspace "Flakstad Software" webtui --addr :3334
`),
                RunE: func(cmd *cobra.Command, args []string) error {
                        dir, err := resolveDir(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        actorID := strings.TrimSpace(app.ActorID)
                        if actorID == "" {
                                if db, err := (store.Store{Dir: dir}).Load(); err == nil && db != nil {
                                        actorID = strings.TrimSpace(db.CurrentActorID)
                                }
                        }

                        srv, err := webtui.NewServer(webtui.ServerConfig{
                                Addr:      strings.TrimSpace(addr),
                                Dir:       dir,
                                Workspace: strings.TrimSpace(app.Workspace),
                                ActorID:   actorID,
                        })
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        listenAddr := srv.Addr()
                        if listenAddr == "" {
                                return writeErr(cmd, errors.New("webtui: missing --addr"))
                        }

                        _ = writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "addr":      listenAddr,
                                        "workspace": strings.TrimSpace(app.Workspace),
                                        "dir":       dir,
                                        "actor":     actorID,
                                        "startedAt": time.Now().UTC().Format(time.RFC3339Nano),
                                },
                                "_hints": []string{
                                        "open http://" + listenAddr,
                                },
                        })

                        fmt.Fprintf(cmd.ErrOrStderr(), "Clarity webtui running at http://%s (workspace=%s)\n", listenAddr, strings.TrimSpace(app.Workspace))
                        return http.ListenAndServe(listenAddr, srv.Handler())
                },
        }

        cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:3334", "Bind address (host:port or :port)")
        return cmd
}
