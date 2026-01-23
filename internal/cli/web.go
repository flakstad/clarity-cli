package cli

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"clarity-cli/internal/web"

	"github.com/spf13/cobra"
)

func newWebCmd(app *App) *cobra.Command {
	var addr string
	var open bool

	cmd := &cobra.Command{
		Use:   "web",
		Short: "Run a simple HTML UI (experimental, no JS)",
		Long: strings.TrimSpace(`
Run a simple HTML UI served from a local HTTP server.

This is an experimental alternative to ` + "`clarity webtui`" + ` and is intentionally minimal:
- Just server-rendered HTML + CSS (no JavaScript)
- Currently: project list for the selected workspace
`),
		Example: strings.TrimSpace(`
# Serve the current workspace on localhost
clarity web --addr 127.0.0.1:3335

# Serve a specific workspace
clarity --workspace "Flakstad Software" web --addr :3335
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveDir(app)
			if err != nil {
				return writeErr(cmd, err)
			}

			listenAddr := strings.TrimSpace(addr)
			if listenAddr == "" {
				return writeErr(cmd, errors.New("web: missing --addr"))
			}

			srv, err := web.NewServer(web.ServerConfig{
				Dir:       dir,
				Workspace: strings.TrimSpace(app.Workspace),
			})
			if err != nil {
				return writeErr(cmd, err)
			}

			ln, err := net.Listen("tcp", listenAddr)
			if err != nil {
				return writeErr(cmd, err)
			}

			actualAddr := ln.Addr().String()
			url := "http://" + actualAddr + "/"

			opened := false
			openErr := ""
			if open {
				if err := openPath(url); err != nil {
					openErr = err.Error()
				} else {
					opened = true
				}
			}

			hints := []string{}
			if !opened {
				hints = append(hints, "open "+url)
			}

			_ = writeOut(cmd, app, map[string]any{
				"data": map[string]any{
					"addr":      actualAddr,
					"url":       url,
					"workspace": strings.TrimSpace(app.Workspace),
					"dir":       dir,
					"opened":    opened,
					"openError": openErr,
					"startedAt": time.Now().UTC().Format(time.RFC3339Nano),
				},
				"_hints": hints,
			})

			fmt.Fprintf(cmd.ErrOrStderr(), "Clarity web running at %s (workspace=%s)\n", url, strings.TrimSpace(app.Workspace))
			if openErr != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "Failed to open browser: %s\n", openErr)
			}

			return http.Serve(ln, srv.Handler())
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:3335", "Bind address (host:port or :port)")
	cmd.Flags().BoolVar(&open, "open", true, "Open the UI in your default browser")
	return cmd
}
