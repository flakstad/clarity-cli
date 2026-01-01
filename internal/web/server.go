package web

import (
        "context"
        "embed"
        "encoding/json"
        "errors"
        "html/template"
        "net/http"
        "os"
        "path/filepath"
        "sort"
        "strings"
        "time"

        "clarity-cli/internal/gitrepo"
        "clarity-cli/internal/model"
        "clarity-cli/internal/perm"
        "clarity-cli/internal/statusutil"
        "clarity-cli/internal/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

type ServerConfig struct {
        Addr      string
        Dir       string
        Workspace string
        ActorID   string
        ReadOnly  bool
        AuthMode  string // none|dev|magic

        // ComponentsDir points at a local checkout of `clarity-components` (for now),
        // used to serve `outline.js` to the browser.
        ComponentsDir string
}

type Server struct {
        cfg  ServerConfig
        tmpl *template.Template
}

func NewServer(cfg ServerConfig) (*Server, error) {
        cfg.Addr = strings.TrimSpace(cfg.Addr)
        cfg.Dir = strings.TrimSpace(cfg.Dir)
        cfg.Workspace = strings.TrimSpace(cfg.Workspace)
        cfg.ActorID = strings.TrimSpace(cfg.ActorID)
        cfg.AuthMode = strings.ToLower(strings.TrimSpace(cfg.AuthMode))
        cfg.ComponentsDir = strings.TrimSpace(cfg.ComponentsDir)
        if cfg.Addr == "" {
                return nil, errors.New("web: addr is empty")
        }
        if cfg.Dir == "" {
                return nil, errors.New("web: dir is empty")
        }
        if cfg.AuthMode == "" {
                cfg.AuthMode = "none"
        }
        if cfg.AuthMode != "none" && cfg.AuthMode != "dev" && cfg.AuthMode != "magic" {
                return nil, errors.New("web: invalid auth mode (expected none|dev|magic)")
        }

        tmpl, err := template.New("base").Funcs(template.FuncMap{
                "trim": strings.TrimSpace,
        }).ParseFS(templatesFS, "templates/*.html")
        if err != nil {
                return nil, err
        }

        return &Server{cfg: cfg, tmpl: tmpl}, nil
}

func (s *Server) Addr() string { return s.cfg.Addr }

func (s *Server) Handler() http.Handler {
        mux := http.NewServeMux()
        mux.HandleFunc("GET /health", s.handleHealth)
        mux.HandleFunc("GET /static/outline.js", s.handleOutlineJS)
        mux.HandleFunc("GET /", s.handleHome)
        mux.HandleFunc("GET /login", s.handleLoginGet)
        mux.HandleFunc("POST /login", s.handleLoginPost)
        mux.HandleFunc("GET /verify", s.handleVerifyGet)
        mux.HandleFunc("POST /logout", s.handleLogoutPost)
        mux.HandleFunc("GET /agenda", s.handleAgenda)
        mux.HandleFunc("GET /sync", s.handleSync)
        mux.HandleFunc("POST /sync/pull", s.handleSyncPull)
        mux.HandleFunc("POST /sync/push", s.handleSyncPush)
        mux.HandleFunc("GET /projects", s.handleProjects)
        mux.HandleFunc("GET /projects/{projectId}", s.handleProject)
        mux.HandleFunc("GET /outlines/{outlineId}", s.handleOutline)
        mux.HandleFunc("GET /items/{itemId}", s.handleItem)
        mux.HandleFunc("POST /items/{itemId}/comments", s.handleItemCommentAdd)
        return mux
}

func (s *Server) handleOutlineJS(w http.ResponseWriter, r *http.Request) {
        dir := strings.TrimSpace(s.cfg.ComponentsDir)
        if dir == "" {
                http.NotFound(w, r)
                return
        }
        p := filepath.Join(dir, "outline.js")
        b, err := os.ReadFile(p)
        if err != nil {
                http.NotFound(w, r)
                return
        }
        w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write(b)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok\n"))
}

type homeVM struct {
        Now       string
        Workspace string
        Dir       string
        ReadOnly  bool
        ActorID   string
        AuthMode  string
        Git       gitrepo.Status
        Projects  []model.Project
        Ready     []model.Item
}

func unarchivedProjects(projects []model.Project) []model.Project {
        out := make([]model.Project, 0, len(projects))
        for _, p := range projects {
                if p.Archived {
                        continue
                }
                out = append(out, p)
        }
        return out
}

func unarchivedOutlines(outlines []model.Outline, projectID string) []model.Outline {
        out := make([]model.Outline, 0, len(outlines))
        for _, o := range outlines {
                if o.ProjectID != projectID {
                        continue
                }
                if o.Archived {
                        continue
                }
                out = append(out, o)
        }
        return out
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
        defer cancel()

        st, _ := gitrepo.GetStatus(ctx, s.cfg.Dir)

        db, err := (store.Store{Dir: s.cfg.Dir}).Load()
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }

        actorID := s.actorForRequest(r)
        ready := readyItems(db)

        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        if err := s.tmpl.ExecuteTemplate(w, "home.html", homeVM{
                Now:       time.Now().Format(time.RFC3339),
                Workspace: s.cfg.Workspace,
                Dir:       s.cfg.Dir,
                ReadOnly:  s.cfg.ReadOnly,
                ActorID:   actorID,
                AuthMode:  s.cfg.AuthMode,
                Git:       st,
                Projects:  unarchivedProjects(db.Projects),
                Ready:     ready,
        }); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
}

type loginVM struct {
        Now       string
        Workspace string
        Dir       string
        Actors    []model.Actor
        AuthMode  string
        Error     string
}

func (s *Server) handleLoginGet(w http.ResponseWriter, r *http.Request) {
        if s.cfg.AuthMode != "dev" && s.cfg.AuthMode != "magic" {
                http.NotFound(w, r)
                return
        }
        db, err := (store.Store{Dir: s.cfg.Dir}).Load()
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }

        actors := []model.Actor{}
        if s.cfg.AuthMode == "dev" {
                for _, a := range db.Actors {
                        if a.Kind == model.ActorKindHuman {
                                actors = append(actors, a)
                        }
                }
        }

        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        tpl := "login_dev.html"
        if s.cfg.AuthMode == "magic" {
                tpl = "login_magic.html"
        }
        _ = s.tmpl.ExecuteTemplate(w, tpl, loginVM{
                Now:       time.Now().Format(time.RFC3339),
                Workspace: s.cfg.Workspace,
                Dir:       s.cfg.Dir,
                Actors:    actors,
                AuthMode:  s.cfg.AuthMode,
        })
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
        if s.cfg.AuthMode != "dev" && s.cfg.AuthMode != "magic" {
                http.NotFound(w, r)
                return
        }
        if err := r.ParseForm(); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
        }

        db, err := (store.Store{Dir: s.cfg.Dir}).Load()
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }

        secret, err := loadOrInitSecretKey(s.cfg.Dir)
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }

        switch s.cfg.AuthMode {
        case "dev":
                actorID := strings.TrimSpace(r.Form.Get("actor"))
                if actorID == "" {
                        http.Redirect(w, r, "/login", http.StatusSeeOther)
                        return
                }
                a, ok := db.FindActor(actorID)
                if !ok || a == nil || a.Kind != model.ActorKindHuman {
                        w.Header().Set("Content-Type", "text/html; charset=utf-8")
                        _ = s.tmpl.ExecuteTemplate(w, "login_dev.html", loginVM{
                                Now:       time.Now().Format(time.RFC3339),
                                Workspace: s.cfg.Workspace,
                                Dir:       s.cfg.Dir,
                                Actors:    db.Actors,
                                AuthMode:  s.cfg.AuthMode,
                                Error:     "unknown actor",
                        })
                        return
                }
                sess, err := newSessionToken(secret, actorID, 30*24*time.Hour)
                if err != nil {
                        http.Error(w, err.Error(), http.StatusInternalServerError)
                        return
                }
                http.SetCookie(w, &http.Cookie{
                        Name:     sessionCookieName,
                        Value:    sess,
                        Path:     "/",
                        HttpOnly: true,
                        SameSite: http.SameSiteLaxMode,
                })
                http.Redirect(w, r, "/", http.StatusSeeOther)
                return
        case "magic":
                email := strings.ToLower(strings.TrimSpace(r.Form.Get("email")))
                if email == "" {
                        http.Redirect(w, r, "/login", http.StatusSeeOther)
                        return
                }
                users, _, err := store.LoadUsers(s.cfg.Dir)
                if err != nil {
                        http.Error(w, err.Error(), http.StatusInternalServerError)
                        return
                }
                actorID, ok := users.ActorIDForEmail(email)
                if !ok {
                        w.Header().Set("Content-Type", "text/html; charset=utf-8")
                        _ = s.tmpl.ExecuteTemplate(w, "login_magic.html", loginVM{
                                Now:       time.Now().Format(time.RFC3339),
                                Workspace: s.cfg.Workspace,
                                Dir:       s.cfg.Dir,
                                AuthMode:  s.cfg.AuthMode,
                                Error:     "email is not registered in meta/users.json",
                        })
                        return
                }
                if a, ok := db.FindActor(actorID); !ok || a == nil {
                        http.Error(w, "meta/users.json maps to unknown actorId", http.StatusInternalServerError)
                        return
                }

                tok, err := newMagicToken(secret, email, 15*time.Minute)
                if err != nil {
                        http.Error(w, err.Error(), http.StatusInternalServerError)
                        return
                }

                link := "http://" + r.Host + "/verify?token=" + tok
                _ = writeOutboxEmail(s.cfg.Dir, email, "Clarity login link", link)

                w.Header().Set("Content-Type", "text/html; charset=utf-8")
                _ = s.tmpl.ExecuteTemplate(w, "login_magic_sent.html", map[string]any{
                        "Now":       time.Now().Format(time.RFC3339),
                        "Workspace": s.cfg.Workspace,
                        "Dir":       s.cfg.Dir,
                        "Email":     email,
                        "Link":      link,
                })
                return
        default:
                http.NotFound(w, r)
                return
        }
}

func (s *Server) handleLogoutPost(w http.ResponseWriter, r *http.Request) {
        if s.cfg.AuthMode != "dev" && s.cfg.AuthMode != "magic" {
                http.NotFound(w, r)
                return
        }
        http.SetCookie(w, &http.Cookie{
                Name:     sessionCookieName,
                Value:    "",
                Path:     "/",
                MaxAge:   -1,
                HttpOnly: true,
                SameSite: http.SameSiteLaxMode,
        })
        http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleVerifyGet(w http.ResponseWriter, r *http.Request) {
        if s.cfg.AuthMode != "magic" {
                http.NotFound(w, r)
                return
        }

        token := strings.TrimSpace(r.URL.Query().Get("token"))
        if token == "" {
                http.Error(w, "missing token", http.StatusBadRequest)
                return
        }
        secret, err := loadOrInitSecretKey(s.cfg.Dir)
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        sp, err := verifyToken(secret, token)
        if err != nil || sp.Typ != "magic" {
                http.Error(w, "invalid token", http.StatusForbidden)
                return
        }

        users, _, err := store.LoadUsers(s.cfg.Dir)
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        actorID, ok := users.ActorIDForEmail(sp.Sub)
        if !ok {
                http.Error(w, "email is not registered in meta/users.json", http.StatusForbidden)
                return
        }

        sess, err := newSessionToken(secret, actorID, 30*24*time.Hour)
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        http.SetCookie(w, &http.Cookie{
                Name:     sessionCookieName,
                Value:    sess,
                Path:     "/",
                HttpOnly: true,
                SameSite: http.SameSiteLaxMode,
        })
        http.Redirect(w, r, "/", http.StatusSeeOther)
}

type agendaVM struct {
        Now       string
        Workspace string
        Dir       string
        ActorID   string
        Items     []model.Item
}

func (s *Server) handleAgenda(w http.ResponseWriter, r *http.Request) {
        db, err := (store.Store{Dir: s.cfg.Dir}).Load()
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }

        actorID := strings.TrimSpace(s.actorForRequest(r))
        items := agendaItems(db, actorID)

        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        if err := s.tmpl.ExecuteTemplate(w, "agenda.html", agendaVM{
                Now:       time.Now().Format(time.RFC3339),
                Workspace: s.cfg.Workspace,
                Dir:       s.cfg.Dir,
                ActorID:   actorID,
                Items:     items,
        }); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
}

type syncVM struct {
        Now       string
        Workspace string
        Dir       string
        ReadOnly  bool
        Git       gitrepo.Status
        Message   string
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
        defer cancel()

        st, _ := gitrepo.GetStatus(ctx, s.cfg.Dir)
        msg := strings.TrimSpace(r.URL.Query().Get("msg"))

        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        if err := s.tmpl.ExecuteTemplate(w, "sync.html", syncVM{
                Now:       time.Now().Format(time.RFC3339),
                Workspace: s.cfg.Workspace,
                Dir:       s.cfg.Dir,
                ReadOnly:  s.cfg.ReadOnly,
                Git:       st,
                Message:   msg,
        }); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
}

func (s *Server) handleSyncPull(w http.ResponseWriter, r *http.Request) {
        if s.cfg.ReadOnly {
                http.Error(w, "read-only", http.StatusForbidden)
                return
        }

        ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
        defer cancel()

        before, err := gitrepo.GetStatus(ctx, s.cfg.Dir)
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        if !before.IsRepo {
                http.Error(w, "not a git repository", http.StatusBadRequest)
                return
        }
        if before.Unmerged || before.InProgress {
                http.Error(w, "repo has an in-progress merge/rebase; resolve first", http.StatusConflict)
                return
        }
        if before.DirtyTracked {
                http.Error(w, "repo has local changes; commit/push first", http.StatusConflict)
                return
        }

        if err := gitrepo.PullRebase(ctx, s.cfg.Dir); err != nil {
                http.Error(w, err.Error(), http.StatusConflict)
                return
        }
        http.Redirect(w, r, "/sync?msg=pull%20--rebase%20ok", http.StatusSeeOther)
}

func (s *Server) handleSyncPush(w http.ResponseWriter, r *http.Request) {
        if s.cfg.ReadOnly {
                http.Error(w, "read-only", http.StatusForbidden)
                return
        }

        ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
        defer cancel()

        before, err := gitrepo.GetStatus(ctx, s.cfg.Dir)
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        if !before.IsRepo {
                http.Error(w, "not a git repository", http.StatusBadRequest)
                return
        }
        if before.Unmerged || before.InProgress {
                http.Error(w, "repo has an in-progress merge/rebase; resolve first", http.StatusConflict)
                return
        }

        if err := r.ParseForm(); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
        }
        msg := strings.TrimSpace(r.Form.Get("message"))
        if msg == "" {
                actor := strings.TrimSpace(s.actorForRequest(r))
                if actor != "" {
                        msg = "clarity: update (" + actor + ")"
                }
        }

        committed, err := gitrepo.CommitWorkspaceCanonical(ctx, s.cfg.Dir, msg)
        if err != nil {
                http.Error(w, err.Error(), http.StatusConflict)
                return
        }

        // Pull/rebase (best-effort; same as CLI's default).
        pulled := false
        cur, err := gitrepo.GetStatus(ctx, s.cfg.Dir)
        if err == nil && strings.TrimSpace(cur.Upstream) != "" {
                if !cur.Unmerged && !cur.InProgress && !cur.DirtyTracked {
                        if err := gitrepo.PullRebase(ctx, s.cfg.Dir); err == nil {
                                pulled = true
                        }
                }
        }

        pushed := false
        if err := gitrepo.Push(ctx, s.cfg.Dir); err == nil {
                pushed = true
        } else if gitrepo.IsNonFastForwardPushErr(err) {
                // Retry once: pull --rebase + push.
                if err := gitrepo.PullRebase(ctx, s.cfg.Dir); err != nil {
                        http.Error(w, err.Error(), http.StatusConflict)
                        return
                }
                pulled = true
                if err := gitrepo.Push(ctx, s.cfg.Dir); err != nil {
                        http.Error(w, err.Error(), http.StatusConflict)
                        return
                }
                pushed = true
        } else {
                http.Error(w, err.Error(), http.StatusConflict)
                return
        }

        parts := []string{}
        if committed {
                parts = append(parts, "commit")
        }
        if pulled {
                parts = append(parts, "pull")
        }
        if pushed {
                parts = append(parts, "push")
        }
        if len(parts) == 0 {
                parts = append(parts, "no-op")
        }

        http.Redirect(w, r, "/sync?msg="+strings.Join(parts, "%20%2B%20")+"%20ok", http.StatusSeeOther)
}

type projectsVM struct {
        Now       string
        Workspace string
        Dir       string
        Projects  []model.Project
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
        db, err := (store.Store{Dir: s.cfg.Dir}).Load()
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }

        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        if err := s.tmpl.ExecuteTemplate(w, "projects.html", projectsVM{
                Now:       time.Now().Format(time.RFC3339),
                Workspace: s.cfg.Workspace,
                Dir:       s.cfg.Dir,
                Projects:  unarchivedProjects(db.Projects),
        }); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
}

type projectVM struct {
        Now       string
        Workspace string
        Dir       string
        Project   model.Project
        Outlines  []model.Outline
}

func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
        projectID := strings.TrimSpace(r.PathValue("projectId"))
        if projectID == "" {
                http.Error(w, "missing project id", http.StatusBadRequest)
                return
        }

        db, err := (store.Store{Dir: s.cfg.Dir}).Load()
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        p, ok := db.FindProject(projectID)
        if !ok || p == nil {
                http.NotFound(w, r)
                return
        }

        outlines := unarchivedOutlines(db.Outlines, projectID)

        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        if err := s.tmpl.ExecuteTemplate(w, "project.html", projectVM{
                Now:       time.Now().Format(time.RFC3339),
                Workspace: s.cfg.Workspace,
                Dir:       s.cfg.Dir,
                Project:   *p,
                Outlines:  outlines,
        }); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
}

type outlineVM struct {
        Now       string
        Workspace string
        Dir       string
        ActorID   string
        ReadOnly  bool
        Outline   model.Outline

        // Outline component view-model (progressive enhancement).
        UseOutlineComponent bool
        ItemsJSON           template.JS
        StatusLabelsJSON    template.JS
        AssigneesJSON       template.JS
        TagsJSON            template.JS

        // Fallback list (no JS / no components).
        Items []model.Item
}

func (s *Server) handleOutline(w http.ResponseWriter, r *http.Request) {
        outlineID := strings.TrimSpace(r.PathValue("outlineId"))
        if outlineID == "" {
                http.Error(w, "missing outline id", http.StatusBadRequest)
                return
        }

        db, err := (store.Store{Dir: s.cfg.Dir}).Load()
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        o, ok := db.FindOutline(outlineID)
        if !ok || o == nil {
                http.NotFound(w, r)
                return
        }
        if o.Archived {
                http.NotFound(w, r)
                return
        }

        items := make([]model.Item, 0)
        for _, it := range db.Items {
                if it.OutlineID == outlineID && !it.Archived {
                        items = append(items, it)
                }
        }

        actorID := strings.TrimSpace(s.actorForRequest(r))
        useComponent := strings.TrimSpace(s.cfg.ComponentsDir) != ""
        itemsJSON, statusJSON, assigneesJSON, tagsJSON := outlineComponentPayload(db, *o, actorID)

        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        if err := s.tmpl.ExecuteTemplate(w, "outline.html", outlineVM{
                Now:                 time.Now().Format(time.RFC3339),
                Workspace:           s.cfg.Workspace,
                Dir:                 s.cfg.Dir,
                ActorID:             actorID,
                ReadOnly:            s.cfg.ReadOnly,
                Outline:             *o,
                UseOutlineComponent: useComponent,
                ItemsJSON:           itemsJSON,
                StatusLabelsJSON:    statusJSON,
                AssigneesJSON:       assigneesJSON,
                TagsJSON:            tagsJSON,
                Items:               items,
        }); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
}

type outlineTodoNode struct {
        ID       string `json:"id"`
        Text     string `json:"text"`
        Status   string `json:"status"`
        Editable bool   `json:"editable"`

        Priority bool     `json:"priority,omitempty"`
        OnHold   bool     `json:"onHold,omitempty"`
        Due      string   `json:"due,omitempty"`
        Schedule string   `json:"schedule,omitempty"`
        Assign   string   `json:"assign,omitempty"`
        Tags     []string `json:"tags,omitempty"`

        Children []outlineTodoNode `json:"children,omitempty"`
}

type outlineStatusLabel struct {
        Label      string `json:"label"`
        IsEndState bool   `json:"isEndState"`
}

func outlineComponentPayload(db *store.DB, o model.Outline, actorID string) (items template.JS, statusLabels template.JS, assignees template.JS, tags template.JS) {
        // Build status labels contract for the component.
        status := make([]outlineStatusLabel, 0, len(o.StatusDefs))
        for _, sd := range o.StatusDefs {
                lbl := strings.TrimSpace(sd.Label)
                if lbl == "" {
                        continue
                }
                status = append(status, outlineStatusLabel{Label: lbl, IsEndState: sd.IsEndState})
        }
        if len(status) == 0 {
                status = append(status, outlineStatusLabel{Label: "TODO", IsEndState: false})
                status = append(status, outlineStatusLabel{Label: "DONE", IsEndState: true})
        }

        // Build assignee list contract for the component.
        assigneeLabels := make([]string, 0, len(db.Actors))
        for _, a := range db.Actors {
                if strings.TrimSpace(a.Name) == "" {
                        continue
                }
                assigneeLabels = append(assigneeLabels, a.Name)
        }
        sort.Strings(assigneeLabels)

        // Tags: unique across workspace.
        tagSet := map[string]struct{}{}
        for _, it := range db.Items {
                for _, t := range it.Tags {
                        t = strings.TrimSpace(t)
                        if t != "" {
                                tagSet[t] = struct{}{}
                        }
                }
        }
        allTags := make([]string, 0, len(tagSet))
        for t := range tagSet {
                allTags = append(allTags, t)
        }
        sort.Strings(allTags)

        // Prepare status mapping from statusID -> label.
        statusLabelByID := map[string]string{}
        for _, sd := range o.StatusDefs {
                if strings.TrimSpace(sd.ID) == "" || strings.TrimSpace(sd.Label) == "" {
                        continue
                }
                statusLabelByID[sd.ID] = sd.Label
        }

        // Filter and group items by parent, sorted by rank.
        itemsInOutline := make([]model.Item, 0)
        byID := map[string]model.Item{}
        for _, it := range db.Items {
                if it.OutlineID != o.ID || it.Archived {
                        continue
                }
                itemsInOutline = append(itemsInOutline, it)
                byID[it.ID] = it
        }

        children := map[string][]model.Item{}
        for _, it := range itemsInOutline {
                parent := ""
                if it.ParentID != nil {
                        parent = strings.TrimSpace(*it.ParentID)
                        if parent != "" {
                                if pit, ok := byID[parent]; !ok || pit.Archived {
                                        parent = ""
                                }
                        }
                }
                children[parent] = append(children[parent], it)
        }

        for k := range children {
                sort.Slice(children[k], func(i, j int) bool {
                        a := children[k][i]
                        b := children[k][j]
                        ra := strings.TrimSpace(a.Rank)
                        rb := strings.TrimSpace(b.Rank)
                        if ra != "" && rb != "" && ra != rb {
                                return ra < rb
                        }
                        return a.CreatedAt.Before(b.CreatedAt)
                })
        }

        actorID = strings.TrimSpace(actorID)

        var build func(parent string) []outlineTodoNode
        build = func(parent string) []outlineTodoNode {
                out := make([]outlineTodoNode, 0, len(children[parent]))
                for _, it := range children[parent] {
                        st := strings.TrimSpace(it.StatusID)
                        if lbl, ok := statusLabelByID[st]; ok && strings.TrimSpace(lbl) != "" {
                                st = lbl
                        }
                        node := outlineTodoNode{
                                ID:       it.ID,
                                Text:     it.Title,
                                Status:   st,
                                Editable: actorID != "" && perm.CanEditItem(db, actorID, &it),
                                Priority: it.Priority,
                                OnHold:   it.OnHold,
                                Tags:     it.Tags,
                                Children: build(it.ID),
                        }

                        if it.Due != nil && strings.TrimSpace(it.Due.Date) != "" {
                                node.Due = strings.TrimSpace(it.Due.Date)
                                if it.Due.Time != nil && strings.TrimSpace(*it.Due.Time) != "" {
                                        node.Due = node.Due + " " + strings.TrimSpace(*it.Due.Time)
                                }
                        }
                        if it.Schedule != nil && strings.TrimSpace(it.Schedule.Date) != "" {
                                node.Schedule = strings.TrimSpace(it.Schedule.Date)
                                if it.Schedule.Time != nil && strings.TrimSpace(*it.Schedule.Time) != "" {
                                        node.Schedule = node.Schedule + " " + strings.TrimSpace(*it.Schedule.Time)
                                }
                        }
                        if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) != "" {
                                if a, ok := db.FindActor(strings.TrimSpace(*it.AssignedActorID)); ok && a != nil && strings.TrimSpace(a.Name) != "" {
                                        node.Assign = strings.TrimSpace(a.Name)
                                } else {
                                        node.Assign = strings.TrimSpace(*it.AssignedActorID)
                                }
                        }

                        out = append(out, node)
                }
                return out
        }

        todos := build("")

        bTodos, _ := json.Marshal(todos)
        bStatus, _ := json.Marshal(status)
        bAssignees, _ := json.Marshal(assigneeLabels)
        bTags, _ := json.Marshal(allTags)
        return template.JS(bTodos), template.JS(bStatus), template.JS(bAssignees), template.JS(bTags)
}

type itemVM struct {
        Now       string
        Workspace string
        Dir       string
        ReadOnly  bool
        ActorID   string
        Item      model.Item
        Comments  []model.Comment
}

func (s *Server) handleItem(w http.ResponseWriter, r *http.Request) {
        itemID := strings.TrimSpace(r.PathValue("itemId"))
        if itemID == "" {
                http.Error(w, "missing item id", http.StatusBadRequest)
                return
        }

        db, err := (store.Store{Dir: s.cfg.Dir}).Load()
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        it, ok := db.FindItem(itemID)
        if !ok || it == nil {
                http.NotFound(w, r)
                return
        }

        comments := db.CommentsForItem(itemID)

        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        if err := s.tmpl.ExecuteTemplate(w, "item.html", itemVM{
                Now:       time.Now().Format(time.RFC3339),
                Workspace: s.cfg.Workspace,
                Dir:       s.cfg.Dir,
                ReadOnly:  s.cfg.ReadOnly,
                ActorID:   strings.TrimSpace(s.cfg.ActorID),
                Item:      *it,
                Comments:  comments,
        }); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
}

func (s *Server) handleItemCommentAdd(w http.ResponseWriter, r *http.Request) {
        if s.cfg.ReadOnly {
                http.Error(w, "read-only", http.StatusForbidden)
                return
        }

        itemID := strings.TrimSpace(r.PathValue("itemId"))
        if itemID == "" {
                http.Error(w, "missing item id", http.StatusBadRequest)
                return
        }

        actorID := strings.TrimSpace(s.actorForRequest(r))
        if actorID == "" {
                http.Error(w, "missing actor (start server with --actor, set currentActorId, or /login in dev auth mode)", http.StatusUnauthorized)
                return
        }

        if err := r.ParseForm(); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
        }
        body := strings.TrimSpace(r.Form.Get("body"))
        if body == "" {
                http.Redirect(w, r, "/items/"+itemID, http.StatusSeeOther)
                return
        }

        st := store.Store{Dir: s.cfg.Dir}
        db, err := st.Load()
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        if _, ok := db.FindActor(actorID); !ok {
                http.Error(w, "unknown actor", http.StatusForbidden)
                return
        }
        if _, ok := db.FindItem(itemID); !ok {
                http.NotFound(w, r)
                return
        }

        c := model.Comment{
                ID:        st.NextID(db, "cmt"),
                ItemID:    itemID,
                AuthorID:  actorID,
                Body:      body,
                CreatedAt: time.Now().UTC(),
        }
        db.Comments = append(db.Comments, c)
        if err := st.AppendEvent(actorID, "comment.add", c.ID, c); err != nil {
                http.Error(w, err.Error(), http.StatusConflict)
                return
        }
        if err := st.Save(db); err != nil {
                http.Error(w, err.Error(), http.StatusConflict)
                return
        }

        http.Redirect(w, r, "/items/"+itemID, http.StatusSeeOther)
}

const sessionCookieName = "clarity_web_session"

func (s *Server) actorForRequest(r *http.Request) string {
        // Fixed actor override is useful for local-only usage and early automation.
        if strings.TrimSpace(s.cfg.ActorID) != "" {
                return strings.TrimSpace(s.cfg.ActorID)
        }

        c, err := r.Cookie(sessionCookieName)
        if err != nil {
                return ""
        }
        secret, err := loadOrInitSecretKey(s.cfg.Dir)
        if err != nil {
                return ""
        }
        sp, err := verifyToken(secret, c.Value)
        if err != nil || sp.Typ != "session" {
                return ""
        }
        return strings.TrimSpace(sp.Sub)
}

func readyItems(db *store.DB) []model.Item {
        blocked := map[string]bool{}
        for _, d := range db.Deps {
                if d.Type == model.DependencyBlocks {
                        blocked[d.FromItemID] = true
                }
        }

        out := make([]model.Item, 0)
        for _, it := range db.Items {
                if it.Archived {
                        continue
                }
                if it.OnHold {
                        continue
                }
                if blocked[it.ID] {
                        continue
                }
                if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) != "" {
                        continue
                }
                if isEndState(db, it.OutlineID, it.StatusID) {
                        continue
                }
                out = append(out, it)
        }
        return out
}

func agendaItems(db *store.DB, actorID string) []model.Item {
        actorID = strings.TrimSpace(actorID)
        if actorID == "" {
                return nil
        }

        out := make([]model.Item, 0)
        for _, it := range db.Items {
                if it.Archived {
                        continue
                }
                if isEndState(db, it.OutlineID, it.StatusID) {
                        continue
                }
                if it.OwnerActorID == actorID {
                        out = append(out, it)
                        continue
                }
                if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) == actorID {
                        out = append(out, it)
                        continue
                }
        }
        return out
}

func isEndState(db *store.DB, outlineID, statusID string) bool {
        o, ok := db.FindOutline(outlineID)
        if ok && o != nil {
                return statusutil.IsEndState(*o, statusID)
        }
        return statusutil.IsEndState(model.Outline{}, statusID)
}
