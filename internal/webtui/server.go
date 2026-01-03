package webtui

import (
        "embed"
        "errors"
        "html/template"
        "io"
        "net/http"
        "strings"
)

//go:embed templates/*.html static/*.css static/*.js static/xterm/*.js static/xterm/*.css
var assetsFS embed.FS

type ServerConfig struct {
        Addr      string
        Dir       string
        Workspace string
        ActorID   string
}

type Server struct {
        cfg  ServerConfig
        tmpl *template.Template
}

func NewServer(cfg ServerConfig) (*Server, error) {
        if strings.TrimSpace(cfg.Addr) == "" {
                return nil, errors.New("webtui: missing addr")
        }
        tmpl, err := template.ParseFS(assetsFS, "templates/*.html")
        if err != nil {
                return nil, err
        }
        return &Server{cfg: cfg, tmpl: tmpl}, nil
}

func (s *Server) Addr() string {
        return strings.TrimSpace(s.cfg.Addr)
}

func (s *Server) Handler() http.Handler {
        mux := http.NewServeMux()

        mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
                http.Redirect(w, r, "/terminal", http.StatusFound)
        })
        mux.HandleFunc("GET /terminal", s.handleTerminal)
        mux.HandleFunc("GET /ws", s.handleWS)

        mux.HandleFunc("GET /static/app.css", s.handleStatic("static/app.css", "text/css; charset=utf-8"))
        mux.HandleFunc("GET /static/app.js", s.handleStatic("static/app.js", "text/javascript; charset=utf-8"))
        mux.HandleFunc("GET /static/xterm/xterm.css", s.handleStatic("static/xterm/xterm.css", "text/css; charset=utf-8"))
        mux.HandleFunc("GET /static/xterm/xterm.js", s.handleStatic("static/xterm/xterm.js", "text/javascript; charset=utf-8"))
        mux.HandleFunc("GET /static/xterm/xterm-addon-fit.js", s.handleStatic("static/xterm/xterm-addon-fit.js", "text/javascript; charset=utf-8"))

        return mux
}

func (s *Server) handleStatic(path, contentType string) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                b, err := assetsFS.ReadFile(path)
                if err != nil {
                        http.Error(w, "not found", http.StatusNotFound)
                        return
                }
                w.Header().Set("Content-Type", contentType)
                _, _ = w.Write(b)
        }
}

type terminalVM struct {
        Workspace string
        ActorID   string
        Dir       string
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
        vm := terminalVM{
                Workspace: strings.TrimSpace(s.cfg.Workspace),
                ActorID:   strings.TrimSpace(s.cfg.ActorID),
                Dir:       strings.TrimSpace(s.cfg.Dir),
        }
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        if err := s.tmpl.ExecuteTemplate(w, "terminal.html", vm); err != nil {
                // Best-effort; if headers already sent, just write.
                http.Error(w, err.Error(), http.StatusInternalServerError)
                _, _ = io.WriteString(w, err.Error())
                return
        }
}
