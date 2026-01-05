package tui

import (
        "errors"
        "fmt"
        "io"
        "os"
        "os/exec"
        "path/filepath"
        "runtime"
        "strings"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/charmbracelet/bubbles/filepicker"
        "github.com/charmbracelet/bubbles/key"
        tea "github.com/charmbracelet/bubbletea"
        "github.com/charmbracelet/lipgloss"
)

type attachmentOpenDoneMsg struct {
        err error
}

type urlOpenDoneMsg struct {
        err error
}

type attachmentDraft struct {
        Path  string
        Title string
        Alt   string
}

func attachmentFilePickerHeight(screenH int) int {
        // Conservative sizing: leave room for the modal title, borders, and help line.
        h := screenH - 16
        if h < 8 {
                h = 8
        }
        if h > 18 {
                h = 18
        }
        return h
}

func (m *appModel) openAttachment(a model.Attachment) tea.Cmd {
        if m == nil {
                return nil
        }
        p := m.store.AttachmentAbsPath(a)
        if strings.TrimSpace(p) == "" {
                return func() tea.Msg { return attachmentOpenDoneMsg{err: errors.New("empty attachment path")} }
        }

        return func() tea.Msg {
                var cmd *exec.Cmd
                switch runtime.GOOS {
                case "darwin":
                        cmd = exec.Command("open", p)
                case "windows":
                        cmd = exec.Command("cmd", "/c", "start", "", p)
                default:
                        cmd = exec.Command("xdg-open", p)
                }
                // Prevent any output from flashing in the terminal.
                cmd.Stdout = io.Discard
                cmd.Stderr = io.Discard
                if err := cmd.Start(); err != nil {
                        return attachmentOpenDoneMsg{err: err}
                }
                return attachmentOpenDoneMsg{err: cmd.Wait()}
        }
}

func (m *appModel) openURL(u string) tea.Cmd {
        if m == nil {
                return nil
        }
        u = strings.TrimSpace(u)
        if u == "" {
                return func() tea.Msg { return urlOpenDoneMsg{err: errors.New("empty url")} }
        }

        return func() tea.Msg {
                var cmd *exec.Cmd
                switch runtime.GOOS {
                case "darwin":
                        cmd = exec.Command("open", u)
                case "windows":
                        cmd = exec.Command("cmd", "/c", "start", "", u)
                default:
                        cmd = exec.Command("xdg-open", u)
                }
                cmd.Stdout = io.Discard
                cmd.Stderr = io.Discard
                if err := cmd.Start(); err != nil {
                        return urlOpenDoneMsg{err: err}
                }
                return urlOpenDoneMsg{err: cmd.Wait()}
        }
}

func (m *appModel) openAttachmentFilePicker() tea.Cmd {
        if m == nil {
                return nil
        }

        fp := filepicker.New()
        fp.AllowedTypes = nil // allow any file type
        fp.FileAllowed = true
        fp.DirAllowed = false
        fp.ShowHidden = false
        fp.ShowPermissions = false
        fp.ShowSize = true
        fp.AutoHeight = false
        fp.Height = attachmentFilePickerHeight(m.height)
        fp.Cursor = "›"
        fp.KeyMap.Back = key.NewBinding(
                key.WithKeys("h", "backspace", "left"),
                key.WithHelp("h", "up"),
        )

        // Align picker colors with the Clarity palette.
        fp.Styles.Cursor = lipgloss.NewStyle().Foreground(colorAccent)
        fp.Styles.Selected = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
        fp.Styles.Directory = lipgloss.NewStyle().Foreground(colorAccent)
        fp.Styles.Symlink = lipgloss.NewStyle().Foreground(colorAccent)
        fp.Styles.DisabledFile = styleMuted()
        fp.Styles.DisabledSelected = styleMuted()
        fp.Styles.Permission = styleMuted()
        fp.Styles.FileSize = styleMuted().Width(fp.Styles.FileSize.GetWidth()).Align(lipgloss.Right)

        // Start in the last used directory, else fall back to the user's home.
        startDir := strings.TrimSpace(m.attachmentFilePickerLastDir)
        if startDir == "" {
                if home, err := os.UserHomeDir(); err == nil {
                        startDir = strings.TrimSpace(home)
                }
        }
        if startDir == "" {
                startDir = "."
        }
        fp.CurrentDirectory = startDir

        m.attachmentFilePicker = fp
        m.modal = modalPickAttachmentFile
        m.modalForID = ""
        m.modalForKey = ""

        return fp.Init()
}

func (m *appModel) queueCommentDraftAttachment() error {
        if m == nil {
                return nil
        }
        path := strings.TrimSpace(m.attachmentAddPath)
        if path == "" {
                return errors.New("attachment: missing path")
        }
        title := strings.TrimSpace(m.attachmentAddTitle)
        alt := strings.TrimSpace(m.attachmentAddAlt)

        m.commentDraftAttachments = append(m.commentDraftAttachments, attachmentDraft{
                Path:  path,
                Title: title,
                Alt:   alt,
        })

        name := strings.TrimSpace(title)
        if name == "" {
                name = filepath.Base(path)
        }
        m.showMinibuffer("Queued attachment: " + name)

        // Reset attachment draft fields.
        m.attachmentAddKind = ""
        m.attachmentAddEntityID = ""
        m.attachmentAddPath = ""
        m.attachmentAddTitle = ""
        m.attachmentAddAlt = ""
        m.attachmentAddTitleHint = ""
        m.attachmentAddFlow = attachmentAddFlowCommit

        // Return to the original comment modal.
        m.modal = m.attachmentAddReturnModal
        m.modalForID = strings.TrimSpace(m.attachmentAddReturnForID)
        m.modalForKey = strings.TrimSpace(m.attachmentAddReturnForKey)
        m.attachmentAddReturnModal = modalNone
        m.attachmentAddReturnForID = ""
        m.attachmentAddReturnForKey = ""

        m.textFocus = textFocusBody
        m.textarea.Focus()
        return nil
}

func (m *appModel) renderAttachmentFilePickerModal() string {
        if m == nil {
                return ""
        }

        target := strings.TrimSpace(m.attachmentAddKind)
        if id := strings.TrimSpace(m.attachmentAddEntityID); id != "" {
                if target != "" {
                        target += " "
                }
                target += id
        }
        if target != "" {
                target = "Target: " + target + "\n\n"
        }

        help := styleMuted().Render("enter: select   esc/ctrl+g: cancel   h/backspace: up   l/right: open dir   j/k: move")
        body := target + m.attachmentFilePicker.View() + "\n" + help
        return renderModalBox(m.width, "Attachment: file", body)
}

func (m *appModel) commitAttachmentAdd() error {
        if m == nil || m.db == nil {
                return nil
        }
        kind := strings.TrimSpace(m.attachmentAddKind)
        entityID := strings.TrimSpace(m.attachmentAddEntityID)
        path := strings.TrimSpace(m.attachmentAddPath)
        title := strings.TrimSpace(m.attachmentAddTitle)
        alt := strings.TrimSpace(m.attachmentAddAlt)

        if kind == "" || entityID == "" || path == "" {
                return errors.New("attachments: missing target/path")
        }

        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db

        actorID := m.editActorID()
        if actorID == "" {
                return errors.New("no current actor")
        }

        switch strings.ToLower(kind) {
        case "item":
                it, ok := m.db.FindItem(entityID)
                if !ok || it == nil {
                        return errors.New("item not found")
                }
                if !canEditItem(m.db, actorID, it) {
                        return errors.New("permission denied")
                }
        case "comment":
                var c *model.Comment
                for i := range m.db.Comments {
                        if strings.TrimSpace(m.db.Comments[i].ID) == entityID {
                                c = &m.db.Comments[i]
                                break
                        }
                }
                if c == nil {
                        return errors.New("comment not found")
                }
                it, ok := m.db.FindItem(strings.TrimSpace(c.ItemID))
                if !ok || it == nil {
                        return errors.New("comment item not found")
                }
                if !canEditItem(m.db, actorID, it) {
                        return errors.New("permission denied")
                }
        default:
                return errors.New("attachments: invalid kind")
        }

        a, err := m.store.AddAttachment(m.db, actorID, kind, entityID, path, title, alt, store.DefaultAttachmentMaxBytes)
        if err != nil {
                return err
        }
        if err := m.appendEvent(actorID, "attachment.add", a.ID, a); err != nil {
                return err
        }
        if err := m.store.Save(m.db); err != nil {
                return err
        }

        name := strings.TrimSpace(a.Title)
        if name == "" {
                name = strings.TrimSpace(a.OriginalName)
        }
        if name == "" {
                name = filepath.Base(strings.TrimSpace(a.Path))
        }
        m.showMinibuffer("Attached: " + name)

        // Reset draft.
        m.attachmentAddKind = ""
        m.attachmentAddEntityID = ""
        m.attachmentAddPath = ""
        m.attachmentAddTitle = ""
        m.attachmentAddAlt = ""
        m.attachmentAddTitleHint = ""

        return nil
}

func (m *appModel) commitAttachmentEdit() error {
        if m == nil || m.db == nil {
                return nil
        }
        attachmentID := strings.TrimSpace(m.attachmentEditID)
        if attachmentID == "" {
                return errors.New("attachments: missing attachment id")
        }

        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db

        actorID := m.editActorID()
        if actorID == "" {
                return errors.New("no current actor")
        }

        a, ok := m.db.FindAttachment(attachmentID)
        if !ok || a == nil {
                return errors.New("attachment not found")
        }

        // Permission: reuse item permission rules via the owning item.
        switch strings.ToLower(strings.TrimSpace(a.EntityKind)) {
        case "item":
                it, ok := m.db.FindItem(strings.TrimSpace(a.EntityID))
                if !ok || it == nil {
                        return errors.New("attachment item not found")
                }
                if !canEditItem(m.db, actorID, it) {
                        return errors.New("permission denied")
                }
        case "comment":
                var c *model.Comment
                for i := range m.db.Comments {
                        if strings.TrimSpace(m.db.Comments[i].ID) == strings.TrimSpace(a.EntityID) {
                                c = &m.db.Comments[i]
                                break
                        }
                }
                if c == nil {
                        return errors.New("attachment comment not found")
                }
                it, ok := m.db.FindItem(strings.TrimSpace(c.ItemID))
                if !ok || it == nil {
                        return errors.New("attachment comment item not found")
                }
                if !canEditItem(m.db, actorID, it) {
                        return errors.New("permission denied")
                }
        default:
                return errors.New("attachment: invalid entity kind")
        }

        updated, err := m.store.UpdateAttachmentMetadata(m.db, actorID, attachmentID, m.attachmentEditTitle, m.attachmentEditAlt)
        if err != nil {
                return err
        }
        if err := m.appendEvent(actorID, "attachment.update", updated.ID, updated); err != nil {
                return err
        }
        if err := m.store.Save(m.db); err != nil {
                return err
        }

        name := strings.TrimSpace(updated.Title)
        if name == "" {
                name = strings.TrimSpace(updated.OriginalName)
        }
        if name == "" {
                name = filepath.Base(strings.TrimSpace(updated.Path))
        }
        m.showMinibuffer("Updated attachment: " + name)

        m.attachmentEditID = ""
        m.attachmentEditTitle = ""
        m.attachmentEditAlt = ""
        return nil
}

func (m *appModel) attachQueuedDraftsToComment(commentID string) error {
        if m == nil {
                return nil
        }
        commentID = strings.TrimSpace(commentID)
        if commentID == "" || len(m.commentDraftAttachments) == 0 {
                return nil
        }

        actorID := m.currentWriteActorID()
        if strings.TrimSpace(actorID) == "" {
                return nil
        }

        db, err := m.store.Load()
        if err != nil {
                return err
        }
        m.db = db

        added := 0
        for i := range m.commentDraftAttachments {
                d := m.commentDraftAttachments[i]
                a, err := m.store.AddAttachment(m.db, actorID, "comment", commentID, d.Path, d.Title, d.Alt, store.DefaultAttachmentMaxBytes)
                if err != nil {
                        m.showMinibuffer("Attachment error: " + err.Error())
                        continue
                }
                if err := m.appendEvent(actorID, "attachment.add", a.ID, a); err != nil {
                        return err
                }
                added++
        }
        if added == 0 {
                return nil
        }
        if err := m.store.Save(m.db); err != nil {
                return err
        }
        m.refreshEventsTail()
        m.captureStoreModTimes()
        m.showMinibuffer(fmt.Sprintf("Comment added (+%d attachment(s))", added))
        return nil
}
