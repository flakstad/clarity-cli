package tui

import (
        "fmt"
        "os"
        "os/exec"
        "strings"

        tea "github.com/charmbracelet/bubbletea"
)

type externalEditorDoneMsg struct {
        err error
}

func externalEditorName() string {
        if v := strings.TrimSpace(os.Getenv("VISUAL")); v != "" {
                return v
        }
        if v := strings.TrimSpace(os.Getenv("EDITOR")); v != "" {
                return v
        }
        return "vi"
}

func (m *appModel) openExternalEditorForTextarea() (tea.Cmd, error) {
        editor := externalEditorName()
        args := splitShellWords(editor)
        if len(args) == 0 {
                args = []string{"vi"}
        }

        f, err := os.CreateTemp("", "clarity-md-*.md")
        if err != nil {
                return nil, err
        }
        path := f.Name()

        if _, err := f.WriteString(m.textarea.Value()); err != nil {
                _ = f.Close()
                _ = os.Remove(path)
                return nil, err
        }
        _ = f.Close()

        m.externalEditorPath = path
        m.externalEditorBefore = m.textarea.Value()

        cmd := exec.Command(args[0], append(args[1:], path)...)
        return tea.ExecProcess(cmd, func(err error) tea.Msg {
                return externalEditorDoneMsg{err: err}
        }), nil
}

func (m *appModel) applyExternalEditorResult(msg externalEditorDoneMsg) {
        path := m.externalEditorPath
        before := m.externalEditorBefore

        m.externalEditorPath = ""
        m.externalEditorBefore = ""
        defer func() { _ = os.Remove(path) }()

        if strings.TrimSpace(path) == "" {
                return
        }

        if msg.err != nil {
                m.showMinibuffer("Editor failed: " + msg.err.Error())
                return
        }

        b, err := os.ReadFile(path)
        if err != nil {
                m.showMinibuffer("Editor read failed: " + err.Error())
                return
        }

        after := string(b)
        m.textarea.SetValue(after)

        if strings.TrimSpace(after) == strings.TrimSpace(before) {
                m.showMinibuffer(fmt.Sprintf("No changes from %s", externalEditorName()))
                return
        }
        m.showMinibuffer(fmt.Sprintf("Updated from %s (ctrl+s to save)", externalEditorName()))
}
