//go:build !webview

package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newWebViewCmd(app *App) *cobra.Command {
	var addr string

	cmd := &cobra.Command{
		Use:    "webview",
		Short:  "Open the minimal HTML UI in a native webview window (requires -tags webview)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = addr
			return writeErr(cmd, errors.New("webview support is not built in; re-run with: go run -tags webview ./cmd/clarity webview"))
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:0", "Bind address for the local server (host:port or :port)")
	return cmd
}
