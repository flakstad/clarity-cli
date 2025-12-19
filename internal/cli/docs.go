package cli

import (
        "sort"
        "fmt"

        "clarity-cli/internal/docs"

        "github.com/spf13/cobra"
)

func newDocsCmd(app *App) *cobra.Command {
        var raw bool

        cmd := &cobra.Command{
                Use:   "docs [topic]",
                Short: "Show on-demand documentation (for humans and agents)",
                Args:  cobra.MaximumNArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        if len(args) == 0 {
                                topics := docs.Topics()
                                sort.Strings(topics)
                                return writeOut(cmd, app, map[string]any{"data": map[string]any{"topics": topics}})
                        }

                        topic := args[0]
                        body, ok := docs.Get(topic)
                        if !ok {
                                return writeErr(cmd, fmt.Errorf("unknown docs topic: %q (run `clarity docs` to list topics)", topic))
                        }

                        if raw {
                                _, err := fmt.Fprint(cmd.OutOrStdout(), body)
                                if err != nil {
                                        return err
                                }
                                return nil
                        }

                        return writeOut(cmd, app, map[string]any{"data": map[string]any{"topic": topic, "markdown": body}})
                },
        }

        cmd.Flags().BoolVar(&raw, "raw", false, "Print raw markdown (no JSON envelope)")

        return cmd
}
