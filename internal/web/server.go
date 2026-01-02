package web

import (
        "context"
        "embed"
        "encoding/json"
        "errors"
        "fmt"
        "html/template"
        "io"
        "net/http"
        "net/url"
        "os"
        "path/filepath"
        "sort"
        "strconv"
        "strings"
        "sync"
        "time"

        "clarity-cli/internal/gitrepo"
        "clarity-cli/internal/model"
        "clarity-cli/internal/mutate"
        "clarity-cli/internal/perm"
        "clarity-cli/internal/statusutil"
        "clarity-cli/internal/store"

        "github.com/starfederation/datastar-go/datastar"
)

//go:embed templates/*.html static/*.js static/*.css
var assetsFS embed.FS

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

        autoCommit *gitrepo.DebouncedCommitter
        bc         *resourceBroadcaster
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
        }).ParseFS(assetsFS, "templates/*.html")
        if err != nil {
                return nil, err
        }

        srv := &Server{cfg: cfg, tmpl: tmpl}
        if !cfg.ReadOnly && gitrepo.AutoCommitEnabled() {
                srv.autoCommit = gitrepo.NewDebouncedCommitter(gitrepo.DebouncedCommitterOpts{
                        WorkspaceDir:   cfg.Dir,
                        Debounce:       1 * time.Second,
                        AutoPush:       gitrepo.AutoPushEnabled(),
                        AutoPullRebase: gitrepo.AutoPullRebaseEnabled(),
                })
        }
        srv.bc = newResourceBroadcaster(cfg.Dir)
        go srv.bc.watchLoop()
        return srv, nil
}

func (s *Server) Addr() string { return s.cfg.Addr }

func (s *Server) Handler() http.Handler {
        mux := http.NewServeMux()
        mux.HandleFunc("GET /health", s.handleHealth)
        mux.HandleFunc("GET /events", s.handleWorkspaceEvents)
        mux.HandleFunc("GET /projects/{projectId}/events", s.handleProjectEvents)
        mux.HandleFunc("GET /outlines/{outlineId}/events", s.handleOutlineEvents)
        mux.HandleFunc("GET /items/{itemId}/events", s.handleItemEvents)
        mux.HandleFunc("GET /static/app.css", s.handleAppCSS)
        mux.HandleFunc("GET /static/app.js", s.handleAppJS)
        mux.HandleFunc("GET /static/datastar.js", s.handleDatastarJS)
        mux.HandleFunc("GET /static/outline.js", s.handleOutlineJS)
        mux.HandleFunc("GET /", s.handleHome)
        mux.HandleFunc("GET /login", s.handleLoginGet)
        mux.HandleFunc("POST /login", s.handleLoginPost)
        mux.HandleFunc("GET /verify", s.handleVerifyGet)
        mux.HandleFunc("POST /logout", s.handleLogoutPost)
        mux.HandleFunc("GET /agenda", s.handleAgenda)
        mux.HandleFunc("GET /sync", s.handleSync)
        mux.HandleFunc("POST /sync/init", s.handleSyncInit)
        mux.HandleFunc("POST /sync/remote", s.handleSyncRemote)
        mux.HandleFunc("POST /sync/push-upstream", s.handleSyncPushUpstream)
        mux.HandleFunc("POST /sync/pull", s.handleSyncPull)
        mux.HandleFunc("POST /sync/push", s.handleSyncPush)
        mux.HandleFunc("GET /projects", s.handleProjects)
        mux.HandleFunc("GET /projects/{projectId}", s.handleProject)
        mux.HandleFunc("GET /outlines/{outlineId}", s.handleOutline)
        mux.HandleFunc("POST /outlines/{outlineId}/items", s.handleOutlineItemCreate)
        mux.HandleFunc("POST /outlines/{outlineId}/apply", s.handleOutlineApply)
        mux.HandleFunc("GET /items/{itemId}", s.handleItem)
        mux.HandleFunc("POST /items/{itemId}/edit", s.handleItemEdit)
        mux.HandleFunc("POST /items/{itemId}/comments", s.handleItemCommentAdd)
        mux.HandleFunc("POST /items/{itemId}/worklog", s.handleItemWorklogAdd)
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

func (s *Server) handleDatastarJS(w http.ResponseWriter, r *http.Request) {
        b, err := assetsFS.ReadFile("static/datastar.js")
        if err != nil || len(b) == 0 {
                http.NotFound(w, r)
                return
        }
        w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write(b)
}

func (s *Server) handleAppJS(w http.ResponseWriter, r *http.Request) {
        b, err := assetsFS.ReadFile("static/app.js")
        if err != nil || len(b) == 0 {
                http.NotFound(w, r)
                return
        }
        w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write(b)
}

func (s *Server) handleAppCSS(w http.ResponseWriter, r *http.Request) {
        b, err := assetsFS.ReadFile("static/app.css")
        if err != nil || len(b) == 0 {
                http.NotFound(w, r)
                return
        }
        w.Header().Set("Content-Type", "text/css; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write(b)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok\n"))
}

type resourceKey struct {
        kind string
        id   string
}

func (k resourceKey) String() string {
        kind := strings.TrimSpace(k.kind)
        id := strings.TrimSpace(k.id)
        if id == "" {
                return kind
        }
        return kind + ":" + id
}

type resourceHub struct {
        mu   sync.Mutex
        subs map[chan struct{}]struct{}
}

func newResourceHub() *resourceHub {
        return &resourceHub{subs: map[chan struct{}]struct{}{}}
}

func (h *resourceHub) subscribe() (ch chan struct{}, cancel func()) {
        ch = make(chan struct{}, 8)
        h.mu.Lock()
        h.subs[ch] = struct{}{}
        h.mu.Unlock()
        return ch, func() {
                h.mu.Lock()
                delete(h.subs, ch)
                h.mu.Unlock()
                close(ch)
        }
}

func (h *resourceHub) broadcast() {
        h.mu.Lock()
        for ch := range h.subs {
                select {
                case ch <- struct{}{}:
                default:
                }
        }
        h.mu.Unlock()
}

type resourceBroadcaster struct {
        dir string

        mu      sync.Mutex
        hubs    map[string]*resourceHub
        fp      string
        seen    map[string]struct{}
        seenLRU []string
}

func newResourceBroadcaster(dir string) *resourceBroadcaster {
        return &resourceBroadcaster{
                dir:  filepath.Clean(strings.TrimSpace(dir)),
                hubs: map[string]*resourceHub{},
                seen: map[string]struct{}{},
        }
}

func (b *resourceBroadcaster) hubFor(key resourceKey) *resourceHub {
        k := key.String()
        if k == "" {
                k = "workspace"
        }
        b.mu.Lock()
        h := b.hubs[k]
        if h == nil {
                h = newResourceHub()
                b.hubs[k] = h
        }
        b.mu.Unlock()
        return h
}

func (b *resourceBroadcaster) fingerprint() string {
        // Keep this cheap: only look at canonical content roots.
        type stamp struct {
                modNano int64
                size    int64
        }
        var max stamp

        checkPath := func(p string) {
                st, err := os.Stat(p)
                if err != nil {
                        return
                }
                if st.ModTime().UnixNano() > max.modNano {
                        max.modNano = st.ModTime().UnixNano()
                }
                max.size += st.Size()
        }
        checkDir := func(dir string) {
                ents, err := os.ReadDir(dir)
                if err != nil {
                        return
                }
                for _, ent := range ents {
                        if ent.IsDir() {
                                continue
                        }
                        checkPath(filepath.Join(dir, ent.Name()))
                }
        }

        root := filepath.Clean(b.dir)
        checkDir(filepath.Join(root, "events"))
        checkPath(filepath.Join(root, "meta", "workspace.json"))

        if max.modNano == 0 && max.size == 0 {
                return ""
        }
        return strconv.FormatInt(max.modNano, 10) + ":" + strconv.FormatInt(max.size, 10)
}

func (b *resourceBroadcaster) currentFingerprint() string {
        b.mu.Lock()
        defer b.mu.Unlock()
        return b.fp
}

func (b *resourceBroadcaster) noteSeen(eventID string) bool {
        eventID = strings.TrimSpace(eventID)
        if eventID == "" {
                return false
        }
        b.mu.Lock()
        defer b.mu.Unlock()
        if _, ok := b.seen[eventID]; ok {
                return false
        }
        b.seen[eventID] = struct{}{}
        b.seenLRU = append(b.seenLRU, eventID)
        const capEvents = 1000
        if len(b.seenLRU) > capEvents {
                evict := b.seenLRU[:len(b.seenLRU)-capEvents]
                b.seenLRU = b.seenLRU[len(b.seenLRU)-capEvents:]
                for _, id := range evict {
                        delete(b.seen, id)
                }
        }
        return true
}

func (b *resourceBroadcaster) setFingerprint(fp string) {
        b.mu.Lock()
        b.fp = fp
        b.mu.Unlock()
}

func (b *resourceBroadcaster) watchLoop() {
        lastFP := ""
        t := time.NewTicker(1 * time.Second)
        defer t.Stop()

        for range t.C {
                fp := strings.TrimSpace(b.fingerprint())
                if fp == "" {
                        continue
                }
                if lastFP == "" {
                        lastFP = fp
                        b.setFingerprint(fp)
                        continue
                }
                if fp == lastFP {
                        continue
                }
                lastFP = fp
                b.setFingerprint(fp)

                // Read a tail window and broadcast newly observed events.
                evs, err := store.ReadEventsTail(b.dir, 200)
                if err != nil {
                        continue
                }

                changed := map[resourceKey]struct{}{}
                add := func(k resourceKey) {
                        if strings.TrimSpace(k.String()) == "" {
                                return
                        }
                        changed[k] = struct{}{}
                }

                db, _ := (store.Store{Dir: b.dir}).Load()
                for _, ev := range evs {
                        if !b.noteSeen(ev.ID) {
                                continue
                        }

                        typ := strings.TrimSpace(ev.Type)
                        switch {
                        case strings.HasPrefix(typ, "item."):
                                itemID := strings.TrimSpace(ev.EntityID)
                                if itemID != "" {
                                        add(resourceKey{kind: "item", id: itemID})
                                        if db != nil {
                                                if it, ok := db.FindItem(itemID); ok && it != nil {
                                                        add(resourceKey{kind: "outline", id: it.OutlineID})
                                                        add(resourceKey{kind: "project", id: it.ProjectID})
                                                }
                                        }
                                }
                                add(resourceKey{kind: "workspace"})
                        case strings.HasPrefix(typ, "outline."):
                                add(resourceKey{kind: "outline", id: ev.EntityID})
                                add(resourceKey{kind: "workspace"})
                        case strings.HasPrefix(typ, "project."):
                                add(resourceKey{kind: "project", id: ev.EntityID})
                                add(resourceKey{kind: "workspace"})
                        case strings.HasPrefix(typ, "comment."):
                                // comment.add includes itemId in payload.
                                itemID := ""
                                if p, ok := ev.Payload.(map[string]any); ok {
                                        if id, ok := p["itemId"].(string); ok {
                                                itemID = strings.TrimSpace(id)
                                        }
                                }
                                if itemID != "" {
                                        add(resourceKey{kind: "item", id: itemID})
                                        if db != nil {
                                                if it, ok := db.FindItem(itemID); ok && it != nil {
                                                        add(resourceKey{kind: "outline", id: it.OutlineID})
                                                        add(resourceKey{kind: "project", id: it.ProjectID})
                                                }
                                        }
                                }
                                add(resourceKey{kind: "workspace"})
                        case strings.HasPrefix(typ, "worklog."):
                                itemID := ""
                                if p, ok := ev.Payload.(map[string]any); ok {
                                        if id, ok := p["itemId"].(string); ok {
                                                itemID = strings.TrimSpace(id)
                                        }
                                }
                                if itemID != "" {
                                        add(resourceKey{kind: "item", id: itemID})
                                        if db != nil {
                                                if it, ok := db.FindItem(itemID); ok && it != nil {
                                                        add(resourceKey{kind: "outline", id: it.OutlineID})
                                                        add(resourceKey{kind: "project", id: it.ProjectID})
                                                }
                                        }
                                }
                                add(resourceKey{kind: "workspace"})
                        default:
                                add(resourceKey{kind: "workspace"})
                        }
                }

                for k := range changed {
                        b.hubFor(k).broadcast()
                }
        }
}

func (s *Server) renderTemplate(name string, data any) (string, error) {
        var b strings.Builder
        if err := s.tmpl.ExecuteTemplate(&b, name, data); err != nil {
                return "", err
        }
        return b.String(), nil
}

func (s *Server) writeHTMLTemplate(w http.ResponseWriter, name string, data any) {
        html, err := s.renderTemplate(name, data)
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = io.WriteString(w, html)
}

func (s *Server) serveDatastarStream(w http.ResponseWriter, r *http.Request, key resourceKey, renderMain func() (string, error)) {
        sse := datastar.NewSSE(w, r)

        fp := strings.TrimSpace(s.bc.currentFingerprint())
        _ = sse.MarshalAndPatchSignals(map[string]any{"wsVersion": fp})

        h := s.bc.hubFor(key)
        ch, cancel := h.subscribe()
        defer cancel()

        keepAlive := time.NewTicker(25 * time.Second)
        defer keepAlive.Stop()

        for {
                select {
                case <-sse.Context().Done():
                        return
                case <-keepAlive.C:
                        _ = sse.PatchSignals([]byte(`{}`))
                case <-ch:
                        html, err := renderMain()
                        if err != nil {
                                _ = sse.ExecuteScript(fmt.Sprintf(`console.error(%q)`, err.Error()))
                                continue
                        }
                        if strings.TrimSpace(html) == "" {
                                continue
                        }
                        _ = sse.PatchElements(html, datastar.WithSelector("#clarity-main"), datastar.WithMode(datastar.ElementPatchModeOuter))

                        fp := strings.TrimSpace(s.bc.currentFingerprint())
                        _ = sse.MarshalAndPatchSignals(map[string]any{"wsVersion": fp})
                }
        }
}

func (s *Server) handleWorkspaceEvents(w http.ResponseWriter, r *http.Request) {
        view := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("view")))
        switch view {
        case "home":
                s.serveDatastarStream(w, r, resourceKey{kind: "workspace"}, func() (string, error) {
                        ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
                        defer cancel()

                        st, _ := gitrepo.GetStatus(ctx, s.cfg.Dir)
                        db, err := (store.Store{Dir: s.cfg.Dir}).Load()
                        if err != nil {
                                return "", err
                        }
                        actorID := s.actorForRequest(r)
                        vm := homeVM{
                                Now:       time.Now().Format(time.RFC3339),
                                Workspace: s.cfg.Workspace,
                                Dir:       s.cfg.Dir,
                                ReadOnly:  s.cfg.ReadOnly,
                                ActorID:   actorID,
                                AuthMode:  s.cfg.AuthMode,
                                Git:       st,
                                Projects:  unarchivedProjects(db.Projects),
                                Ready:     readyItems(db),
                                StreamURL: "/events?view=home",
                        }
                        return s.renderTemplate("home_main", vm)
                })
        case "projects":
                s.serveDatastarStream(w, r, resourceKey{kind: "workspace"}, func() (string, error) {
                        db, err := (store.Store{Dir: s.cfg.Dir}).Load()
                        if err != nil {
                                return "", err
                        }
                        vm := projectsVM{
                                Now:       time.Now().Format(time.RFC3339),
                                Workspace: s.cfg.Workspace,
                                Dir:       s.cfg.Dir,
                                Projects:  unarchivedProjects(db.Projects),
                                StreamURL: "/events?view=projects",
                        }
                        return s.renderTemplate("projects_main", vm)
                })
        case "agenda":
                s.serveDatastarStream(w, r, resourceKey{kind: "workspace"}, func() (string, error) {
                        db, err := (store.Store{Dir: s.cfg.Dir}).Load()
                        if err != nil {
                                return "", err
                        }
                        actorID := strings.TrimSpace(s.actorForRequest(r))
                        vm := agendaVM{
                                Now:       time.Now().Format(time.RFC3339),
                                Workspace: s.cfg.Workspace,
                                Dir:       s.cfg.Dir,
                                ActorID:   actorID,
                                Items:     agendaItems(db, actorID),
                                StreamURL: "/events?view=agenda",
                        }
                        return s.renderTemplate("agenda_main", vm)
                })
        case "sync":
                s.serveDatastarStream(w, r, resourceKey{kind: "workspace"}, func() (string, error) {
                        ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
                        defer cancel()
                        st, _ := gitrepo.GetStatus(ctx, s.cfg.Dir)
                        vm := syncVM{
                                Now:       time.Now().Format(time.RFC3339),
                                Workspace: s.cfg.Workspace,
                                Dir:       s.cfg.Dir,
                                ReadOnly:  s.cfg.ReadOnly,
                                Git:       st,
                                Message:   "",
                                StreamURL: "/events?view=sync",
                        }
                        return s.renderTemplate("sync_main", vm)
                })
        default:
                http.Error(w, "missing/invalid view (expected home|projects|agenda|sync)", http.StatusBadRequest)
                return
        }
}

func (s *Server) handleProjectEvents(w http.ResponseWriter, r *http.Request) {
        id := strings.TrimSpace(r.PathValue("projectId"))
        if id == "" {
                http.Error(w, "missing project id", http.StatusBadRequest)
                return
        }
        s.serveDatastarStream(w, r, resourceKey{kind: "project", id: id}, func() (string, error) {
                db, err := (store.Store{Dir: s.cfg.Dir}).Load()
                if err != nil {
                        return "", err
                }
                p, ok := db.FindProject(id)
                if !ok || p == nil || p.Archived {
                        return "", errors.New("project not found")
                }
                vm := projectVM{
                        Now:       time.Now().Format(time.RFC3339),
                        Workspace: s.cfg.Workspace,
                        Dir:       s.cfg.Dir,
                        Project:   *p,
                        Outlines:  unarchivedOutlines(db.Outlines, id),
                        StreamURL: "/projects/" + p.ID + "/events?view=project",
                }
                return s.renderTemplate("project_main", vm)
        })
}

func (s *Server) handleOutlineEvents(w http.ResponseWriter, r *http.Request) {
        id := strings.TrimSpace(r.PathValue("outlineId"))
        if id == "" {
                http.Error(w, "missing outline id", http.StatusBadRequest)
                return
        }
        s.serveDatastarStream(w, r, resourceKey{kind: "outline", id: id}, func() (string, error) {
                db, err := (store.Store{Dir: s.cfg.Dir}).Load()
                if err != nil {
                        return "", err
                }
                o, ok := db.FindOutline(id)
                if !ok || o == nil || o.Archived {
                        return "", errors.New("outline not found")
                }
                actorID := strings.TrimSpace(s.actorForRequest(r))
                useComponent := strings.TrimSpace(s.cfg.ComponentsDir) != ""
                itemsJSON, statusJSON, assigneesJSON, tagsJSON := outlineComponentPayload(db, *o, actorID)

                items := make([]model.Item, 0)
                for _, it := range db.Items {
                        if it.OutlineID == id && !it.Archived {
                                items = append(items, it)
                        }
                }

                vm := outlineVM{
                        Now:                 time.Now().Format(time.RFC3339),
                        Workspace:           s.cfg.Workspace,
                        Dir:                 s.cfg.Dir,
                        ActorID:             actorID,
                        ReadOnly:            s.cfg.ReadOnly,
                        Outline:             *o,
                        StreamURL:           "/outlines/" + o.ID + "/events?view=outline",
                        UseOutlineComponent: useComponent,
                        ItemsJSON:           itemsJSON,
                        StatusLabelsJSON:    statusJSON,
                        AssigneesJSON:       assigneesJSON,
                        TagsJSON:            tagsJSON,
                        Items:               items,
                }
                return s.renderTemplate("outline_main", vm)
        })
}

func (s *Server) handleItemEvents(w http.ResponseWriter, r *http.Request) {
        id := strings.TrimSpace(r.PathValue("itemId"))
        if id == "" {
                http.Error(w, "missing item id", http.StatusBadRequest)
                return
        }
        s.serveDatastarStream(w, r, resourceKey{kind: "item", id: id}, func() (string, error) {
                db, err := (store.Store{Dir: s.cfg.Dir}).Load()
                if err != nil {
                        return "", err
                }
                it, ok := db.FindItem(id)
                if !ok || it == nil || it.Archived {
                        return "", errors.New("item not found")
                }

                comments := db.CommentsForItem(id)
                actorID := strings.TrimSpace(s.actorForRequest(r))
                canEdit := actorID != "" && perm.CanEditItem(db, actorID, it)
                statusDefs := []model.OutlineStatusDef{}
                if o, ok := db.FindOutline(it.OutlineID); ok && o != nil {
                        statusDefs = o.StatusDefs
                }

                var worklog []model.WorklogEntry
                if actorID != "" {
                        for _, w := range db.WorklogForItem(id) {
                                if strings.TrimSpace(w.AuthorID) == actorID {
                                        worklog = append(worklog, w)
                                }
                        }
                }

                vm := itemVM{
                        Now:       time.Now().Format(time.RFC3339),
                        Workspace: s.cfg.Workspace,
                        Dir:       s.cfg.Dir,
                        ReadOnly:  s.cfg.ReadOnly,
                        ActorID:   actorID,
                        Item:      *it,
                        Comments:  comments,
                        ReplyTo:   "",
                        Worklog:   worklog,
                        StreamURL: "/items/" + it.ID + "/events?view=item",

                        CanEdit:        canEdit,
                        StatusDefs:     statusDefs,
                        ErrorMessage:   "",
                        SuccessMessage: "",
                }
                return s.renderTemplate("item_main", vm)
        })
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
        StreamURL string
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

        s.writeHTMLTemplate(w, "home.html", homeVM{
                Now:       time.Now().Format(time.RFC3339),
                Workspace: s.cfg.Workspace,
                Dir:       s.cfg.Dir,
                ReadOnly:  s.cfg.ReadOnly,
                ActorID:   actorID,
                AuthMode:  s.cfg.AuthMode,
                Git:       st,
                Projects:  unarchivedProjects(db.Projects),
                Ready:     ready,
                StreamURL: "/events?view=home",
        })
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
        StreamURL string
}

func (s *Server) handleAgenda(w http.ResponseWriter, r *http.Request) {
        db, err := (store.Store{Dir: s.cfg.Dir}).Load()
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }

        actorID := strings.TrimSpace(s.actorForRequest(r))
        items := agendaItems(db, actorID)

        s.writeHTMLTemplate(w, "agenda.html", agendaVM{
                Now:       time.Now().Format(time.RFC3339),
                Workspace: s.cfg.Workspace,
                Dir:       s.cfg.Dir,
                ActorID:   actorID,
                Items:     items,
                StreamURL: "/events?view=agenda",
        })
}

type syncVM struct {
        Now       string
        Workspace string
        Dir       string
        ReadOnly  bool
        Git       gitrepo.Status
        Message   string
        StreamURL string
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
        defer cancel()

        st, _ := gitrepo.GetStatus(ctx, s.cfg.Dir)
        msg := strings.TrimSpace(r.URL.Query().Get("msg"))

        s.writeHTMLTemplate(w, "sync.html", syncVM{
                Now:       time.Now().Format(time.RFC3339),
                Workspace: s.cfg.Workspace,
                Dir:       s.cfg.Dir,
                ReadOnly:  s.cfg.ReadOnly,
                Git:       st,
                Message:   msg,
                StreamURL: "/events?view=sync",
        })
}

func (s *Server) handleSyncInit(w http.ResponseWriter, r *http.Request) {
        if s.cfg.ReadOnly {
                http.Error(w, "read-only", http.StatusForbidden)
                return
        }

        ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
        defer cancel()

        st, err := gitrepo.GetStatus(ctx, s.cfg.Dir)
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        if st.IsRepo {
                http.Redirect(w, r, "/sync?msg=already%20a%20git%20repo", http.StatusSeeOther)
                return
        }

        if err := gitrepo.Init(ctx, s.cfg.Dir); err != nil {
                http.Error(w, err.Error(), http.StatusConflict)
                return
        }
        http.Redirect(w, r, "/sync?msg=git%20init%20ok", http.StatusSeeOther)
}

func (s *Server) handleSyncRemote(w http.ResponseWriter, r *http.Request) {
        if s.cfg.ReadOnly {
                http.Error(w, "read-only", http.StatusForbidden)
                return
        }
        if err := r.ParseForm(); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
        }
        remoteURL := strings.TrimSpace(r.Form.Get("remoteUrl"))
        remoteName := strings.TrimSpace(r.Form.Get("remoteName"))
        if remoteURL == "" {
                http.Redirect(w, r, "/sync?msg=missing%20remote%20url", http.StatusSeeOther)
                return
        }

        ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
        defer cancel()

        st, err := gitrepo.GetStatus(ctx, s.cfg.Dir)
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        if !st.IsRepo {
                http.Error(w, "not a git repository (init first)", http.StatusBadRequest)
                return
        }
        if st.Unmerged || st.InProgress {
                http.Error(w, "repo has an in-progress merge/rebase; resolve first", http.StatusConflict)
                return
        }

        if err := gitrepo.SetRemoteURL(ctx, s.cfg.Dir, remoteName, remoteURL); err != nil {
                http.Error(w, err.Error(), http.StatusConflict)
                return
        }
        http.Redirect(w, r, "/sync?msg=remote%20set%20ok", http.StatusSeeOther)
}

func (s *Server) handleSyncPushUpstream(w http.ResponseWriter, r *http.Request) {
        if s.cfg.ReadOnly {
                http.Error(w, "read-only", http.StatusForbidden)
                return
        }
        if err := r.ParseForm(); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
        }
        remoteName := strings.TrimSpace(r.Form.Get("remoteName"))

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

        if err := gitrepo.PushSetUpstream(ctx, s.cfg.Dir, remoteName, "HEAD"); err != nil {
                http.Error(w, err.Error(), http.StatusConflict)
                return
        }
        http.Redirect(w, r, "/sync?msg=push%20--set-upstream%20ok", http.StatusSeeOther)
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
        committed := false
        var commitErr error
        if msg == "" {
                db, loadErr := (store.Store{Dir: s.cfg.Dir}).Load()
                actorLabel := strings.TrimSpace(s.actorForRequest(r))
                if loadErr == nil && db != nil && actorLabel != "" {
                        if a, ok := db.FindActor(actorLabel); ok && a != nil && strings.TrimSpace(a.Name) != "" {
                                actorLabel = strings.TrimSpace(a.Name)
                        }
                }
                committed, commitErr = gitrepo.CommitWorkspaceCanonicalAuto(ctx, s.cfg.Dir, actorLabel)
        } else {
                committed, commitErr = gitrepo.CommitWorkspaceCanonical(ctx, s.cfg.Dir, msg)
        }
        if commitErr != nil {
                http.Error(w, commitErr.Error(), http.StatusConflict)
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
        StreamURL string
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
        db, err := (store.Store{Dir: s.cfg.Dir}).Load()
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }

        s.writeHTMLTemplate(w, "projects.html", projectsVM{
                Now:       time.Now().Format(time.RFC3339),
                Workspace: s.cfg.Workspace,
                Dir:       s.cfg.Dir,
                Projects:  unarchivedProjects(db.Projects),
                StreamURL: "/events?view=projects",
        })
}

type projectVM struct {
        Now       string
        Workspace string
        Dir       string
        Project   model.Project
        Outlines  []model.Outline
        StreamURL string
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
        if p.Archived {
                http.NotFound(w, r)
                return
        }

        outlines := unarchivedOutlines(db.Outlines, projectID)

        s.writeHTMLTemplate(w, "project.html", projectVM{
                Now:       time.Now().Format(time.RFC3339),
                Workspace: s.cfg.Workspace,
                Dir:       s.cfg.Dir,
                Project:   *p,
                Outlines:  outlines,
                StreamURL: "/projects/" + p.ID + "/events?view=project",
        })
}

type outlineVM struct {
        Now       string
        Workspace string
        Dir       string
        ActorID   string
        ReadOnly  bool
        Outline   model.Outline
        StreamURL string

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

        s.writeHTMLTemplate(w, "outline.html", outlineVM{
                Now:                 time.Now().Format(time.RFC3339),
                Workspace:           s.cfg.Workspace,
                Dir:                 s.cfg.Dir,
                ActorID:             actorID,
                ReadOnly:            s.cfg.ReadOnly,
                Outline:             *o,
                StreamURL:           "/outlines/" + o.ID + "/events?view=outline",
                UseOutlineComponent: useComponent,
                ItemsJSON:           itemsJSON,
                StatusLabelsJSON:    statusJSON,
                AssigneesJSON:       assigneesJSON,
                TagsJSON:            tagsJSON,
                Items:               items,
        })
}

func (s *Server) handleOutlineItemCreate(w http.ResponseWriter, r *http.Request) {
        if s.cfg.ReadOnly {
                http.Error(w, "read-only", http.StatusForbidden)
                return
        }

        outlineID := strings.TrimSpace(r.PathValue("outlineId"))
        if outlineID == "" {
                http.Error(w, "missing outline id", http.StatusBadRequest)
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
        title := strings.TrimSpace(r.Form.Get("title"))
        if title == "" {
                http.Redirect(w, r, "/outlines/"+outlineID, http.StatusSeeOther)
                return
        }

        st := store.Store{Dir: s.cfg.Dir}
        db, err := st.Load()
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        o, ok := db.FindOutline(outlineID)
        if !ok || o == nil || o.Archived {
                http.NotFound(w, r)
                return
        }
        if _, ok := db.FindActor(actorID); !ok {
                http.Error(w, "unknown actor", http.StatusForbidden)
                return
        }

        now := time.Now().UTC()
        itemID := st.NextID(db, "item")

        statusID := "todo"
        if len(o.StatusDefs) > 0 && strings.TrimSpace(o.StatusDefs[0].ID) != "" {
                statusID = strings.TrimSpace(o.StatusDefs[0].ID)
        }

        rank := nextAppendRank(db, outlineID, nil)
        it := model.Item{
                ID:           itemID,
                ProjectID:    o.ProjectID,
                OutlineID:    o.ID,
                ParentID:     nil,
                Rank:         rank,
                Title:        title,
                StatusID:     statusID,
                Priority:     false,
                OnHold:       false,
                Archived:     false,
                OwnerActorID: actorID,
                CreatedBy:    actorID,
                CreatedAt:    now,
                UpdatedAt:    now,
        }
        db.Items = append(db.Items, it)
        if err := st.AppendEvent(actorID, "item.create", it.ID, it); err != nil {
                http.Error(w, err.Error(), http.StatusConflict)
                return
        }
        if err := st.Save(db); err != nil {
                http.Error(w, err.Error(), http.StatusConflict)
                return
        }
        if s.autoCommit != nil {
                actorLabel := strings.TrimSpace(actorID)
                if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
                        actorLabel = strings.TrimSpace(a.Name)
                }
                s.autoCommit.Notify(actorLabel)
        }

        http.Redirect(w, r, "/outlines/"+outlineID, http.StatusSeeOther)
}

func nextAppendRank(db *store.DB, outlineID string, parentID *string) string {
        var siblings []*model.Item
        for i := range db.Items {
                it := &db.Items[i]
                if it.OutlineID != outlineID {
                        continue
                }
                if it.Archived {
                        continue
                }
                if !sameParentWeb(it.ParentID, parentID) {
                        continue
                }
                siblings = append(siblings, it)
        }
        store.SortItemsByRankOrder(siblings)
        if len(siblings) == 0 {
                r, err := store.RankInitial()
                if err != nil || strings.TrimSpace(r) == "" {
                        return "h"
                }
                return r
        }
        last := strings.TrimSpace(siblings[len(siblings)-1].Rank)
        r, err := store.RankAfter(last)
        if err != nil || strings.TrimSpace(r) == "" {
                if last == "" {
                        return "h"
                }
                return last + "0"
        }
        return r
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
                        // Keep stable indices for status-<idx> mapping.
                        lbl = strings.TrimSpace(sd.ID)
                }
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

type outlineApplyReq struct {
        Type   string          `json:"type"`
        Detail json.RawMessage `json:"detail"`
}

type outlineApplyResp struct {
        Items        json.RawMessage `json:"items"`
        StatusLabels json.RawMessage `json:"statusLabels"`
        Assignees    json.RawMessage `json:"assignees"`
        Tags         json.RawMessage `json:"tags"`
}

func (s *Server) handleOutlineApply(w http.ResponseWriter, r *http.Request) {
        if s.cfg.ReadOnly {
                http.Error(w, "read-only", http.StatusForbidden)
                return
        }

        outlineID := strings.TrimSpace(r.PathValue("outlineId"))
        if outlineID == "" {
                http.Error(w, "missing outline id", http.StatusBadRequest)
                return
        }

        actorID := strings.TrimSpace(s.actorForRequest(r))
        if actorID == "" {
                http.Error(w, "missing actor (start server with --actor, set currentActorId, or /login in dev auth mode)", http.StatusUnauthorized)
                return
        }

        var req outlineApplyReq
        dec := json.NewDecoder(r.Body)
        dec.DisallowUnknownFields()
        if err := dec.Decode(&req); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
        }
        req.Type = strings.TrimSpace(req.Type)
        if req.Type == "" {
                http.Error(w, "missing type", http.StatusBadRequest)
                return
        }

        st := store.Store{Dir: s.cfg.Dir}
        db, err := st.Load()
        if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        o, ok := db.FindOutline(outlineID)
        if !ok || o == nil || o.Archived {
                http.NotFound(w, r)
                return
        }
        if _, ok := db.FindActor(actorID); !ok {
                http.Error(w, "unknown actor", http.StatusForbidden)
                return
        }

        changed := false
        now := time.Now().UTC()

        switch req.Type {
        case "outline:edit:save":
                var d struct {
                        ID      string `json:"id"`
                        NewText string `json:"newText"`
                }
                if err := json.Unmarshal(req.Detail, &d); err != nil {
                        http.Error(w, err.Error(), http.StatusBadRequest)
                        return
                }
                itemID := strings.TrimSpace(d.ID)
                title := strings.TrimSpace(d.NewText)
                if itemID == "" || title == "" {
                        http.Error(w, "missing id or newText", http.StatusBadRequest)
                        return
                }
                it, ok := db.FindItem(itemID)
                if !ok || it == nil || it.Archived || it.OutlineID != outlineID {
                        http.NotFound(w, r)
                        return
                }
                if !perm.CanEditItem(db, actorID, it) {
                        http.Error(w, "owner-only", http.StatusForbidden)
                        return
                }
                if strings.TrimSpace(it.Title) != title {
                        it.Title = title
                        it.UpdatedAt = now
                        if err := st.AppendEvent(actorID, "item.set_title", it.ID, map[string]any{"title": it.Title}); err != nil {
                                http.Error(w, err.Error(), http.StatusConflict)
                                return
                        }
                        changed = true
                }

        case "outline:toggle":
                var d struct {
                        ID string `json:"id"`
                        To string `json:"to"`
                }
                if err := json.Unmarshal(req.Detail, &d); err != nil {
                        http.Error(w, err.Error(), http.StatusBadRequest)
                        return
                }
                itemID := strings.TrimSpace(d.ID)
                if itemID == "" {
                        http.Error(w, "missing id", http.StatusBadRequest)
                        return
                }
                it, ok := db.FindItem(itemID)
                if !ok || it == nil || it.Archived || it.OutlineID != outlineID {
                        http.NotFound(w, r)
                        return
                }
                statusID, err := statusIDFromOutlineToggle(*o, strings.TrimSpace(d.To))
                if err != nil {
                        http.Error(w, err.Error(), http.StatusBadRequest)
                        return
                }
                res, err := mutate.SetItemStatus(db, actorID, it.ID, statusID, nil)
                if err != nil {
                        http.Error(w, err.Error(), http.StatusConflict)
                        return
                }
                if res.Changed {
                        it.UpdatedAt = now
                        if err := st.AppendEvent(actorID, "item.set_status", it.ID, res.EventPayload); err != nil {
                                http.Error(w, err.Error(), http.StatusConflict)
                                return
                        }
                        changed = true
                }

        case "outline:move":
                var d struct {
                        ID       string  `json:"id"`
                        ParentID *string `json:"parentId"`
                        BeforeID *string `json:"beforeId"`
                        AfterID  *string `json:"afterId"`
                }
                if err := json.Unmarshal(req.Detail, &d); err != nil {
                        http.Error(w, err.Error(), http.StatusBadRequest)
                        return
                }
                itemID := strings.TrimSpace(d.ID)
                if itemID == "" {
                        http.Error(w, "missing id", http.StatusBadRequest)
                        return
                }
                parentID := ""
                if d.ParentID != nil {
                        parentID = strings.TrimSpace(*d.ParentID)
                }
                before := ""
                after := ""
                if d.BeforeID != nil {
                        before = strings.TrimSpace(*d.BeforeID)
                }
                if d.AfterID != nil {
                        after = strings.TrimSpace(*d.AfterID)
                }
                if err := applyItemMoveOrReparent(db, actorID, itemID, outlineID, parentID, before, after, now, st); err != nil {
                        http.Error(w, err.Error(), http.StatusConflict)
                        return
                }
                changed = true

        default:
                http.Error(w, "unsupported type: "+req.Type, http.StatusBadRequest)
                return
        }

        if changed {
                if err := st.Save(db); err != nil {
                        http.Error(w, err.Error(), http.StatusConflict)
                        return
                }
                if s.autoCommit != nil {
                        actorLabel := strings.TrimSpace(actorID)
                        if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
                                actorLabel = strings.TrimSpace(a.Name)
                        }
                        s.autoCommit.Notify(actorLabel)
                }
        }

        itemsJSON, statusJSON, assigneesJSON, tagsJSON := outlineComponentPayload(db, *o, actorID)
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _ = json.NewEncoder(w).Encode(outlineApplyResp{
                Items:        json.RawMessage([]byte(itemsJSON)),
                StatusLabels: json.RawMessage([]byte(statusJSON)),
                Assignees:    json.RawMessage([]byte(assigneesJSON)),
                Tags:         json.RawMessage([]byte(tagsJSON)),
        })
}

func statusIDFromOutlineToggle(o model.Outline, to string) (string, error) {
        to = strings.TrimSpace(to)
        if to == "" || strings.EqualFold(to, "none") {
                return "", nil
        }
        if strings.HasPrefix(to, "status-") {
                idxStr := strings.TrimSpace(strings.TrimPrefix(to, "status-"))
                idx, err := strconv.Atoi(idxStr)
                if err != nil || idx < 0 {
                        return "", errors.New("invalid status index")
                }
                if idx >= len(o.StatusDefs) {
                        return "", errors.New("status index out of range")
                }
                return strings.TrimSpace(o.StatusDefs[idx].ID), nil
        }
        // Fallback: treat as a status id-like string.
        return statusutil.NormalizeStatusID(to)
}

func applyItemMoveOrReparent(db *store.DB, actorID, itemID, outlineID, parentID, before, after string, now time.Time, st store.Store) error {
        it, ok := db.FindItem(itemID)
        if !ok || it == nil {
                return errors.New("item not found")
        }
        if it.Archived || it.OutlineID != outlineID {
                return errors.New("item not found")
        }
        if !perm.CanEditItem(db, actorID, it) {
                return errors.New("owner-only")
        }

        // Normalize parent pointer.
        var newParent *string
        if strings.TrimSpace(parentID) != "" {
                pid := strings.TrimSpace(parentID)
                pit, ok := db.FindItem(pid)
                if !ok || pit == nil || pit.Archived || pit.OutlineID != outlineID {
                        return errors.New("parent not found in outline")
                }
                if pid == it.ID || isAncestor(db, it.ID, pid) {
                        return errors.New("cannot set parent (cycle)")
                }
                newParent = &pid
        }

        refID := ""
        mode := ""
        if strings.TrimSpace(after) != "" {
                refID = strings.TrimSpace(after)
                mode = "after"
        } else if strings.TrimSpace(before) != "" {
                refID = strings.TrimSpace(before)
                mode = "before"
        }

        sibs := siblingItemsWeb(db, outlineID, newParent, itemID)
        insertAt := len(sibs)
        if refID != "" {
                refIdx := indexOfItemWeb(sibs, refID)
                if refIdx >= 0 {
                        insertAt = refIdx
                        if mode == "after" {
                                insertAt = refIdx + 1
                        }
                }
                if insertAt < 0 {
                        insertAt = 0
                }
                if insertAt > len(sibs) {
                        insertAt = len(sibs)
                }
        }

        destFull := append([]*model.Item{}, sibs...)
        destFull = append(destFull, it)
        res, err := store.PlanReorderRanks(destFull, it.ID, insertAt)
        if err != nil {
                return err
        }

        for id, r := range res.RankByID {
                x, ok := db.FindItem(id)
                if !ok || x == nil {
                        continue
                }
                if strings.TrimSpace(x.Rank) == strings.TrimSpace(r) {
                        continue
                }
                x.Rank = r
                x.UpdatedAt = now
        }
        prevParent := ""
        if it.ParentID != nil {
                prevParent = strings.TrimSpace(*it.ParentID)
        }
        nextParent := ""
        if newParent != nil {
                nextParent = strings.TrimSpace(*newParent)
        }
        it.ParentID = newParent
        it.UpdatedAt = now

        if prevParent != nextParent {
                payload := map[string]any{"parent": parentID, "before": before, "after": after, "rank": strings.TrimSpace(it.Rank)}
                if res.UsedFallback && len(res.RankByID) > 1 {
                        rebalance := map[string]string{}
                        for id, r := range res.RankByID {
                                if id == it.ID {
                                        continue
                                }
                                rebalance[id] = r
                        }
                        if len(rebalance) > 0 {
                                payload["rebalance"] = rebalance
                                payload["rebalanceCount"] = len(rebalance)
                        }
                }
                return st.AppendEvent(actorID, "item.set_parent", it.ID, payload)
        }

        if strings.TrimSpace(before) == "" && strings.TrimSpace(after) == "" {
                return nil
        }

        payload := map[string]any{"before": before, "after": after, "rank": strings.TrimSpace(it.Rank)}
        if res.UsedFallback && len(res.RankByID) > 1 {
                rebalance := map[string]string{}
                for id, r := range res.RankByID {
                        if id == it.ID {
                                continue
                        }
                        rebalance[id] = r
                }
                if len(rebalance) > 0 {
                        payload["rebalance"] = rebalance
                        payload["rebalanceCount"] = len(rebalance)
                }
        }
        return st.AppendEvent(actorID, "item.move", it.ID, payload)
}

func siblingItemsWeb(db *store.DB, outlineID string, parentID *string, excludeID string) []*model.Item {
        var out []*model.Item
        for i := range db.Items {
                it := &db.Items[i]
                if it.OutlineID != outlineID {
                        continue
                }
                if it.Archived {
                        continue
                }
                if strings.TrimSpace(it.ID) == strings.TrimSpace(excludeID) {
                        continue
                }
                if !sameParentWeb(it.ParentID, parentID) {
                        continue
                }
                out = append(out, it)
        }
        store.SortItemsByRankOrder(out)
        return out
}

func sameParentWeb(a, b *string) bool {
        if a == nil && b == nil {
                return true
        }
        if a == nil || b == nil {
                return false
        }
        return strings.TrimSpace(*a) == strings.TrimSpace(*b)
}

func indexOfItemWeb(items []*model.Item, id string) int {
        id = strings.TrimSpace(id)
        if id == "" {
                return -1
        }
        for i, it := range items {
                if strings.TrimSpace(it.ID) == id {
                        return i
                }
        }
        return -1
}

func isAncestor(db *store.DB, itemID, ancestorID string) bool {
        itemID = strings.TrimSpace(itemID)
        ancestorID = strings.TrimSpace(ancestorID)
        if itemID == "" || ancestorID == "" || itemID == ancestorID {
                return false
        }
        cur, ok := db.FindItem(ancestorID)
        if !ok || cur == nil {
                return false
        }
        for {
                if cur.ParentID == nil {
                        return false
                }
                pid := strings.TrimSpace(*cur.ParentID)
                if pid == "" {
                        return false
                }
                if pid == itemID {
                        return true
                }
                next, ok := db.FindItem(pid)
                if !ok || next == nil {
                        return false
                }
                cur = next
        }
}

type itemVM struct {
        Now       string
        Workspace string
        Dir       string
        ReadOnly  bool
        ActorID   string
        Item      model.Item
        Comments  []model.Comment
        ReplyTo   string
        Worklog   []model.WorklogEntry
        StreamURL string

        CanEdit        bool
        StatusDefs     []model.OutlineStatusDef
        ErrorMessage   string
        SuccessMessage string
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
        if it.Archived {
                http.NotFound(w, r)
                return
        }

        comments := db.CommentsForItem(itemID)
        actorID := strings.TrimSpace(s.actorForRequest(r))
        canEdit := actorID != "" && perm.CanEditItem(db, actorID, it)
        statusDefs := []model.OutlineStatusDef{}
        if o, ok := db.FindOutline(it.OutlineID); ok && o != nil {
                statusDefs = o.StatusDefs
        }
        replyTo := strings.TrimSpace(r.URL.Query().Get("replyTo"))
        if replyTo != "" {
                ok := false
                for _, c := range comments {
                        if c.ID == replyTo {
                                ok = true
                                break
                        }
                }
                if !ok {
                        replyTo = ""
                }
        }
        errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
        okMsg := strings.TrimSpace(r.URL.Query().Get("ok"))

        var worklog []model.WorklogEntry
        if actorID != "" {
                for _, w := range db.WorklogForItem(itemID) {
                        if strings.TrimSpace(w.AuthorID) == actorID {
                                worklog = append(worklog, w)
                        }
                }
        }

        s.writeHTMLTemplate(w, "item.html", itemVM{
                Now:            time.Now().Format(time.RFC3339),
                Workspace:      s.cfg.Workspace,
                Dir:            s.cfg.Dir,
                ReadOnly:       s.cfg.ReadOnly,
                ActorID:        actorID,
                Item:           *it,
                Comments:       comments,
                ReplyTo:        replyTo,
                Worklog:        worklog,
                StreamURL:      "/items/" + it.ID + "/events?view=item",
                CanEdit:        canEdit,
                StatusDefs:     statusDefs,
                ErrorMessage:   errMsg,
                SuccessMessage: okMsg,
        })
}

func (s *Server) handleItemEdit(w http.ResponseWriter, r *http.Request) {
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

        title := strings.TrimSpace(r.Form.Get("title"))
        description := r.Form.Get("description")
        statusID := strings.TrimSpace(r.Form.Get("status"))

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
        it, ok := db.FindItem(itemID)
        if !ok || it == nil || it.Archived {
                http.NotFound(w, r)
                return
        }
        if !perm.CanEditItem(db, actorID, it) {
                http.Error(w, "owner-only", http.StatusForbidden)
                return
        }

        now := time.Now().UTC()
        changed := false

        if title != "" && strings.TrimSpace(it.Title) != title {
                it.Title = title
                it.UpdatedAt = now
                if err := st.AppendEvent(actorID, "item.set_title", it.ID, map[string]any{"title": it.Title}); err != nil {
                        http.Error(w, err.Error(), http.StatusConflict)
                        return
                }
                changed = true
        }

        if strings.TrimSpace(it.Description) != strings.TrimSpace(description) {
                it.Description = description
                it.UpdatedAt = now
                if err := st.AppendEvent(actorID, "item.set_description", it.ID, map[string]any{"description": it.Description}); err != nil {
                        http.Error(w, err.Error(), http.StatusConflict)
                        return
                }
                changed = true
        }

        // Status (optional).
        if statusID != "" || it.StatusID != "" {
                o, ok := db.FindOutline(it.OutlineID)
                if !ok || o == nil || o.Archived {
                        http.Error(w, "outline not found", http.StatusConflict)
                        return
                }
                res, err := mutate.SetItemStatus(db, actorID, it.ID, statusID, nil)
                if err != nil {
                        // Common case: note required.
                        http.Redirect(w, r, "/items/"+itemID+"?err="+urlQueryEscape(err.Error()), http.StatusSeeOther)
                        return
                }
                if res.Changed {
                        it.UpdatedAt = now
                        if err := st.AppendEvent(actorID, "item.set_status", it.ID, res.EventPayload); err != nil {
                                http.Error(w, err.Error(), http.StatusConflict)
                                return
                        }
                        changed = true
                }
        }

        if changed {
                if err := st.Save(db); err != nil {
                        http.Error(w, err.Error(), http.StatusConflict)
                        return
                }
                if s.autoCommit != nil {
                        actorLabel := strings.TrimSpace(actorID)
                        if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
                                actorLabel = strings.TrimSpace(a.Name)
                        }
                        s.autoCommit.Notify(actorLabel)
                }
        }

        http.Redirect(w, r, "/items/"+itemID+"?ok=updated", http.StatusSeeOther)
}

func urlQueryEscape(s string) string {
        return url.QueryEscape(s)
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
        replyTo := strings.TrimSpace(r.Form.Get("replyTo"))
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
        if replyTo != "" {
                ok := false
                for _, c := range db.CommentsForItem(itemID) {
                        if c.ID == replyTo {
                                ok = true
                                break
                        }
                }
                if !ok {
                        replyTo = ""
                }
        }

        c := model.Comment{
                ID:        st.NextID(db, "cmt"),
                ItemID:    itemID,
                AuthorID:  actorID,
                Body:      body,
                CreatedAt: time.Now().UTC(),
        }
        if replyTo != "" {
                c.ReplyToCommentID = &replyTo
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
        if s.autoCommit != nil {
                actorLabel := strings.TrimSpace(actorID)
                if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
                        actorLabel = strings.TrimSpace(a.Name)
                }
                s.autoCommit.Notify(actorLabel)
        }

        http.Redirect(w, r, "/items/"+itemID, http.StatusSeeOther)
}

func (s *Server) handleItemWorklogAdd(w http.ResponseWriter, r *http.Request) {
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

        wle := model.WorklogEntry{
                ID:        st.NextID(db, "wlg"),
                ItemID:    itemID,
                AuthorID:  actorID,
                Body:      body,
                CreatedAt: time.Now().UTC(),
        }
        db.Worklog = append(db.Worklog, wle)
        if err := st.AppendEvent(actorID, "worklog.add", wle.ID, wle); err != nil {
                http.Error(w, err.Error(), http.StatusConflict)
                return
        }
        if err := st.Save(db); err != nil {
                http.Error(w, err.Error(), http.StatusConflict)
                return
        }
        if s.autoCommit != nil {
                actorLabel := strings.TrimSpace(actorID)
                if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
                        actorLabel = strings.TrimSpace(a.Name)
                }
                s.autoCommit.Notify(actorLabel)
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
