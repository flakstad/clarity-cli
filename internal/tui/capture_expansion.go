package tui

import (
        "bytes"
        "context"
        "os"
        "os/exec"
        "runtime"
        "strings"
        "time"
)

type captureExpansionContext struct {
        Now       time.Time
        Workspace string
        OutlineID string

        Clipboard string
        Selection string
        URL       string

        Vars map[string]string
}

func newCaptureExpansionContext(workspace, outlineID string) captureExpansionContext {
        return captureExpansionContext{
                Now:       time.Now(),
                Workspace: strings.TrimSpace(workspace),
                OutlineID: strings.TrimSpace(outlineID),
                Clipboard: bestEffortCaptureValue("CLARITY_CAPTURE_CLIPBOARD", bestEffortClipboardText),
                Selection: strings.TrimSpace(os.Getenv("CLARITY_CAPTURE_SELECTION")),
                URL:       strings.TrimSpace(os.Getenv("CLARITY_CAPTURE_URL")),
        }
}

func bestEffortCaptureValue(envKey string, fallback func() string) string {
        if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
                return v
        }
        if fallback == nil {
                return ""
        }
        return strings.TrimSpace(fallback())
}

func bestEffortClipboardText() string {
        ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
        defer cancel()

        switch runtime.GOOS {
        case "darwin":
                return runSmallCommand(ctx, "pbpaste")
        case "windows":
                // Best-effort; ignored if powershell isn't available.
                return runSmallCommand(ctx, "powershell.exe", "-NoProfile", "-Command", "Get-Clipboard")
        default:
                // Linux/others: try common clipboard tools.
                if s := runSmallCommand(ctx, "wl-paste", "--no-newline"); s != "" {
                        return s
                }
                if s := runSmallCommand(ctx, "xclip", "-selection", "clipboard", "-o"); s != "" {
                        return s
                }
                if s := runSmallCommand(ctx, "xsel", "--clipboard", "--output"); s != "" {
                        return s
                }
                return ""
        }
}

func runSmallCommand(ctx context.Context, name string, args ...string) string {
        cmd := exec.CommandContext(ctx, name, args...)
        var stdout bytes.Buffer
        cmd.Stdout = &stdout
        _ = cmd.Run()
        s := stdout.String()
        s = strings.TrimRight(s, "\r\n")
        if len(s) > 64*1024 {
                s = s[:64*1024]
        }
        return s
}

func expandCaptureTemplateString(in string, ctx captureExpansionContext) string {
        if in == "" {
                return ""
        }

        var out strings.Builder
        out.Grow(len(in))

        for {
                start := strings.Index(in, "{{")
                if start < 0 {
                        out.WriteString(in)
                        break
                }
                out.WriteString(in[:start])
                in = in[start+2:]

                end := strings.Index(in, "}}")
                if end < 0 {
                        out.WriteString("{{")
                        out.WriteString(in)
                        break
                }

                token := strings.TrimSpace(in[:end])
                in = in[end+2:]

                switch token {
                case "now":
                        out.WriteString(ctx.Now.Format(time.RFC3339))
                case "date":
                        out.WriteString(ctx.Now.Format("2006-01-02"))
                case "time":
                        out.WriteString(ctx.Now.Format("15:04"))
                case "workspace":
                        out.WriteString(ctx.Workspace)
                case "outline":
                        out.WriteString(ctx.OutlineID)
                case "clipboard":
                        out.WriteString(ctx.Clipboard)
                case "selection":
                        out.WriteString(ctx.Selection)
                case "url":
                        out.WriteString(ctx.URL)
                default:
                        if ctx.Vars != nil {
                                if v, ok := ctx.Vars[token]; ok {
                                        out.WriteString(v)
                                        continue
                                }
                        }
                        out.WriteString("{{")
                        out.WriteString(token)
                        out.WriteString("}}")
                }
        }

        return out.String()
}
