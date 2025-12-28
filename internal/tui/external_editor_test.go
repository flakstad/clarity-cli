package tui

import (
        "os"
        "path/filepath"
        "testing"

        "github.com/charmbracelet/bubbles/textarea"
)

func TestApplyExternalEditorResult_UpdatesTextareaAndCleansUp(t *testing.T) {
        t.Parallel()

        var m appModel
        m.textarea = textarea.New()
        m.textarea.SetValue("before")

        dir := t.TempDir()
        path := filepath.Join(dir, "edited.md")
        if err := os.WriteFile(path, []byte("after\n"), 0600); err != nil {
                t.Fatalf("write temp file: %v", err)
        }

        m.externalEditorPath = path
        m.externalEditorBefore = "before"
        m.applyExternalEditorResult(externalEditorDoneMsg{err: nil})

        if got := m.textarea.Value(); got != "after\n" {
                t.Fatalf("expected textarea to be updated, got %q", got)
        }
        if _, err := os.Stat(path); !os.IsNotExist(err) {
                t.Fatalf("expected temp file to be removed, stat err=%v", err)
        }
}
