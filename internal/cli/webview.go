//go:build webview

package cli

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"clarity-cli/internal/web"

	"github.com/spf13/cobra"
	webview "github.com/webview/webview_go"
)

func newWebViewCmd(app *App) *cobra.Command {
	var addr string
	var title string
	var width int
	var height int
	var debug bool

	cmd := &cobra.Command{
		Use:   "webview",
		Short: "Open the minimal HTML UI in a native webview window (experimental)",
		Long: strings.TrimSpace(`
Open the experimental minimal HTML UI in a native webview window.

Notes:
- This command is build-tagged and requires: -tags webview
- It starts a local HTTP server and points the webview at it.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveDir(app)
			if err != nil {
				return writeErr(cmd, err)
			}

			listenAddr := strings.TrimSpace(addr)
			if listenAddr == "" {
				return writeErr(cmd, errors.New("webview: missing --addr"))
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
			defer ln.Close()

			actualAddr := ln.Addr().String()
			url := "http://" + actualAddr + "/"

			go func() { _ = http.Serve(ln, srv.Handler()) }()

			w := webview.New(debug)
			defer w.Destroy()
			w.SetTitle(strings.TrimSpace(title))
			w.SetSize(width, height, webview.HintNone)
			w.Navigate(url)
			fmt.Fprintf(cmd.ErrOrStderr(), "Clarity webview running at %s (workspace=%s)\n", url, strings.TrimSpace(app.Workspace))
			w.Run()
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:0", "Bind address for the local server (host:port or :port)")
	cmd.Flags().StringVar(&title, "title", "Clarity", "Window title")
	cmd.Flags().IntVar(&width, "width", 1100, "Window width (pixels)")
	cmd.Flags().IntVar(&height, "height", 750, "Window height (pixels)")
	cmd.Flags().BoolVar(&debug, "debug", false, "Enable webview debug mode")
	return cmd
}
