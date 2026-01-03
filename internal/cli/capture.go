package cli

import (
        "errors"
        "fmt"
        "strings"

        "clarity-cli/internal/store"
        "clarity-cli/internal/tui"

        "github.com/spf13/cobra"
)

func newCaptureCmd(app *App) *cobra.Command {
        var noOutput bool
        var exit0OnCancel bool
        var hotkey bool

        cmd := &cobra.Command{
                Use:           "capture",
                Short:         "Org-capture style quick capture (TUI)",
                SilenceUsage:  true,
                SilenceErrors: true, // cancel should be quiet (non-zero exit)
                RunE: func(cmd *cobra.Command, args []string) error {
                        if hotkey {
                                noOutput = true
                                exit0OnCancel = true
                        }

                        cfg, err := store.LoadConfig()
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := store.ValidateCaptureTemplates(cfg); err != nil {
                                return writeErr(cmd, err)
                        }
                        if len(cfg.CaptureTemplates) == 0 {
                                return writeErr(cmd, errors.New("no capture templates configured (add `captureTemplates` to ~/.clarity/config.json)"))
                        }

                        res, err := tui.RunCapture(cfg, app.ActorID)
                        if err != nil {
                                if errors.Is(err, tui.ErrCaptureCanceled) {
                                        if exit0OnCancel {
                                                return nil
                                        }
                                        return err
                                }
                                return writeErr(cmd, err)
                        }
                        if noOutput {
                                return nil
                        }

                        // Load the created item so we can return a stable JSON envelope.
                        dir := app.Dir
                        ws := strings.TrimSpace(res.Workspace)
                        if ws != "" {
                                dir, err = store.WorkspaceDir(ws)
                                if err != nil {
                                        return writeErr(cmd, err)
                                }
                        } else if strings.TrimSpace(res.Dir) != "" {
                                dir = strings.TrimSpace(res.Dir)
                        }

                        s := store.Store{Dir: dir}
                        db, err := s.Load()
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        it, ok := db.FindItem(res.ItemID)
                        if !ok || it == nil {
                                return writeErr(cmd, fmt.Errorf("created item not found: %s", res.ItemID))
                        }

                        ref := res.ItemID
                        if ws != "" {
                                ref = ref + " --workspace " + fmt.Sprintf("%q", ws)
                        }
                        hints := []string{
                                "clarity items show " + ref,
                        }
                        return writeOut(cmd, app, map[string]any{"data": it, "_hints": hints})
                },
        }

        cmd.Flags().BoolVar(&hotkey, "hotkey", false, "Hotkey mode (implies --no-output and --exit-0-on-cancel)")
        cmd.Flags().BoolVar(&noOutput, "no-output", false, "Do not print JSON output (useful for hotkey capture windows)")
        cmd.Flags().BoolVar(&exit0OnCancel, "exit-0-on-cancel", false, "Exit 0 when capture is canceled (useful for hotkey capture windows)")
        return cmd
}
