package cli

import (
        "errors"
        "fmt"
        "os/exec"
        "path/filepath"
        "runtime"
        "strings"

        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newAttachmentsCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "attachments",
                Short: "Manage file attachments",
        }

        cmd.AddCommand(newAttachmentsAddCmd(app))
        cmd.AddCommand(newAttachmentsListCmd(app))
        cmd.AddCommand(newAttachmentsOpenCmd(app))
        cmd.AddCommand(newAttachmentsExportCmd(app))

        return cmd
}

func newAttachmentsAddCmd(app *App) *cobra.Command {
        var title string
        var alt string
        var maxMB int64
        var kind string

        cmd := &cobra.Command{
                Use:   "add <entity-id> <path>",
                Short: "Attach a local file to an item or comment",
                Args:  cobra.ExactArgs(2),
                RunE: func(cmd *cobra.Command, args []string) error {
                        entityID := strings.TrimSpace(args[0])
                        path := strings.TrimSpace(args[1])
                        if entityID == "" || path == "" {
                                return errors.New("missing entity id/path")
                        }
                        if kind == "" {
                                if strings.HasPrefix(entityID, "cmt-") {
                                        kind = "comment"
                                } else {
                                        kind = "item"
                                }
                        }

                        db, st, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        actorID, err := currentActorID(app, db)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        var maxBytes int64 = store.DefaultAttachmentMaxBytes
                        if maxMB > 0 {
                                maxBytes = maxMB * 1024 * 1024
                        }

                        // Validate entity existence.
                        switch strings.ToLower(strings.TrimSpace(kind)) {
                        case "item":
                                if _, ok := db.FindItem(entityID); !ok {
                                        return fmt.Errorf("item not found: %s", entityID)
                                }
                        case "comment":
                                found := false
                                for i := range db.Comments {
                                        if strings.TrimSpace(db.Comments[i].ID) == entityID {
                                                found = true
                                                break
                                        }
                                }
                                if !found {
                                        return fmt.Errorf("comment not found: %s", entityID)
                                }
                        default:
                                return fmt.Errorf("invalid --kind: %q (expected item|comment)", kind)
                        }

                        a, err := st.AddAttachment(db, actorID, kind, entityID, path, title, alt, maxBytes)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := st.AppendEvent(actorID, "attachment.add", a.ID, a); err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := st.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": a,
                                "_hints": []string{
                                        "clarity attachments open " + a.ID,
                                        "clarity attachments export " + a.ID + " <dest-path>",
                                },
                        })
                },
        }

        cmd.Flags().StringVar(&kind, "kind", "", "Entity kind (item|comment); default inferred from id prefix")
        cmd.Flags().StringVar(&title, "title", "", "Attachment title (optional)")
        cmd.Flags().StringVar(&alt, "alt", "", "Attachment description/alt text (optional)")
        cmd.Flags().Int64Var(&maxMB, "max-mb", 50, "Max file size in MB")

        return cmd
}

func newAttachmentsListCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "list <entity-id>",
                Short: "List attachments for an item or comment",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        entityID := strings.TrimSpace(args[0])
                        if entityID == "" {
                                return errors.New("missing entity id")
                        }
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        var kind string
                        if strings.HasPrefix(entityID, "cmt-") {
                                kind = "comment"
                        } else {
                                kind = "item"
                        }
                        var out any
                        if kind == "comment" {
                                out = db.AttachmentsForComment(entityID)
                        } else {
                                out = db.AttachmentsForItem(entityID)
                        }
                        return writeOut(cmd, app, map[string]any{"data": out})
                },
        }
        return cmd
}

func newAttachmentsOpenCmd(app *App) *cobra.Command {
        var printPath bool

        cmd := &cobra.Command{
                Use:   "open <attachment-id>",
                Short: "Open an attachment using the OS default handler",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        id := strings.TrimSpace(args[0])
                        if id == "" {
                                return errors.New("missing attachment id")
                        }
                        db, st, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        a, ok := db.FindAttachment(id)
                        if !ok || a == nil {
                                return writeErr(cmd, fmt.Errorf("attachment not found: %s", id))
                        }
                        p := st.AttachmentAbsPath(*a)
                        if printPath {
                                fmt.Fprintln(cmd.OutOrStdout(), p)
                                return nil
                        }
                        return writeErr(cmd, openPath(p))
                },
        }
        cmd.Flags().BoolVar(&printPath, "print-path", false, "Print the attachment path instead of opening it")
        return cmd
}

func newAttachmentsExportCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "export <attachment-id> <dest-path>",
                Short: "Copy an attachment to a destination path",
                Args:  cobra.ExactArgs(2),
                RunE: func(cmd *cobra.Command, args []string) error {
                        id := strings.TrimSpace(args[0])
                        dest := filepath.Clean(strings.TrimSpace(args[1]))
                        if id == "" || dest == "" {
                                return errors.New("missing attachment id/destination")
                        }
                        db, st, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        a, ok := db.FindAttachment(id)
                        if !ok || a == nil {
                                return writeErr(cmd, fmt.Errorf("attachment not found: %s", id))
                        }
                        src := st.AttachmentAbsPath(*a)
                        if err := store.CopyFile(src, dest); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{"exportedTo": dest},
                        })
                },
        }
        return cmd
}

func openPath(path string) error {
        path = strings.TrimSpace(path)
        if path == "" {
                return errors.New("empty path")
        }
        switch runtime.GOOS {
        case "darwin":
                return exec.Command("open", path).Run()
        case "windows":
                return exec.Command("cmd", "/c", "start", "", path).Run()
        default:
                return exec.Command("xdg-open", path).Run()
        }
}
