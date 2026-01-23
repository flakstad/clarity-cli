package web

import (
	"errors"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"clarity-cli/internal/store"
)

type ServerConfig struct {
	Dir       string
	Workspace string
}

type Server struct {
	cfg  ServerConfig
	db   *store.DB
	tmpl *template.Template
}

func NewServer(cfg ServerConfig) (*Server, error) {
	cfg.Dir = strings.TrimSpace(cfg.Dir)
	cfg.Workspace = strings.TrimSpace(cfg.Workspace)
	if cfg.Dir == "" {
		return nil, errors.New("web: missing dir")
	}

	db, err := (store.Store{Dir: cfg.Dir}).Load()
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New("index").Parse(indexHTML)
	if err != nil {
		return nil, err
	}

	return &Server{
		cfg:  cfg,
		db:   db,
		tmpl: tmpl,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /static/app.css", s.handleCSS)
	return withSecurityHeaders(mux)
}

type indexProject struct {
	ID       string
	Name     string
	Archived bool
}

type indexModel struct {
	Workspace string
	Projects  []indexProject
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	projects := make([]indexProject, 0, len(s.db.Projects))
	for _, p := range s.db.Projects {
		projects = append(projects, indexProject{
			ID:       p.ID,
			Name:     p.Name,
			Archived: p.Archived,
		})
	}
	sort.SliceStable(projects, func(i, j int) bool {
		return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tmpl.Execute(w, indexModel{
		Workspace: s.cfg.Workspace,
		Projects:  projects,
	})
}

func (s *Server) handleCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(appCSS))
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

const indexHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Clarity</title>
    <link rel="stylesheet" href="/static/app.css" />
  </head>
  <body>
    <main class="container">
      <header class="header">
        <h1 class="title">Clarity</h1>
        <div class="subtitle">Workspace: {{if .Workspace}}{{.Workspace}}{{else}}default{{end}}</div>
      </header>

      <section class="panel">
        <h2 class="panelTitle">Projects</h2>
        {{if .Projects}}
          <ul class="list">
            {{range .Projects}}
              <li class="row">
                <div class="rowMain">
                  <div class="rowName">{{.Name}}</div>
                  <div class="rowMeta">
                    <span class="mono">{{.ID}}</span>
                    {{if .Archived}}<span class="badge">archived</span>{{end}}
                  </div>
                </div>
              </li>
            {{end}}
          </ul>
        {{else}}
          <div class="empty">No projects yet.</div>
        {{end}}
      </section>
    </main>
  </body>
</html>
`

const appCSS = `
:root{
  --bg: #0b0c10;
  --panel: #111217;
  --text: #e8eaf0;
  --muted: #a6adbb;
  --line: rgba(255,255,255,0.08);
  --badge: rgba(255,255,255,0.10);
  --mono: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  --sans: ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, "Apple Color Emoji", "Segoe UI Emoji";
}
*{box-sizing:border-box}
html,body{height:100%}
body{
  margin:0;
  font-family:var(--sans);
  background: radial-gradient(1200px 800px at 20% 10%, #141528, var(--bg));
  color:var(--text);
}
.container{
  max-width: 860px;
  margin: 0 auto;
  padding: 32px 20px 60px;
}
.header{margin-bottom:18px}
.title{margin:0; font-size:28px; letter-spacing:0.2px}
.subtitle{margin-top:6px; color:var(--muted); font-size:14px}
.panel{
  background: rgba(17,18,23,0.88);
  border: 1px solid var(--line);
  border-radius: 12px;
  overflow:hidden;
  backdrop-filter: blur(8px);
}
.panelTitle{
  margin:0;
  padding: 14px 16px;
  font-size: 14px;
  letter-spacing: 0.4px;
  text-transform: uppercase;
  color: var(--muted);
  border-bottom: 1px solid var(--line);
}
.list{list-style:none; margin:0; padding:0}
.row{border-bottom:1px solid var(--line)}
.row:last-child{border-bottom:none}
.rowMain{padding: 14px 16px}
.rowName{font-size:16px; margin-bottom:6px}
.rowMeta{display:flex; gap:10px; align-items:center; color:var(--muted); font-size:12px}
.mono{font-family:var(--mono)}
.badge{
  display:inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--badge);
  color: var(--text);
}
.empty{padding: 16px; color: var(--muted)}
@media (prefers-color-scheme: light){
  :root{
    --bg:#f4f6fb;
    --panel:#ffffff;
    --text:#111827;
    --muted:#4b5563;
    --line: rgba(17,24,39,0.12);
    --badge: rgba(17,24,39,0.08);
  }
  body{background: radial-gradient(1200px 800px at 20% 10%, #e7eeff, var(--bg))}
  .panel{background: rgba(255,255,255,0.92)}
}
`
