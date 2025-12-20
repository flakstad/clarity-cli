package tui

import (
        "errors"
        "os/exec"
        "runtime"
        "strings"
)

func copyToClipboard(s string) error {
        s = strings.ReplaceAll(s, "\r\n", "\n")

        switch runtime.GOOS {
        case "darwin":
                return runClipboardCmd("pbcopy", nil, s)
        case "windows":
                // Try clip.exe first; fall back to PowerShell.
                if err := runClipboardCmd("cmd", []string{"/c", "clip"}, s); err == nil {
                        return nil
                }
                return runClipboardCmd("powershell", []string{"-NoProfile", "-Command", "Set-Clipboard"}, s)
        default:
                // Prefer Wayland if available, then X11 fallbacks.
                if err := runClipboardCmd("wl-copy", nil, s); err == nil {
                        return nil
                }
                if err := runClipboardCmd("xclip", []string{"-selection", "clipboard"}, s); err == nil {
                        return nil
                }
                return runClipboardCmd("xsel", []string{"--clipboard", "--input"}, s)
        }
}

func runClipboardCmd(name string, args []string, stdin string) error {
        if _, err := exec.LookPath(name); err != nil {
                return err
        }
        cmd := exec.Command(name, args...)
        cmd.Stdin = strings.NewReader(stdin)
        if err := cmd.Run(); err != nil {
                return errors.New(name + ": " + err.Error())
        }
        return nil
}
