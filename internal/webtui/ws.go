package webtui

import (
        "context"
        "encoding/json"
        "errors"
        "io"
        "net/http"
        "os"
        "os/exec"
        "strings"
        "sync"
        "time"

        "github.com/creack/pty"
        "github.com/gorilla/websocket"
)

type wsMsg struct {
        Type string `json:"type"`
        Cols int    `json:"cols"`
        Rows int    `json:"rows"`
}

var wsUpgrader = websocket.Upgrader{
        ReadBufferSize:  32 * 1024,
        WriteBufferSize: 32 * 1024,
        CheckOrigin: func(r *http.Request) bool {
                origin := strings.TrimSpace(r.Header.Get("Origin"))
                if origin == "" {
                        return true
                }
                // Basic same-origin check; good enough for localhost demo.
                host := strings.TrimSpace(r.Host)
                return strings.Contains(origin, "://"+host)
        },
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
        conn, err := wsUpgrader.Upgrade(w, r, nil)
        if err != nil {
                http.Error(w, "websocket upgrade failed", http.StatusBadRequest)
                return
        }
        defer conn.Close()

        ctx, cancel := context.WithCancel(r.Context())
        defer cancel()

        ptmx, cmd, cleanup, err := s.startPTYSession()
        if err != nil {
                _ = conn.WriteMessage(websocket.TextMessage, []byte("failed to start session: "+err.Error()))
                return
        }
        defer cleanup()

        var wg sync.WaitGroup
        errCh := make(chan error, 2)

        wg.Add(1)
        go func() {
                defer wg.Done()
                errCh <- pumpPTYToWS(ctx, ptmx, conn)
        }()

        wg.Add(1)
        go func() {
                defer wg.Done()
                errCh <- pumpWSToPTY(ctx, conn, ptmx)
        }()

        // Wait for either direction to stop.
        select {
        case <-ctx.Done():
        case <-errCh:
        }
        cancel()

        // Best-effort: tell the child to exit.
        _ = cmd.Process.Kill()

        wg.Wait()
}

func (s *Server) startPTYSession() (*os.File, *exec.Cmd, func(), error) {
        exe, err := os.Executable()
        if err != nil {
                return nil, nil, nil, err
        }

        dir := strings.TrimSpace(s.cfg.Dir)
        workspace := strings.TrimSpace(s.cfg.Workspace)
        actor := strings.TrimSpace(s.cfg.ActorID)

        args := []string{}
        if dir != "" {
                args = append(args, "--dir", dir)
        }
        if workspace != "" {
                args = append(args, "--workspace", workspace)
        }
        if actor != "" {
                args = append(args, "--actor", actor)
        }
        // No subcommand => interactive TUI.
        cmd := exec.Command(exe, args...)
        cmd.Env = append(os.Environ(),
                "TERM=xterm-256color",
                "COLORTERM=truecolor",
        )

        ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 120, Rows: 40})
        if err != nil {
                return nil, nil, nil, err
        }

        cleanup := func() {
                _ = ptmx.Close()
                _ = cmd.Process.Kill()
                _, _ = cmd.Process.Wait()
        }

        return ptmx, cmd, cleanup, nil
}

func pumpPTYToWS(ctx context.Context, ptmx *os.File, conn *websocket.Conn) error {
        buf := make([]byte, 32*1024)
        for {
                select {
                case <-ctx.Done():
                        return ctx.Err()
                default:
                }
                n, err := ptmx.Read(buf)
                if n > 0 {
                        _ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
                        if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
                                return werr
                        }
                }
                if err != nil {
                        if errors.Is(err, io.EOF) {
                                return nil
                        }
                        return err
                }
        }
}

func pumpWSToPTY(ctx context.Context, conn *websocket.Conn, ptmx *os.File) error {
        for {
                select {
                case <-ctx.Done():
                        return ctx.Err()
                default:
                }
                mt, data, err := conn.ReadMessage()
                if err != nil {
                        return err
                }

                // Control messages are JSON text. Keystroke frames are plain text or binary.
                if mt == websocket.TextMessage && len(data) > 0 && data[0] == '{' {
                        var m wsMsg
                        if jerr := json.Unmarshal(data, &m); jerr != nil {
                                continue
                        }
                        if strings.TrimSpace(strings.ToLower(m.Type)) == "resize" && m.Cols > 0 && m.Rows > 0 {
                                _ = pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(m.Cols), Rows: uint16(m.Rows)})
                        }
                        continue
                }

                if len(data) == 0 {
                        continue
                }
                if _, err := ptmx.Write(data); err != nil {
                        return err
                }
        }
}
