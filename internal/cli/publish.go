package cli

import (
        "errors"
        "strings"

        "clarity-cli/internal/publish"

        "github.com/spf13/cobra"
)

func newPublishCmd(app *App) *cobra.Command {
        var toDir string
        var includeArchived bool
        var includeWorklog bool
        var overwrite bool

        cmd := &cobra.Command{
                Use:   "publish",
                Short: "Export derived Markdown artifacts (not canonical)",
        }

        itemCmd := &cobra.Command{
                Use:   "item <item-id>",
                Short: "Publish a single item as Markdown",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        actorID, err := currentActorID(app, db)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        toDir = strings.TrimSpace(toDir)
                        if toDir == "" {
                                return writeErr(cmd, errors.New("missing --to"))
                        }
                        res, err := publish.WriteItem(db, args[0], toDir, publish.WriteOptions{
                                IncludeArchived: includeArchived,
                                IncludeWorklog:  includeWorklog,
                                Overwrite:       overwrite,
                                ActorID:         actorID,
                        })
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{
                                "data": res,
                                "_hints": []string{
                                        "git status",
                                        "git add -A",
                                        "git commit -m \"Publish: " + args[0] + "\"",
                                },
                        })
                },
        }
        outlineCmd := &cobra.Command{
                Use:   "outline <outline-id>",
                Short: "Publish an outline index + item pages as Markdown",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        actorID, err := currentActorID(app, db)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        toDir = strings.TrimSpace(toDir)
                        if toDir == "" {
                                return writeErr(cmd, errors.New("missing --to"))
                        }
                        res, err := publish.WriteOutline(db, args[0], toDir, publish.WriteOptions{
                                IncludeArchived: includeArchived,
                                IncludeWorklog:  includeWorklog,
                                Overwrite:       overwrite,
                                ActorID:         actorID,
                        })
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{
                                "data": res,
                                "_hints": []string{
                                        "git status",
                                        "git add -A",
                                        "git commit -m \"Publish: outline " + args[0] + "\"",
                                },
                        })
                },
        }

        cmd.PersistentFlags().StringVar(&toDir, "to", "", "Output directory")
        _ = cmd.MarkPersistentFlagRequired("to")
        cmd.PersistentFlags().BoolVar(&includeArchived, "include-archived", false, "Include archived items")
        cmd.PersistentFlags().BoolVar(&includeWorklog, "include-worklog", false, "Include your private worklog entries")
        cmd.PersistentFlags().BoolVar(&overwrite, "overwrite", true, "Overwrite existing files")

        cmd.AddCommand(itemCmd)
        cmd.AddCommand(outlineCmd)
        return cmd
}
