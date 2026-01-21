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

	// OutlineMode selects the outline UI strategy.
	// - native: server-rendered HTML + keyboard JS (preferred)
	// - component: clarity-outline web component driven by Datastar signals
	OutlineMode string // native|component
}

type Server struct {
	mu   sync.RWMutex
	cfg  ServerConfig
	tmpl *template.Template

	autoCommit *gitrepo.DebouncedCommitter
	bc         *resourceBroadcaster
}

func (s *Server) cfgSnapshot() ServerConfig {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	return cfg
}

func (s *Server) dir() string {
	s.mu.RLock()
	d := s.cfg.Dir
	s.mu.RUnlock()
	return d
}

func (s *Server) workspaceName() string {
	s.mu.RLock()
	w := s.cfg.Workspace
	s.mu.RUnlock()
	return w
}

func (s *Server) readOnly() bool {
	s.mu.RLock()
	ro := s.cfg.ReadOnly
	s.mu.RUnlock()
	return ro
}

func (s *Server) authMode() string {
	s.mu.RLock()
	a := s.cfg.AuthMode
	s.mu.RUnlock()
	return a
}

func (s *Server) outlineMode() string {
	s.mu.RLock()
	m := s.cfg.OutlineMode
	s.mu.RUnlock()
	return m
}

func (s *Server) componentsDir() string {
	s.mu.RLock()
	d := s.cfg.ComponentsDir
	s.mu.RUnlock()
	return d
}

func (s *Server) broadcaster() *resourceBroadcaster {
	s.mu.RLock()
	b := s.bc
	s.mu.RUnlock()
	return b
}

func (s *Server) committer() *gitrepo.DebouncedCommitter {
	s.mu.RLock()
	c := s.autoCommit
	s.mu.RUnlock()
	return c
}

type baseVM struct {
	Now       string
	Workspace string
	Dir       string
	ReadOnly  bool
	ActorID   string
	AuthMode  string
	Git       gitrepo.Status
	StreamURL string
}

type navOptionsVM struct {
	Recent []navRecentItem `json:"recent"`
}

type navRecentItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (s *Server) baseVMForRequest(r *http.Request, streamURL string) baseVM {
	ctx, cancel := context.WithTimeout(r.Context(), 1200*time.Millisecond)
	defer cancel()

	cfg := s.cfgSnapshot()
	st, _ := gitrepo.GetStatus(ctx, cfg.Dir)

	return baseVM{
		Now:       time.Now().Format(time.RFC3339),
		Workspace: cfg.Workspace,
		Dir:       cfg.Dir,
		ReadOnly:  cfg.ReadOnly,
		ActorID:   strings.TrimSpace(s.actorForRequest(r)),
		AuthMode:  cfg.AuthMode,
		Git:       st,
		StreamURL: streamURL,
	}
}

func NewServer(cfg ServerConfig) (*Server, error) {
	cfg.Addr = strings.TrimSpace(cfg.Addr)
	cfg.Dir = strings.TrimSpace(cfg.Dir)
	cfg.Workspace = strings.TrimSpace(cfg.Workspace)
	cfg.ActorID = strings.TrimSpace(cfg.ActorID)
	cfg.AuthMode = strings.ToLower(strings.TrimSpace(cfg.AuthMode))
	cfg.ComponentsDir = strings.TrimSpace(cfg.ComponentsDir)
	cfg.OutlineMode = strings.ToLower(strings.TrimSpace(cfg.OutlineMode))
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
	if cfg.OutlineMode == "" {
		cfg.OutlineMode = "native"
	}
	if cfg.OutlineMode != "native" && cfg.OutlineMode != "component" {
		return nil, errors.New("web: invalid outline mode (expected native|component)")
	}

	// Ensure repo hygiene so derived/local-only `.clarity/` state doesn't keep the workspace "dirty".
	// This also makes web auth state safe to write locally without showing up in `git status`.
	root := workspaceRootFromDir(cfg.Dir)
	if root != "" {
		if _, err := store.EnsureGitignoreHasClarityIgnores(filepath.Join(root, ".gitignore")); err != nil {
			return nil, err
		}
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
			Debounce:       8 * time.Second,
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
	mux.HandleFunc("GET /nav/options", s.handleNavOptions)
	mux.HandleFunc("GET /projects/{projectId}/events", s.handleProjectEvents)
	mux.HandleFunc("GET /outlines/{outlineId}/events", s.handleOutlineEvents)
	mux.HandleFunc("GET /items/{itemId}/events", s.handleItemEvents)
	mux.HandleFunc("GET /static/app.css", s.handleAppCSS)
	mux.HandleFunc("GET /static/themes.css", s.handleThemesCSS)
	mux.HandleFunc("GET /static/app.js", s.handleAppJS)
	mux.HandleFunc("GET /static/datastar.js", s.handleDatastarJS)
	mux.HandleFunc("GET /static/outline.js", s.handleOutlineJS)
	mux.HandleFunc("GET /", s.handleHome)
	mux.HandleFunc("GET /login", s.handleLoginGet)
	mux.HandleFunc("POST /login", s.handleLoginPost)
	mux.HandleFunc("GET /verify", s.handleVerifyGet)
	mux.HandleFunc("POST /logout", s.handleLogoutPost)
	mux.HandleFunc("GET /agenda", s.handleAgenda)
	mux.HandleFunc("GET /capture/options", s.handleCaptureOptions)
	mux.HandleFunc("POST /capture", s.handleCaptureCreate)
	mux.HandleFunc("POST /capture/templates", s.handleCaptureTemplateUpsert)
	mux.HandleFunc("DELETE /capture/templates/{keyPath}", s.handleCaptureTemplateDelete)
	mux.HandleFunc("GET /sync", s.handleSync)
	mux.HandleFunc("POST /sync/init", s.handleSyncInit)
	mux.HandleFunc("POST /sync/remote", s.handleSyncRemote)
	mux.HandleFunc("POST /sync/push-upstream", s.handleSyncPushUpstream)
	mux.HandleFunc("POST /sync/pull", s.handleSyncPull)
	mux.HandleFunc("POST /sync/push", s.handleSyncPush)
	mux.HandleFunc("GET /workspaces", s.handleWorkspaces)
	mux.HandleFunc("POST /workspaces/use", s.handleWorkspacesUse)
	mux.HandleFunc("POST /workspaces/new", s.handleWorkspacesNew)
	mux.HandleFunc("POST /workspaces/rename", s.handleWorkspacesRename)
	mux.HandleFunc("GET /archived", s.handleArchived)
	mux.HandleFunc("GET /projects", s.handleProjects)
	mux.HandleFunc("POST /projects", s.handleProjectCreate)
	mux.HandleFunc("POST /projects/{projectId}/rename", s.handleProjectRename)
	mux.HandleFunc("POST /projects/{projectId}/archive", s.handleProjectArchive)
	mux.HandleFunc("POST /projects/{projectId}/outlines", s.handleOutlineCreate)
	mux.HandleFunc("GET /projects/{projectId}", s.handleProject)
	mux.HandleFunc("GET /outlines/{outlineId}", s.handleOutline)
	mux.HandleFunc("GET /outlines/{outlineId}/meta", s.handleOutlineMeta)
	mux.HandleFunc("POST /outlines/{outlineId}/rename", s.handleOutlineRename)
	mux.HandleFunc("POST /outlines/{outlineId}/archive", s.handleOutlineArchive)
	mux.HandleFunc("POST /outlines/{outlineId}/description", s.handleOutlineSetDescription)
	mux.HandleFunc("POST /outlines/{outlineId}/statuses/add", s.handleOutlineStatusAdd)
	mux.HandleFunc("POST /outlines/{outlineId}/statuses/update", s.handleOutlineStatusUpdate)
	mux.HandleFunc("POST /outlines/{outlineId}/statuses/remove", s.handleOutlineStatusRemove)
	mux.HandleFunc("POST /outlines/{outlineId}/statuses/reorder", s.handleOutlineStatusReorder)
	mux.HandleFunc("POST /outlines/{outlineId}/items", s.handleOutlineItemCreate)
	mux.HandleFunc("POST /outlines/{outlineId}/apply", s.handleOutlineApply)
	mux.HandleFunc("GET /items/{itemId}", s.handleItem)
	mux.HandleFunc("GET /items/{itemId}/preview", s.handleItemPreview)
	mux.HandleFunc("GET /items/{itemId}/meta", s.handleItemMeta)
	mux.HandleFunc("POST /items/{itemId}/edit", s.handleItemEdit)
	mux.HandleFunc("POST /items/{itemId}/comments", s.handleItemCommentAdd)
	mux.HandleFunc("POST /items/{itemId}/worklog", s.handleItemWorklogAdd)
	return mux
}

func redirectBack(w http.ResponseWriter, r *http.Request, fallback string) {
	ref := strings.TrimSpace(r.Header.Get("Referer"))
	if ref != "" {
		http.Redirect(w, r, ref, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fallback, http.StatusSeeOther)
}

func (s *Server) handleOutlineJS(w http.ResponseWriter, r *http.Request) {
	dir := strings.TrimSpace(s.componentsDir())
	if dir == "" {
		// Avoid noisy 404s when templates include the component bundle unconditionally.
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.WriteHeader(http.StatusNoContent)
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

func (s *Server) handleThemesCSS(w http.ResponseWriter, r *http.Request) {
	b, err := assetsFS.ReadFile("static/themes.css")
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

	stopOnce sync.Once
	stopCh   chan struct{}
}

func newResourceBroadcaster(dir string) *resourceBroadcaster {
	return &resourceBroadcaster{
		dir:    filepath.Clean(strings.TrimSpace(dir)),
		hubs:   map[string]*resourceHub{},
		seen:   map[string]struct{}{},
		stopCh: make(chan struct{}),
	}
}

func (b *resourceBroadcaster) Stop() {
	if b == nil {
		return
	}
	b.stopOnce.Do(func() {
		close(b.stopCh)
	})
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

	for {
		select {
		case <-b.stopCh:
			return
		case <-t.C:
		}

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

func (s *Server) serveDatastarElementsStream(w http.ResponseWriter, r *http.Request, key resourceKey, selector string, mode datastar.ElementPatchMode, render func() (string, error)) {
	sse := datastar.NewSSE(w, r)

	bc := s.broadcaster()
	fp := ""
	if bc != nil {
		fp = strings.TrimSpace(bc.currentFingerprint())
	}
	_ = sse.MarshalAndPatchSignals(map[string]any{"wsVersion": fp})

	h := bc.hubFor(key)
	ch, cancel := h.subscribe()
	defer cancel()

	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()

	selector = strings.TrimSpace(selector)
	if selector == "" {
		selector = "#clarity-main"
	}

	for {
		select {
		case <-sse.Context().Done():
			return
		case <-keepAlive.C:
			_ = sse.PatchSignals([]byte(`{}`))
		case <-ch:
			html, err := render()
			if err != nil {
				_ = sse.ExecuteScript(fmt.Sprintf(`console.error(%q)`, err.Error()))
				continue
			}
			if strings.TrimSpace(html) == "" {
				continue
			}
			_ = sse.PatchElements(html, datastar.WithSelector(selector), datastar.WithMode(mode))

			fp := ""
			bc := s.broadcaster()
			if bc != nil {
				fp = strings.TrimSpace(bc.currentFingerprint())
			}
			_ = sse.MarshalAndPatchSignals(map[string]any{"wsVersion": fp})
		}
	}
}

func (s *Server) serveDatastarStream(w http.ResponseWriter, r *http.Request, key resourceKey, renderMain func() (string, error)) {
	s.serveDatastarElementsStream(w, r, key, "#clarity-main", datastar.ElementPatchModeOuter, renderMain)
}

func (s *Server) serveDatastarSignalsStream(w http.ResponseWriter, r *http.Request, key resourceKey, renderSignals func() (map[string]any, error)) {
	sse := datastar.NewSSE(w, r)

	bc := s.broadcaster()
	fp := ""
	if bc != nil {
		fp = strings.TrimSpace(bc.currentFingerprint())
	}
	_ = sse.MarshalAndPatchSignals(map[string]any{"wsVersion": fp})

	// Initial state snapshot.
	if sig, err := renderSignals(); err == nil && sig != nil {
		sig["wsVersion"] = fp
		_ = sse.MarshalAndPatchSignals(sig)
	}

	h := bc.hubFor(key)
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
			sig, err := renderSignals()
			if err != nil {
				_ = sse.ExecuteScript(fmt.Sprintf(`console.error(%q)`, err.Error()))
				continue
			}
			if sig == nil {
				sig = map[string]any{}
			}
			fp := ""
			bc := s.broadcaster()
			if bc != nil {
				fp = strings.TrimSpace(bc.currentFingerprint())
			}
			sig["wsVersion"] = fp
			_ = sse.MarshalAndPatchSignals(sig)
		}
	}
}

func (s *Server) handleWorkspaceEvents(w http.ResponseWriter, r *http.Request) {
	view := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("view")))
	switch view {
	case "home":
		s.serveDatastarStream(w, r, resourceKey{kind: "workspace"}, func() (string, error) {
			db, err := (store.Store{Dir: s.dir()}).Load()
			if err != nil {
				return "", err
			}
			vm := homeVM{
				baseVM:   s.baseVMForRequest(r, "/events?view=home"),
				Projects: unarchivedProjects(db.Projects),
				Ready:    readyItems(db),
			}
			return s.renderTemplate("home_main", vm)
		})
	case "projects":
		s.serveDatastarStream(w, r, resourceKey{kind: "workspace"}, func() (string, error) {
			db, err := (store.Store{Dir: s.dir()}).Load()
			if err != nil {
				return "", err
			}
			vm := projectsVM{
				baseVM:   s.baseVMForRequest(r, "/events?view=projects"),
				Projects: unarchivedProjects(db.Projects),
			}
			return s.renderTemplate("projects_main", vm)
		})
	case "agenda":
		s.serveDatastarStream(w, r, resourceKey{kind: "workspace"}, func() (string, error) {
			db, err := (store.Store{Dir: s.dir()}).Load()
			if err != nil {
				return "", err
			}
			actorID := strings.TrimSpace(s.actorForRequest(r))
			vm := agendaVM{
				baseVM: s.baseVMForRequest(r, "/events?view=agenda"),
				Rows:   agendaRowsWeb(db, actorID),
			}
			vm.ActorID = actorID
			return s.renderTemplate("agenda_main", vm)
		})
	case "sync":
		s.serveDatastarStream(w, r, resourceKey{kind: "workspace"}, func() (string, error) {
			vm := syncVM{
				baseVM:  s.baseVMForRequest(r, "/events?view=sync"),
				Message: "",
			}
			return s.renderTemplate("sync_main", vm)
		})
	case "archived":
		s.serveDatastarStream(w, r, resourceKey{kind: "workspace"}, func() (string, error) {
			db, err := (store.Store{Dir: s.dir()}).Load()
			if err != nil {
				return "", err
			}
			vm := archivedVM{
				baseVM:  s.baseVMForRequest(r, "/events?view=archived"),
				Rows:    buildArchivedRows(db),
				Message: "",
			}
			return s.renderTemplate("archived_main", vm)
		})
	default:
		http.Error(w, "missing/invalid view (expected home|projects|agenda|sync|archived)", http.StatusBadRequest)
		return
	}
}

func (s *Server) handleNavOptions(w http.ResponseWriter, r *http.Request) {
	dir := s.dir()
	db, err := (store.Store{Dir: dir}).Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	st, err := (store.Store{Dir: dir}).LoadTUIState()
	if err != nil || st == nil {
		// best-effort: treat as empty
		st = &store.TUIState{Version: 1}
	}

	rec := make([]navRecentItem, 0, 5)
	seen := map[string]bool{}
	for _, id0 := range st.RecentItemIDs {
		id := strings.TrimSpace(id0)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		it, ok := db.FindItem(id)
		if !ok || it == nil || it.Archived {
			continue
		}
		title := strings.TrimSpace(it.Title)
		if title == "" {
			title = "(untitled)"
		}
		rec = append(rec, navRecentItem{ID: id, Title: title})
		if len(rec) >= 5 {
			break
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(navOptionsVM{Recent: rec})
}

func (s *Server) handleProjectEvents(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("projectId"))
	if id == "" {
		http.Error(w, "missing project id", http.StatusBadRequest)
		return
	}
	s.serveDatastarStream(w, r, resourceKey{kind: "project", id: id}, func() (string, error) {
		db, err := (store.Store{Dir: s.dir()}).Load()
		if err != nil {
			return "", err
		}
		p, ok := db.FindProject(id)
		if !ok || p == nil || p.Archived {
			return "", errors.New("project not found")
		}
		vm := projectVM{
			baseVM:   s.baseVMForRequest(r, "/projects/"+p.ID+"/events?view=project"),
			Project:  *p,
			Outlines: unarchivedOutlines(db.Outlines, id),
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

	// Component mode: drive the component via Datastar signals (no DOM morphing).
	if s.outlineMode() == "component" && strings.TrimSpace(s.componentsDir()) != "" {
		s.serveDatastarSignalsStream(w, r, resourceKey{kind: "outline", id: id}, func() (map[string]any, error) {
			db, err := (store.Store{Dir: s.dir()}).Load()
			if err != nil {
				return nil, err
			}
			o, ok := db.FindOutline(id)
			if !ok || o == nil || o.Archived {
				return nil, fmt.Errorf("outline not found")
			}
			actorID := strings.TrimSpace(s.actorForRequest(r))
			itemsJSON, statusJSON, assigneesJSON, tagsJSON := outlineComponentPayload(db, *o, actorID)
			return map[string]any{
				"outlineItems":        string(itemsJSON),
				"outlineStatusLabels": string(statusJSON),
				"outlineAssignees":    string(assigneesJSON),
				"outlineTags":         string(tagsJSON),
			}, nil
		})
		return
	}

	// Native mode: patch only the outline container.
	s.serveDatastarElementsStream(w, r, resourceKey{kind: "outline", id: id}, "#outline-native", datastar.ElementPatchModeInner, func() (string, error) {
		db, err := (store.Store{Dir: s.dir()}).Load()
		if err != nil {
			return "", err
		}
		o, ok := db.FindOutline(id)
		if !ok || o == nil || o.Archived {
			return "", errors.New("outline not found")
		}
		actorID := strings.TrimSpace(s.actorForRequest(r))
		itemsJSON, statusJSON, assigneesJSON, tagsJSON := outlineComponentPayload(db, *o, actorID)

		items := make([]model.Item, 0)
		for _, it := range db.Items {
			if it.OutlineID == id && !it.Archived {
				items = append(items, it)
			}
		}
		openTo, endTo, openLbl, endLbl := outlineToggleTargets(*o)
		nodes := buildOutlineNativeNodes(db, *o, actorID, parseOutlineCollapsedCookie(r, o.ID))
		statusOptions := outlineStatusOptionsJSON(*o)
		actorOptions := actorOptionsJSON(db)
		outlineOptions := outlineOptionsJSON(db)

		vm := outlineVM{
			baseVM:              s.baseVMForRequest(r, "/outlines/"+o.ID+"/events?view=outline"),
			Outline:             *o,
			UseOutlineComponent: false,
			ItemsJSON:           itemsJSON,
			StatusLabelsJSON:    statusJSON,
			AssigneesJSON:       assigneesJSON,
			TagsJSON:            tagsJSON,
			Items:               items,
			NativeNodes:         nodes,
			ToggleOpenTo:        openTo,
			ToggleEndTo:         endTo,
			ToggleOpenLbl:       openLbl,
			ToggleEndLbl:        endLbl,
			StatusOptionsJSON:   statusOptions,
			ActorOptionsJSON:    actorOptions,
			OutlineOptionsJSON:  outlineOptions,
		}
		vm.ActorID = actorID
		return s.renderTemplate("outline_native_inner", vm)
	})
}

func (s *Server) handleItemEvents(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("itemId"))
	if id == "" {
		http.Error(w, "missing item id", http.StatusBadRequest)
		return
	}
	s.serveDatastarStream(w, r, resourceKey{kind: "item", id: id}, func() (string, error) {
		db, err := (store.Store{Dir: s.dir()}).Load()
		if err != nil {
			return "", err
		}
		it, ok := db.FindItem(id)
		if !ok || it == nil {
			return "", errors.New("item not found")
		}
		itemReadOnly := it.Archived

		comments := db.CommentsForItem(id)
		actorID := strings.TrimSpace(s.actorForRequest(r))
		canEdit := !itemReadOnly && actorID != "" && perm.CanEditItem(db, actorID, it)
		statusDefs := []model.OutlineStatusDef{}
		var outline *model.Outline
		if o, ok := db.FindOutline(it.OutlineID); ok && o != nil {
			outline = o
			statusDefs = o.StatusDefs
		}
		statusLabelByID := map[string]string{}
		if outline != nil {
			for _, sd := range outline.StatusDefs {
				sid := strings.TrimSpace(sd.ID)
				if sid == "" {
					continue
				}
				lbl := strings.TrimSpace(sd.Label)
				if lbl == "" {
					lbl = sid
				}
				statusLabelByID[sid] = lbl
			}
		}

		var worklog []model.WorklogEntry
		if actorID != "" {
			for _, w := range db.WorklogForItem(id) {
				if strings.TrimSpace(w.AuthorID) == actorID {
					worklog = append(worklog, w)
				}
			}
		}

		commentVMs := make([]itemCommentVM, 0, len(comments))
		for _, c := range comments {
			authorID := strings.TrimSpace(c.AuthorID)
			authorLabel := actorDisplayLabel(db, authorID)
			commentVMs = append(commentVMs, itemCommentVM{
				ID:               c.ID,
				AuthorID:         authorID,
				AuthorLabel:      authorLabel,
				ReplyToCommentID: c.ReplyToCommentID,
				CreatedAt:        c.CreatedAt,
				BodyHTML:         renderMarkdownHTML(c.Body),
			})
		}

		worklogVMs := make([]itemWorklogVM, 0, len(worklog))
		for _, wle := range worklog {
			worklogVMs = append(worklogVMs, itemWorklogVM{
				ID:        wle.ID,
				CreatedAt: wle.CreatedAt,
				BodyHTML:  renderMarkdownHTML(wle.Body),
			})
		}
		commentsCount := len(comments)
		lastComment := ""
		for _, c := range comments {
			if lastComment == "" || c.CreatedAt.UTC().Format(time.RFC3339) > lastComment {
				lastComment = c.CreatedAt.UTC().Format(time.RFC3339)
			}
		}
		worklogCount := len(worklog)
		lastWorklog := ""
		for _, wle := range worklog {
			if lastWorklog == "" || wle.CreatedAt.UTC().Format(time.RFC3339) > lastWorklog {
				lastWorklog = wle.CreatedAt.UTC().Format(time.RFC3339)
			}
		}
		historyRows, lastHistory := itemHistoryRowsForItem(s.dir(), id, 250)
		assignedID := ""
		if it.AssignedActorID != nil {
			assignedID = strings.TrimSpace(*it.AssignedActorID)
		}
		assignedLabel := ""
		if assignedID != "" {
			assignedLabel = actorDisplayLabel(db, assignedID)
		}
		statusLabel := strings.TrimSpace(it.StatusID)
		if mapped, ok := statusLabelByID[statusLabel]; ok && strings.TrimSpace(mapped) != "" {
			statusLabel = strings.TrimSpace(mapped)
		}
		if statusLabel == "" {
			statusLabel = "(none)"
		}

		tagsInput := ""
		if len(it.Tags) > 0 {
			parts := make([]string, 0, len(it.Tags))
			for _, t := range it.Tags {
				t = strings.TrimSpace(t)
				if t == "" {
					continue
				}
				parts = append(parts, "#"+t)
			}
			tagsInput = strings.Join(parts, " ")
		}
		dueDate := ""
		dueTime := ""
		if it.Due != nil {
			dueDate = strings.TrimSpace(it.Due.Date)
			if it.Due.Time != nil {
				dueTime = strings.TrimSpace(*it.Due.Time)
			}
		}
		schDate := ""
		schTime := ""
		if it.Schedule != nil {
			schDate = strings.TrimSpace(it.Schedule.Date)
			if it.Schedule.Time != nil {
				schTime = strings.TrimSpace(*it.Schedule.Time)
			}
		}

		vm := itemVM{
			baseVM:             s.baseVMForRequest(r, "/items/"+it.ID+"/events?view=item"),
			Item:               *it,
			AssignedID:         assignedID,
			AssignedLabel:      assignedLabel,
			StatusLabel:        statusLabel,
			ActorOptions:       []actorOption(nil),
			Comments:           commentVMs,
			ReplyTo:            "",
			Worklog:            worklogVMs,
			DescriptionHTML:    renderMarkdownHTML(it.Description),
			TagsInput:          tagsInput,
			DueDate:            dueDate,
			DueTime:            dueTime,
			SchDate:            schDate,
			SchTime:            schTime,
			StatusOptionsJSON:  template.HTMLAttr("[]"),
			ActorOptionsJSON:   actorOptionsJSON(db),
			OutlineOptionsJSON: outlineOptionsJSON(db),

			CanEdit:        canEdit,
			StatusDefs:     statusDefs,
			ErrorMessage:   "",
			SuccessMessage: "",
			CommentsCount:  commentsCount,
			LastComment:    lastComment,
			WorklogCount:   worklogCount,
			LastWorklog:    lastWorklog,
			HistoryCount:   len(historyRows),
			LastHistory:    lastHistory,
			History:        historyRows,
		}
		if outline != nil {
			vm.StatusOptionsJSON = outlineStatusOptionsJSON(*outline)
		}
		// Parent + Children (TUI parity).
		if it.ParentID != nil && strings.TrimSpace(*it.ParentID) != "" {
			if pit, ok := db.FindItem(strings.TrimSpace(*it.ParentID)); ok && pit != nil && !pit.Archived {
				row := itemOutlineRowVMFromItem(db, outline, statusLabelByID, *pit)
				vm.ParentRow = &row
			}
		}
		{
			kids := db.ChildrenOf(it.ID)
			kptrs := make([]*model.Item, 0, len(kids))
			for i := range kids {
				if kids[i].Archived {
					continue
				}
				kptrs = append(kptrs, &kids[i])
			}
			store.SortItemsByRankOrder(kptrs)
			maxRows := 8
			if len(kptrs) > maxRows {
				vm.ChildrenMore = len(kptrs) - maxRows
				kptrs = kptrs[:maxRows]
			}
			vm.Children = make([]itemOutlineRowVM, 0, len(kptrs))
			for _, p := range kptrs {
				if p == nil {
					continue
				}
				vm.Children = append(vm.Children, itemOutlineRowVMFromItem(db, outline, statusLabelByID, *p))
			}
		}
		if itemReadOnly {
			vm.ReadOnly = true
		}
		vm.ActorOptions = nil
		if db != nil {
			// include options so the edit form can render an assignee picker
			opts := make([]actorOption, 0, len(db.Actors))
			for _, a := range db.Actors {
				id := strings.TrimSpace(a.ID)
				if id == "" {
					continue
				}
				opts = append(opts, actorOption{ID: id, Label: actorDisplayLabel(db, id), Kind: string(a.Kind)})
			}
			sort.Slice(opts, func(i, j int) bool {
				if opts[i].Kind != opts[j].Kind {
					if opts[i].Kind == string(model.ActorKindHuman) {
						return true
					}
					if opts[j].Kind == string(model.ActorKindHuman) {
						return false
					}
					return opts[i].Kind < opts[j].Kind
				}
				if opts[i].Label != opts[j].Label {
					return opts[i].Label < opts[j].Label
				}
				return opts[i].ID < opts[j].ID
			})
			vm.ActorOptions = opts
		}
		vm.ActorID = actorID
		return s.renderTemplate("item_main", vm)
	})
}

type homeVM struct {
	baseVM
	Projects []model.Project
	Ready    []model.Item
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
	db, err := (store.Store{Dir: s.dir()}).Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ready := readyItems(db)

	vm := homeVM{
		baseVM:   s.baseVMForRequest(r, "/events?view=home"),
		Projects: unarchivedProjects(db.Projects),
		Ready:    ready,
	}
	s.writeHTMLTemplate(w, "home.html", vm)
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
	authMode := s.authMode()
	dir := s.dir()
	ws := s.workspaceName()
	if authMode != "dev" && authMode != "magic" {
		http.NotFound(w, r)
		return
	}
	db, err := (store.Store{Dir: dir}).Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	actors := []model.Actor{}
	if authMode == "dev" {
		for _, a := range db.Actors {
			if a.Kind == model.ActorKindHuman {
				actors = append(actors, a)
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tpl := "login_dev.html"
	if authMode == "magic" {
		tpl = "login_magic.html"
	}
	_ = s.tmpl.ExecuteTemplate(w, tpl, loginVM{
		Now:       time.Now().Format(time.RFC3339),
		Workspace: ws,
		Dir:       dir,
		Actors:    actors,
		AuthMode:  authMode,
	})
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	authMode := s.authMode()
	dir := s.dir()
	ws := s.workspaceName()
	if authMode != "dev" && authMode != "magic" {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	db, err := (store.Store{Dir: dir}).Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	secret, err := loadOrInitSecretKey(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch authMode {
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
				Workspace: ws,
				Dir:       dir,
				Actors:    db.Actors,
				AuthMode:  authMode,
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
		users, _, err := store.LoadUsers(dir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		actorID, ok := users.ActorIDForEmail(email)
		if !ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = s.tmpl.ExecuteTemplate(w, "login_magic.html", loginVM{
				Now:       time.Now().Format(time.RFC3339),
				Workspace: ws,
				Dir:       dir,
				AuthMode:  authMode,
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
		_ = writeOutboxEmail(dir, email, "Clarity login link", link)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = s.tmpl.ExecuteTemplate(w, "login_magic_sent.html", map[string]any{
			"Now":       time.Now().Format(time.RFC3339),
			"Workspace": ws,
			"Dir":       dir,
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
	authMode := s.authMode()
	if authMode != "dev" && authMode != "magic" {
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
	authMode := s.authMode()
	dir := s.dir()
	if authMode != "magic" {
		http.NotFound(w, r)
		return
	}

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}
	secret, err := loadOrInitSecretKey(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sp, err := verifyToken(secret, token)
	if err != nil || sp.Typ != "magic" {
		http.Error(w, "invalid token", http.StatusForbidden)
		return
	}

	users, _, err := store.LoadUsers(dir)
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
	baseVM
	Rows []agendaRow
}

type agendaRow struct {
	Kind string // heading|item

	ProjectID   string
	ProjectName string
	OutlineID   string
	OutlineName string

	// Item row only.
	ItemID        string
	Title         string
	StatusID      string
	StatusLabel   string
	IsEndState    bool
	CanEdit       bool
	AssignedLabel string
	Priority      bool
	OnHold        bool
	DueDate       string
	DueTime       string
	SchDate       string
	SchTime       string
	Tags          []string

	Depth       int
	IndentPx    int
	HasChildren bool
}

func (s *Server) handleAgenda(w http.ResponseWriter, r *http.Request) {
	db, err := (store.Store{Dir: s.dir()}).Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	actorID := strings.TrimSpace(s.actorForRequest(r))
	rows := agendaRowsWeb(db, actorID)
	vm := agendaVM{
		baseVM: s.baseVMForRequest(r, "/events?view=agenda"),
		Rows:   rows,
	}
	// Preserve explicit actor resolution semantics used elsewhere.
	vm.ActorID = actorID
	s.writeHTMLTemplate(w, "agenda.html", vm)
}

type captureOutlineOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func (s *Server) handleCaptureOptions(w http.ResponseWriter, r *http.Request) {
	cfg, err := store.LoadConfig()
	if err != nil || cfg == nil {
		// Best-effort: this endpoint is read-only and can still serve defaults even if
		// the global config is temporarily unreadable.
		cfg = &store.GlobalConfig{}
	}
	wsName := strings.TrimSpace(s.workspaceName())

	db, err := (store.Store{Dir: s.dir()}).Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	projects := map[string]string{}
	for _, p := range db.Projects {
		if p.Archived {
			continue
		}
		id := strings.TrimSpace(p.ID)
		if id == "" {
			continue
		}
		projects[id] = strings.TrimSpace(p.Name)
	}
	opts := make([]captureOutlineOption, 0, len(db.Outlines))
	statusByOutline := map[string]any{}
	for _, o := range db.Outlines {
		if o.Archived {
			continue
		}
		id := strings.TrimSpace(o.ID)
		if id == "" {
			continue
		}
		pname := projects[strings.TrimSpace(o.ProjectID)]
		label := ""
		if o.Name != nil {
			label = strings.TrimSpace(*o.Name)
		}
		if pname != "" {
			label = pname + " / " + label
		}
		if strings.TrimSpace(label) == "" {
			label = id
		}
		opts = append(opts, captureOutlineOption{ID: id, Label: label})

		// Status options per outline (for capture draft status picker).
		type statusOpt struct {
			ID           string `json:"id"`
			Label        string `json:"label"`
			IsEndState   bool   `json:"isEndState"`
			RequiresNote bool   `json:"requiresNote,omitempty"`
		}
		st := make([]statusOpt, 0, len(o.StatusDefs)+1)
		if len(o.StatusDefs) > 0 {
			for _, sd := range o.StatusDefs {
				sid := strings.TrimSpace(sd.ID)
				if sid == "" {
					continue
				}
				lbl := strings.TrimSpace(sd.Label)
				if lbl == "" {
					lbl = sid
				}
				st = append(st, statusOpt{ID: sid, Label: lbl, IsEndState: sd.IsEndState, RequiresNote: sd.RequiresNote})
			}
		} else {
			st = append(st, statusOpt{ID: "todo", Label: "TODO", IsEndState: false})
			st = append(st, statusOpt{ID: "doing", Label: "DOING", IsEndState: false})
			st = append(st, statusOpt{ID: "done", Label: "DONE", IsEndState: true})
		}
		statusByOutline[id] = st
	}
	sort.Slice(opts, func(i, j int) bool {
		if opts[i].Label != opts[j].Label {
			return opts[i].Label < opts[j].Label
		}
		return opts[i].ID < opts[j].ID
	})

	// Capture templates (org-capture style) filtered to the active workspace.
	type captureTemplateOpt struct {
		Name      string   `json:"name"`
		Keys      []string `json:"keys"`
		KeyPath   string   `json:"keyPath"`
		OutlineID string   `json:"outlineId"`
	}
	templates := make([]captureTemplateOpt, 0)
	if cfg != nil && wsName != "" {
		for _, t := range cfg.CaptureTemplates {
			if strings.TrimSpace(t.Target.Workspace) != wsName {
				continue
			}
			keys, err := store.NormalizeCaptureTemplateKeys(t.Keys)
			if err != nil {
				continue
			}
			outID := strings.TrimSpace(t.Target.OutlineID)
			if outID == "" {
				continue
			}
			templates = append(templates, captureTemplateOpt{
				Name:      strings.TrimSpace(t.Name),
				Keys:      keys,
				KeyPath:   strings.Join(keys, ""),
				OutlineID: outID,
			})
		}
		sort.Slice(templates, func(i, j int) bool {
			if templates[i].KeyPath != templates[j].KeyPath {
				return templates[i].KeyPath < templates[j].KeyPath
			}
			return templates[i].Name < templates[j].Name
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"outlines":               opts,
		"statusOptionsByOutline": statusByOutline,
		"templates":              templates,
	})
}

type captureCreateReq struct {
	OutlineID   string `json:"outlineId"`
	StatusID    string `json:"statusId,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

func statusIDInDefsWeb(id string, defs []model.OutlineStatusDef) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, d := range defs {
		if strings.TrimSpace(d.ID) == id {
			return true
		}
	}
	return false
}

func (s *Server) handleCaptureCreate(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
		http.Error(w, "read-only", http.StatusForbidden)
		return
	}

	actorID := strings.TrimSpace(s.actorForRequest(r))
	if actorID == "" {
		http.Error(w, "missing actor (start server with --actor, set currentActorId, or /login in dev auth mode)", http.StatusUnauthorized)
		return
	}

	var req captureCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	outlineID := strings.TrimSpace(req.OutlineID)
	title := strings.TrimSpace(req.Title)
	description := req.Description
	if outlineID == "" || title == "" {
		http.Error(w, "missing outlineId or title", http.StatusBadRequest)
		return
	}

	st := store.Store{Dir: s.dir()}
	db, err := st.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := db.FindActor(actorID); !ok {
		http.Error(w, "unknown actor", http.StatusForbidden)
		return
	}
	o, ok := db.FindOutline(outlineID)
	if !ok || o == nil || o.Archived {
		http.NotFound(w, r)
		return
	}

	now := time.Now().UTC()
	itemID := st.NextID(db, "item")

	statusID := strings.TrimSpace(req.StatusID)
	if statusID != "" && !statusIDInDefsWeb(statusID, o.StatusDefs) {
		statusID = ""
	}
	if statusID == "" {
		statusID = "todo"
		if len(o.StatusDefs) > 0 && strings.TrimSpace(o.StatusDefs[0].ID) != "" {
			statusID = strings.TrimSpace(o.StatusDefs[0].ID)
		}
	}

	rank := nextAppendRank(db, outlineID, nil)
	it := model.Item{
		ID:           itemID,
		ProjectID:    o.ProjectID,
		OutlineID:    o.ID,
		ParentID:     nil,
		Rank:         rank,
		Title:        title,
		Description:  description,
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
	if c := s.committer(); c != nil {
		actorLabel := strings.TrimSpace(actorID)
		if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
			actorLabel = strings.TrimSpace(a.Name)
		}
		c.Notify(actorLabel)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":        it.ID,
		"outlineId": it.OutlineID,
		"href":      "/items/" + it.ID,
	})
}

type captureTemplateUpsertReq struct {
	Name      string `json:"name"`
	KeyPath   string `json:"keyPath"`
	OutlineID string `json:"outlineId"`
}

func (s *Server) handleCaptureTemplateUpsert(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
		http.Error(w, "read-only", http.StatusForbidden)
		return
	}
	wsName := strings.TrimSpace(s.workspaceName())
	if wsName == "" {
		http.Error(w, "missing workspace", http.StatusBadRequest)
		return
	}
	var req captureTemplateUpsertReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	keyPath := strings.TrimSpace(req.KeyPath)
	outlineID := strings.TrimSpace(req.OutlineID)
	if name == "" || keyPath == "" || outlineID == "" {
		http.Error(w, "missing name, keyPath, or outlineId", http.StatusBadRequest)
		return
	}

	// Split into single-rune key sequence entries.
	keys := make([]string, 0, len([]rune(keyPath)))
	for _, r := range []rune(keyPath) {
		keys = append(keys, string(r))
	}
	if _, err := store.NormalizeCaptureTemplateKeys(keys); err != nil {
		http.Error(w, "invalid keys: "+err.Error(), http.StatusBadRequest)
		return
	}

	cfg, err := store.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Upsert by keyPath (globally unique).
	next := make([]store.CaptureTemplate, 0, len(cfg.CaptureTemplates)+1)
	replaced := false
	for _, t := range cfg.CaptureTemplates {
		ks, err := store.NormalizeCaptureTemplateKeys(t.Keys)
		if err != nil {
			next = append(next, t)
			continue
		}
		if strings.Join(ks, "") == keyPath {
			updated := t
			updated.Name = name
			updated.Keys = keys
			updated.Target = store.CaptureTemplateTarget{Workspace: wsName, OutlineID: outlineID}
			next = append(next, updated)
			replaced = true
			continue
		}
		next = append(next, t)
	}
	if !replaced {
		next = append(next, store.CaptureTemplate{
			Name: name,
			Keys: keys,
			Target: store.CaptureTemplateTarget{
				Workspace: wsName,
				OutlineID: outlineID,
			},
		})
	}
	cfg.CaptureTemplates = next
	if err := store.ValidateCaptureTemplates(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := store.SaveConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleCaptureTemplateDelete(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
		http.Error(w, "read-only", http.StatusForbidden)
		return
	}
	keyPath := strings.TrimSpace(r.PathValue("keyPath"))
	if keyPath == "" {
		http.Error(w, "missing keyPath", http.StatusBadRequest)
		return
	}
	cfg, err := store.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	next := make([]store.CaptureTemplate, 0, len(cfg.CaptureTemplates))
	for _, t := range cfg.CaptureTemplates {
		ks, err := store.NormalizeCaptureTemplateKeys(t.Keys)
		if err != nil {
			next = append(next, t)
			continue
		}
		if strings.Join(ks, "") == keyPath {
			continue
		}
		next = append(next, t)
	}
	cfg.CaptureTemplates = next
	if err := store.ValidateCaptureTemplates(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := store.SaveConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

type syncVM struct {
	baseVM
	Message string
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	msg := strings.TrimSpace(r.URL.Query().Get("msg"))
	vm := syncVM{
		baseVM:  s.baseVMForRequest(r, "/events?view=sync"),
		Message: msg,
	}
	s.writeHTMLTemplate(w, "sync.html", vm)
}

func (s *Server) handleSyncInit(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
		http.Error(w, "read-only", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	dir := s.dir()
	st, err := gitrepo.GetStatus(ctx, dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if st.IsRepo {
		http.Redirect(w, r, "/sync?msg=already%20a%20git%20repo", http.StatusSeeOther)
		return
	}

	if err := gitrepo.Init(ctx, dir); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/sync?msg=git%20init%20ok", http.StatusSeeOther)
}

func (s *Server) handleSyncRemote(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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

	dir := s.dir()
	st, err := gitrepo.GetStatus(ctx, dir)
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

	if err := gitrepo.SetRemoteURL(ctx, dir, remoteName, remoteURL); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/sync?msg=remote%20set%20ok", http.StatusSeeOther)
}

func (s *Server) handleSyncPushUpstream(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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

	dir := s.dir()
	before, err := gitrepo.GetStatus(ctx, dir)
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

	if err := gitrepo.PushSetUpstream(ctx, dir, remoteName, "HEAD"); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/sync?msg=push%20--set-upstream%20ok", http.StatusSeeOther)
}

func (s *Server) handleSyncPull(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
		http.Error(w, "read-only", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	dir := s.dir()
	before, err := gitrepo.GetStatus(ctx, dir)
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

	if err := gitrepo.PullRebase(ctx, dir); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/sync?msg=pull%20--rebase%20ok", http.StatusSeeOther)
}

func (s *Server) handleSyncPush(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
		http.Error(w, "read-only", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	dir := s.dir()
	before, err := gitrepo.GetStatus(ctx, dir)
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
		db, loadErr := (store.Store{Dir: dir}).Load()
		actorLabel := strings.TrimSpace(s.actorForRequest(r))
		if loadErr == nil && db != nil && actorLabel != "" {
			if a, ok := db.FindActor(actorLabel); ok && a != nil && strings.TrimSpace(a.Name) != "" {
				actorLabel = strings.TrimSpace(a.Name)
			}
		}
		committed, commitErr = gitrepo.CommitWorkspaceCanonicalAuto(ctx, dir, actorLabel)
	} else {
		committed, commitErr = gitrepo.CommitWorkspaceCanonical(ctx, dir, msg)
	}
	if commitErr != nil {
		http.Error(w, commitErr.Error(), http.StatusConflict)
		return
	}

	// Pull/rebase (best-effort; same as CLI's default).
	pulled := false
	cur, err := gitrepo.GetStatus(ctx, dir)
	if err == nil && strings.TrimSpace(cur.Upstream) != "" {
		if !cur.Unmerged && !cur.InProgress && !cur.DirtyTracked {
			if err := gitrepo.PullRebase(ctx, dir); err == nil {
				pulled = true
			}
		}
	}

	pushed := false
	if err := gitrepo.Push(ctx, dir); err == nil {
		pushed = true
	} else if gitrepo.IsNonFastForwardPushErr(err) {
		// Retry once: pull --rebase + push.
		if err := gitrepo.PullRebase(ctx, dir); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		pulled = true
		if err := gitrepo.Push(ctx, dir); err != nil {
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

type workspacesVM struct {
	baseVM
	Entries          []store.WorkspaceEntry
	CurrentWorkspace string
	Message          string
}

func (s *Server) switchWorkspace(name string) error {
	name, err := store.NormalizeWorkspaceName(name)
	if err != nil {
		return err
	}

	dir, err := store.WorkspaceDir(name)
	if err != nil {
		return err
	}
	st := store.Store{Dir: dir}
	if err := st.Ensure(); err != nil {
		return err
	}

	// Update global config (best-effort MRU).
	cfg, err := store.LoadConfig()
	if err == nil && cfg != nil {
		if cfg.Workspaces != nil {
			if ref, ok := cfg.Workspaces[name]; ok {
				ref.LastOpened = time.Now().UTC().Format(time.RFC3339Nano)
				cfg.Workspaces[name] = ref
			}
		}
		cfg.CurrentWorkspace = name
		_ = store.SaveConfig(cfg)
	}

	// Swap server workspace (affects all subsequent requests).
	s.mu.Lock()
	oldBC := s.bc
	s.cfg.Dir = dir
	s.cfg.Workspace = name
	// Restart broadcaster for the new workspace root.
	s.bc = newResourceBroadcaster(dir)
	go s.bc.watchLoop()
	// Reset auto-commit for the new workspace dir.
	s.autoCommit = nil
	if !s.cfg.ReadOnly && gitrepo.AutoCommitEnabled() {
		s.autoCommit = gitrepo.NewDebouncedCommitter(gitrepo.DebouncedCommitterOpts{
			WorkspaceDir:   dir,
			Debounce:       8 * time.Second,
			AutoPush:       gitrepo.AutoPushEnabled(),
			AutoPullRebase: gitrepo.AutoPullRebaseEnabled(),
		})
	}
	s.mu.Unlock()

	if oldBC != nil {
		oldBC.Stop()
	}
	return nil
}

func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	cfg, err := store.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	current := ""
	current = strings.TrimSpace(cfg.CurrentWorkspace)
	if current == "" {
		current = strings.TrimSpace(s.workspaceName())
	}
	if current == "" {
		current = "default"
	}

	ents, err := store.ListWorkspaceEntries()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	msg := strings.TrimSpace(r.URL.Query().Get("msg"))
	vm := workspacesVM{
		baseVM:           s.baseVMForRequest(r, ""),
		Entries:          ents,
		CurrentWorkspace: current,
		Message:          msg,
	}
	s.writeHTMLTemplate(w, "workspaces.html", vm)
}

func (s *Server) handleWorkspacesUse(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.Form.Get("name"))
	if name == "" {
		http.Error(w, "missing workspace name", http.StatusBadRequest)
		return
	}
	if err := s.switchWorkspace(name); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/?msg=switched%20workspace", http.StatusSeeOther)
}

func (s *Server) handleWorkspacesNew(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.Form.Get("name"))
	name, err := store.NormalizeWorkspaceName(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Create under legacy root (v1 convenience). Teams can still "migrate" to a Git-backed path later.
	dir, err := store.LegacyWorkspaceDir(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := (store.Store{Dir: dir}).Ensure(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.switchWorkspace(name); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/?msg=created%20workspace", http.StatusSeeOther)
}

func (s *Server) handleWorkspacesRename(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	from := strings.TrimSpace(r.Form.Get("from"))
	to := strings.TrimSpace(r.Form.Get("to"))
	from, err := store.NormalizeWorkspaceName(from)
	if err != nil {
		http.Error(w, "invalid from: "+err.Error(), http.StatusBadRequest)
		return
	}
	to, err = store.NormalizeWorkspaceName(to)
	if err != nil {
		http.Error(w, "invalid to: "+err.Error(), http.StatusBadRequest)
		return
	}
	if from == to {
		http.Redirect(w, r, "/workspaces?msg=no-op", http.StatusSeeOther)
		return
	}

	cfg, err := store.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if cfg == nil {
		cfg = &store.GlobalConfig{}
	}

	// Determine whether this is a registry entry (preferred) or legacy.
	ref, hasRef := store.WorkspaceRef{}, false
	if cfg.Workspaces != nil {
		if r, ok := cfg.Workspaces[from]; ok {
			ref = r
			hasRef = true
		}
	}

	if hasRef {
		// Rename registry key (path stays the same).
		if cfg.Workspaces == nil {
			cfg.Workspaces = map[string]store.WorkspaceRef{}
		}
		delete(cfg.Workspaces, from)
		cfg.Workspaces[to] = ref
	} else {
		// Legacy: rename directory under ~/.clarity/workspaces.
		fromDir, err := store.LegacyWorkspaceDir(from)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		toDir, err := store.LegacyWorkspaceDir(to)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.Rename(fromDir, toDir); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}

	wasCurrent := strings.TrimSpace(cfg.CurrentWorkspace) == from
	if wasCurrent {
		cfg.CurrentWorkspace = to
	}
	if err := store.SaveConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If the server is currently using that workspace name, switch to the new name so
	// the broadcaster and auto-commit stay consistent.
	if strings.TrimSpace(s.workspaceName()) == from || wasCurrent {
		_ = s.switchWorkspace(to)
	}

	http.Redirect(w, r, "/workspaces?msg=renamed", http.StatusSeeOther)
}

type archivedVM struct {
	baseVM
	Rows    []archivedRow
	Message string
}

type archivedRow struct {
	Kind        string // heading|project|outline|item
	Label       string
	ProjectName string
	OutlineName string
	Title       string
	ProjectID   string
	OutlineID   string
	ItemID      string
}

func outlineDisplayNameWeb(o model.Outline) string {
	if o.Name != nil {
		if n := strings.TrimSpace(*o.Name); n != "" {
			return n
		}
	}
	return "(unnamed outline)"
}

func buildArchivedRows(db *store.DB) []archivedRow {
	if db == nil {
		return []archivedRow{{Kind: "heading", Label: "No archived content"}}
	}

	projectNameByID := map[string]string{}
	for _, p := range db.Projects {
		projectNameByID[p.ID] = strings.TrimSpace(p.Name)
	}
	outlineNameByID := map[string]string{}
	for _, o := range db.Outlines {
		outlineNameByID[o.ID] = outlineDisplayNameWeb(o)
	}

	projects := make([]model.Project, 0, len(db.Projects))
	for _, p := range db.Projects {
		if p.Archived {
			projects = append(projects, p)
		}
	}
	sort.Slice(projects, func(i, j int) bool {
		ni := strings.ToLower(strings.TrimSpace(projects[i].Name))
		nj := strings.ToLower(strings.TrimSpace(projects[j].Name))
		if ni == nj {
			return projects[i].ID < projects[j].ID
		}
		if ni == "" {
			return false
		}
		if nj == "" {
			return true
		}
		return ni < nj
	})

	outlines := make([]model.Outline, 0, len(db.Outlines))
	for _, o := range db.Outlines {
		if o.Archived {
			outlines = append(outlines, o)
		}
	}
	sort.Slice(outlines, func(i, j int) bool {
		pi := strings.ToLower(strings.TrimSpace(projectNameByID[outlines[i].ProjectID]))
		pj := strings.ToLower(strings.TrimSpace(projectNameByID[outlines[j].ProjectID]))
		if pi != pj {
			if pi == "" {
				return false
			}
			if pj == "" {
				return true
			}
			return pi < pj
		}
		oi := strings.ToLower(strings.TrimSpace(outlineDisplayNameWeb(outlines[i])))
		oj := strings.ToLower(strings.TrimSpace(outlineDisplayNameWeb(outlines[j])))
		if oi == oj {
			return outlines[i].ID < outlines[j].ID
		}
		if oi == "" {
			return false
		}
		if oj == "" {
			return true
		}
		return oi < oj
	})

	itemsOnly := make([]model.Item, 0, len(db.Items))
	for _, it := range db.Items {
		if it.Archived {
			itemsOnly = append(itemsOnly, it)
		}
	}
	sort.Slice(itemsOnly, func(i, j int) bool {
		pi := strings.ToLower(strings.TrimSpace(projectNameByID[itemsOnly[i].ProjectID]))
		pj := strings.ToLower(strings.TrimSpace(projectNameByID[itemsOnly[j].ProjectID]))
		if pi != pj {
			if pi == "" {
				return false
			}
			if pj == "" {
				return true
			}
			return pi < pj
		}
		oi := strings.ToLower(strings.TrimSpace(outlineNameByID[itemsOnly[i].OutlineID]))
		oj := strings.ToLower(strings.TrimSpace(outlineNameByID[itemsOnly[j].OutlineID]))
		if oi != oj {
			if oi == "" {
				return false
			}
			if oj == "" {
				return true
			}
			return oi < oj
		}
		ti := strings.ToLower(strings.TrimSpace(itemsOnly[i].Title))
		tj := strings.ToLower(strings.TrimSpace(itemsOnly[j].Title))
		if ti == tj {
			return itemsOnly[i].ID < itemsOnly[j].ID
		}
		if ti == "" {
			return false
		}
		if tj == "" {
			return true
		}
		return ti < tj
	})

	if len(projects) == 0 && len(outlines) == 0 && len(itemsOnly) == 0 {
		return []archivedRow{{Kind: "heading", Label: "No archived content"}}
	}

	rows := make([]archivedRow, 0, 8+len(projects)+len(outlines)+len(itemsOnly))
	rows = append(rows, archivedRow{Kind: "heading", Label: "Archived projects"})
	for _, p := range projects {
		rows = append(rows, archivedRow{Kind: "project", ProjectID: p.ID, ProjectName: strings.TrimSpace(p.Name)})
	}
	rows = append(rows, archivedRow{Kind: "heading", Label: "Archived outlines"})
	for _, o := range outlines {
		rows = append(rows, archivedRow{Kind: "outline", OutlineID: o.ID, ProjectName: projectNameByID[o.ProjectID], OutlineName: outlineDisplayNameWeb(o)})
	}
	rows = append(rows, archivedRow{Kind: "heading", Label: "Archived items"})
	for _, it := range itemsOnly {
		rows = append(rows, archivedRow{
			Kind:        "item",
			ProjectName: projectNameByID[it.ProjectID],
			OutlineName: outlineNameByID[it.OutlineID],
			Title:       strings.TrimSpace(it.Title),
			ItemID:      it.ID,
		})
	}
	return rows
}

func (s *Server) handleArchived(w http.ResponseWriter, r *http.Request) {
	db, err := (store.Store{Dir: s.dir()}).Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	msg := strings.TrimSpace(r.URL.Query().Get("msg"))
	vm := archivedVM{
		baseVM:  s.baseVMForRequest(r, "/events?view=archived"),
		Rows:    buildArchivedRows(db),
		Message: msg,
	}
	s.writeHTMLTemplate(w, "archived.html", vm)
}

type projectsVM struct {
	baseVM
	Projects []model.Project
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	db, err := (store.Store{Dir: s.dir()}).Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	vm := projectsVM{
		baseVM:   s.baseVMForRequest(r, "/events?view=projects"),
		Projects: unarchivedProjects(db.Projects),
	}
	s.writeHTMLTemplate(w, "projects.html", vm)
}

func (s *Server) handleProjectCreate(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
		http.Error(w, "read-only", http.StatusForbidden)
		return
	}

	actorID := strings.TrimSpace(s.actorForRequest(r))
	if actorID == "" {
		http.Error(w, "missing actor (start server with --actor, set currentActorId, or /login in dev auth mode)", http.StatusUnauthorized)
		return
	}
	_ = r.ParseForm()
	name := strings.TrimSpace(r.Form.Get("name"))
	if name == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}

	st := store.Store{Dir: s.dir()}
	db, err := st.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := db.FindActor(actorID); !ok {
		http.Error(w, "unknown actor", http.StatusForbidden)
		return
	}

	p := model.Project{
		ID:        st.NextID(db, "proj"),
		Name:      name,
		CreatedBy: actorID,
		CreatedAt: time.Now().UTC(),
	}
	db.Projects = append(db.Projects, p)
	db.CurrentProjectID = p.ID
	if err := st.AppendEvent(actorID, "project.create", p.ID, p); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := st.Save(db); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if c := s.committer(); c != nil {
		actorLabel := strings.TrimSpace(actorID)
		if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
			actorLabel = strings.TrimSpace(a.Name)
		}
		c.Notify(actorLabel)
	}
	http.Redirect(w, r, "/projects/"+p.ID, http.StatusSeeOther)
}

func (s *Server) handleProjectRename(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
		http.Error(w, "read-only", http.StatusForbidden)
		return
	}

	actorID := strings.TrimSpace(s.actorForRequest(r))
	if actorID == "" {
		http.Error(w, "missing actor (start server with --actor, set currentActorId, or /login in dev auth mode)", http.StatusUnauthorized)
		return
	}
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	if projectID == "" {
		http.Error(w, "missing project id", http.StatusBadRequest)
		return
	}
	_ = r.ParseForm()
	name := strings.TrimSpace(r.Form.Get("name"))
	if name == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}

	st := store.Store{Dir: s.dir()}
	db, err := st.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := db.FindActor(actorID); !ok {
		http.Error(w, "unknown actor", http.StatusForbidden)
		return
	}
	p, ok := db.FindProject(projectID)
	if !ok || p == nil || p.Archived {
		http.NotFound(w, r)
		return
	}

	if strings.TrimSpace(p.Name) != name {
		p.Name = name
		if err := st.AppendEvent(actorID, "project.rename", p.ID, map[string]any{"name": p.Name}); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err := st.Save(db); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if c := s.committer(); c != nil {
			actorLabel := strings.TrimSpace(actorID)
			if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
				actorLabel = strings.TrimSpace(a.Name)
			}
			c.Notify(actorLabel)
		}
	}
	redirectBack(w, r, "/projects/"+projectID)
}

func (s *Server) handleProjectArchive(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
		http.Error(w, "read-only", http.StatusForbidden)
		return
	}

	actorID := strings.TrimSpace(s.actorForRequest(r))
	if actorID == "" {
		http.Error(w, "missing actor (start server with --actor, set currentActorId, or /login in dev auth mode)", http.StatusUnauthorized)
		return
	}
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	if projectID == "" {
		http.Error(w, "missing project id", http.StatusBadRequest)
		return
	}
	_ = r.ParseForm()
	archived := true
	if strings.TrimSpace(r.Form.Get("unarchive")) != "" {
		archived = false
	}

	st := store.Store{Dir: s.dir()}
	db, err := st.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := db.FindActor(actorID); !ok {
		http.Error(w, "unknown actor", http.StatusForbidden)
		return
	}
	p, ok := db.FindProject(projectID)
	if !ok || p == nil {
		http.NotFound(w, r)
		return
	}

	if p.Archived != archived {
		p.Archived = archived
		if db.CurrentProjectID == projectID && p.Archived {
			db.CurrentProjectID = ""
		}
		if err := st.AppendEvent(actorID, "project.archive", p.ID, map[string]any{"archived": p.Archived}); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err := st.Save(db); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if c := s.committer(); c != nil {
			actorLabel := strings.TrimSpace(actorID)
			if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
				actorLabel = strings.TrimSpace(a.Name)
			}
			c.Notify(actorLabel)
		}
	}

	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

func (s *Server) handleOutlineCreate(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
		http.Error(w, "read-only", http.StatusForbidden)
		return
	}

	actorID := strings.TrimSpace(s.actorForRequest(r))
	if actorID == "" {
		http.Error(w, "missing actor (start server with --actor, set currentActorId, or /login in dev auth mode)", http.StatusUnauthorized)
		return
	}
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	if projectID == "" {
		http.Error(w, "missing project id", http.StatusBadRequest)
		return
	}
	_ = r.ParseForm()
	name := strings.TrimSpace(r.Form.Get("name"))
	var namePtr *string
	if name != "" {
		tmp := name
		namePtr = &tmp
	}

	st := store.Store{Dir: s.dir()}
	db, err := st.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := db.FindActor(actorID); !ok {
		http.Error(w, "unknown actor", http.StatusForbidden)
		return
	}
	p, ok := db.FindProject(projectID)
	if !ok || p == nil || p.Archived {
		http.NotFound(w, r)
		return
	}

	o := model.Outline{
		ID:         st.NextID(db, "out"),
		ProjectID:  projectID,
		Name:       namePtr,
		StatusDefs: store.DefaultOutlineStatusDefs(),
		CreatedBy:  actorID,
		CreatedAt:  time.Now().UTC(),
	}
	db.Outlines = append(db.Outlines, o)
	if err := st.AppendEvent(actorID, "outline.create", o.ID, o); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := st.Save(db); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if c := s.committer(); c != nil {
		actorLabel := strings.TrimSpace(actorID)
		if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
			actorLabel = strings.TrimSpace(a.Name)
		}
		c.Notify(actorLabel)
	}
	http.Redirect(w, r, "/outlines/"+o.ID, http.StatusSeeOther)
}

type projectVM struct {
	baseVM
	Project  model.Project
	Outlines []model.Outline
}

func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	if projectID == "" {
		http.Error(w, "missing project id", http.StatusBadRequest)
		return
	}

	db, err := (store.Store{Dir: s.dir()}).Load()
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

	vm := projectVM{
		baseVM:   s.baseVMForRequest(r, "/projects/"+p.ID+"/events?view=project"),
		Project:  *p,
		Outlines: outlines,
	}
	s.writeHTMLTemplate(w, "project.html", vm)
}

type outlineVM struct {
	baseVM
	Outline model.Outline

	// Outline component view-model (progressive enhancement).
	UseOutlineComponent bool
	ItemsJSON           template.HTMLAttr
	StatusLabelsJSON    template.HTMLAttr
	AssigneesJSON       template.HTMLAttr
	TagsJSON            template.HTMLAttr

	// Fallback list (no JS / no components).
	Items []model.Item

	// Native outline (server-rendered HTML).
	NativeNodes   []outlineNativeNode
	ToggleOpenTo  string
	ToggleEndTo   string
	ToggleOpenLbl string
	ToggleEndLbl  string

	StatusOptionsJSON  template.HTMLAttr
	ActorOptionsJSON   template.HTMLAttr
	OutlineOptionsJSON template.HTMLAttr
}

type outlineNativeNode struct {
	ID            string
	Title         string
	StatusID      string
	StatusLabel   string
	IsEndState    bool
	CanEdit       bool
	AssignedLabel string

	Priority bool
	OnHold   bool
	DueDate  string
	DueTime  string
	SchDate  string
	SchTime  string
	Tags     []string

	Collapsed bool

	DoneChildren  int
	TotalChildren int

	Children []outlineNativeNode
}

type actorOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind,omitempty"`
}

func actorDisplayLabel(db *store.DB, actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if db == nil || actorID == "" {
		return ""
	}
	if a, ok := db.FindActor(actorID); ok && a != nil {
		if strings.TrimSpace(a.Name) != "" {
			return strings.TrimSpace(a.Name)
		}
	}
	return actorID
}

func actorOptionsJSON(db *store.DB) template.HTMLAttr {
	opts := make([]actorOption, 0)
	if db != nil {
		for _, a := range db.Actors {
			id := strings.TrimSpace(a.ID)
			if id == "" {
				continue
			}
			lbl := strings.TrimSpace(a.Name)
			if lbl == "" {
				lbl = id
			}
			opts = append(opts, actorOption{ID: id, Label: lbl, Kind: string(a.Kind)})
		}
	}
	sort.Slice(opts, func(i, j int) bool {
		// Humans first.
		if opts[i].Kind != opts[j].Kind {
			if opts[i].Kind == string(model.ActorKindHuman) {
				return true
			}
			if opts[j].Kind == string(model.ActorKindHuman) {
				return false
			}
			return opts[i].Kind < opts[j].Kind
		}
		if opts[i].Label != opts[j].Label {
			return opts[i].Label < opts[j].Label
		}
		return opts[i].ID < opts[j].ID
	})
	b, _ := json.Marshal(opts)
	return template.HTMLAttr(b)
}

func outlineOptionsJSON(db *store.DB) template.HTMLAttr {
	type outlineOpt struct {
		ID           string `json:"id"`
		Label        string `json:"label"`
		ProjectID    string `json:"projectId,omitempty"`
		ProjectLabel string `json:"projectLabel,omitempty"`
	}
	opts := make([]outlineOpt, 0)
	if db != nil {
		for _, o := range db.Outlines {
			if o.Archived {
				continue
			}
			id := strings.TrimSpace(o.ID)
			if id == "" {
				continue
			}
			name := "(unnamed outline)"
			if o.Name != nil && strings.TrimSpace(*o.Name) != "" {
				name = strings.TrimSpace(*o.Name)
			}
			projID := strings.TrimSpace(o.ProjectID)
			projLabel := ""
			if projID != "" {
				if p, ok := db.FindProject(projID); ok && p != nil && strings.TrimSpace(p.Name) != "" {
					projLabel = strings.TrimSpace(p.Name)
				} else {
					projLabel = projID
				}
			}
			lbl := name
			if projLabel != "" {
				lbl = projLabel + " / " + name
			}
			opts = append(opts, outlineOpt{ID: id, Label: lbl, ProjectID: projID, ProjectLabel: projLabel})
		}
	}
	sort.Slice(opts, func(i, j int) bool {
		if opts[i].Label != opts[j].Label {
			return opts[i].Label < opts[j].Label
		}
		return opts[i].ID < opts[j].ID
	})
	b, _ := json.Marshal(opts)
	return template.HTMLAttr(b)
}

func outlineToggleTargets(o model.Outline) (openTo, endTo, openLbl, endLbl string) {
	// Prefer explicit outline statuses when present (by stable index).
	if len(o.StatusDefs) > 0 {
		firstOpen := -1
		firstEnd := -1
		for i, sd := range o.StatusDefs {
			if sd.IsEndState && firstEnd < 0 {
				firstEnd = i
			}
			if !sd.IsEndState && firstOpen < 0 {
				firstOpen = i
			}
		}
		if firstOpen < 0 {
			firstOpen = 0
		}
		if firstEnd < 0 {
			if len(o.StatusDefs) > 1 {
				firstEnd = 1
			} else {
				firstEnd = 0
			}
		}
		openLbl = strings.TrimSpace(o.StatusDefs[firstOpen].Label)
		if openLbl == "" {
			openLbl = strings.TrimSpace(o.StatusDefs[firstOpen].ID)
		}
		endLbl = strings.TrimSpace(o.StatusDefs[firstEnd].Label)
		if endLbl == "" {
			endLbl = strings.TrimSpace(o.StatusDefs[firstEnd].ID)
		}
		return fmt.Sprintf("status-%d", firstOpen), fmt.Sprintf("status-%d", firstEnd), openLbl, endLbl
	}
	// Fallback: legacy/default statuses.
	return "todo", "done", "TODO", "DONE"
}

func outlineStatusOptionsJSON(o model.Outline) template.HTMLAttr {
	type opt struct {
		ID           string `json:"id"`
		Label        string `json:"label"`
		IsEndState   bool   `json:"isEndState"`
		RequiresNote bool   `json:"requiresNote,omitempty"`
	}
	out := make([]opt, 0, len(o.StatusDefs)+1)
	if len(o.StatusDefs) > 0 {
		for _, sd := range o.StatusDefs {
			id := strings.TrimSpace(sd.ID)
			if id == "" {
				continue
			}
			lbl := strings.TrimSpace(sd.Label)
			if lbl == "" {
				lbl = id
			}
			out = append(out, opt{ID: id, Label: lbl, IsEndState: sd.IsEndState, RequiresNote: sd.RequiresNote})
		}
	} else {
		out = append(out, opt{ID: "todo", Label: "TODO", IsEndState: false})
		out = append(out, opt{ID: "doing", Label: "DOING", IsEndState: false})
		out = append(out, opt{ID: "done", Label: "DONE", IsEndState: true})
	}
	b, _ := json.Marshal(out)
	return template.HTMLAttr(b)
}

func parseOutlineCollapsedCookie(r *http.Request, outlineID string) map[string]bool {
	outlineID = strings.TrimSpace(outlineID)
	if r == nil || outlineID == "" {
		return map[string]bool{}
	}
	name := "clarity_outline_collapsed_" + outlineID
	c, err := r.Cookie(name)
	if err != nil || c == nil {
		return map[string]bool{}
	}
	raw := strings.TrimSpace(c.Value)
	if raw == "" {
		return map[string]bool{}
	}
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		out[id] = true
	}
	return out
}

func buildOutlineNativeNodes(db *store.DB, o model.Outline, actorID string, collapsed map[string]bool) []outlineNativeNode {
	actorID = strings.TrimSpace(actorID)

	statusLabelByID := map[string]string{}
	for _, sd := range o.StatusDefs {
		if strings.TrimSpace(sd.ID) == "" {
			continue
		}
		lbl := strings.TrimSpace(sd.Label)
		if lbl == "" {
			lbl = strings.TrimSpace(sd.ID)
		}
		statusLabelByID[strings.TrimSpace(sd.ID)] = lbl
	}

	// Group items in outline by parent.
	byID := map[string]*model.Item{}
	children := map[string][]*model.Item{} // parent id => items ("" for root)
	for i := range db.Items {
		it := &db.Items[i]
		if it.OutlineID != o.ID || it.Archived {
			continue
		}
		byID[it.ID] = it
	}
	for _, it := range byID {
		parent := ""
		if it.ParentID != nil {
			pid := strings.TrimSpace(*it.ParentID)
			if pid != "" {
				if pit, ok := byID[pid]; ok && pit != nil && !pit.Archived {
					parent = pid
				}
			}
		}
		children[parent] = append(children[parent], it)
	}

	// Sort siblings by rank.
	for pid := range children {
		store.SortItemsByRankOrder(children[pid])
	}

	var build func(parent string) []outlineNativeNode
	build = func(parent string) []outlineNativeNode {
		sibs := children[parent]
		out := make([]outlineNativeNode, 0, len(sibs))
		for _, it := range sibs {
			if it == nil {
				continue
			}
			// Direct child progress (TUI parity): count only direct children.
			doneChildren := 0
			totalChildren := 0
			if kids := children[it.ID]; len(kids) > 0 {
				totalChildren = len(kids)
				for _, ch := range kids {
					if ch == nil {
						continue
					}
					if statusutil.IsEndState(o, ch.StatusID) {
						doneChildren++
					}
				}
			}
			sid := strings.TrimSpace(it.StatusID)
			lbl := sid
			if mapped, ok := statusLabelByID[sid]; ok && strings.TrimSpace(mapped) != "" {
				lbl = strings.TrimSpace(mapped)
			}
			assignedLabel := ""
			if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) != "" {
				assignedLabel = actorDisplayLabel(db, strings.TrimSpace(*it.AssignedActorID))
			}
			dueDate := ""
			dueTime := ""
			if it.Due != nil {
				dueDate = strings.TrimSpace(it.Due.Date)
				if it.Due.Time != nil {
					dueTime = strings.TrimSpace(*it.Due.Time)
				}
			}
			schDate := ""
			schTime := ""
			if it.Schedule != nil {
				schDate = strings.TrimSpace(it.Schedule.Date)
				if it.Schedule.Time != nil {
					schTime = strings.TrimSpace(*it.Schedule.Time)
				}
			}
			out = append(out, outlineNativeNode{
				ID:            it.ID,
				Title:         it.Title,
				StatusID:      it.StatusID,
				StatusLabel:   lbl,
				IsEndState:    statusutil.IsEndState(o, it.StatusID),
				CanEdit:       actorID != "" && perm.CanEditItem(db, actorID, it),
				AssignedLabel: assignedLabel,
				Priority:      it.Priority,
				OnHold:        it.OnHold,
				DueDate:       dueDate,
				DueTime:       dueTime,
				SchDate:       schDate,
				SchTime:       schTime,
				Tags:          it.Tags,
				Collapsed:     collapsed != nil && collapsed[it.ID],
				DoneChildren:  doneChildren,
				TotalChildren: totalChildren,
				Children:      build(it.ID),
			})
		}
		return out
	}
	return build("")
}

func (s *Server) handleOutline(w http.ResponseWriter, r *http.Request) {
	outlineID := strings.TrimSpace(r.PathValue("outlineId"))
	if outlineID == "" {
		http.Error(w, "missing outline id", http.StatusBadRequest)
		return
	}

	db, err := (store.Store{Dir: s.dir()}).Load()
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
	useComponent := s.outlineMode() == "component" && strings.TrimSpace(s.componentsDir()) != ""
	itemsJSON, statusJSON, assigneesJSON, tagsJSON := outlineComponentPayload(db, *o, actorID)
	openTo, endTo, openLbl, endLbl := outlineToggleTargets(*o)
	nodes := buildOutlineNativeNodes(db, *o, actorID, parseOutlineCollapsedCookie(r, o.ID))
	statusOptions := outlineStatusOptionsJSON(*o)
	actorOptions := actorOptionsJSON(db)
	outlineOptions := outlineOptionsJSON(db)

	vm := outlineVM{
		baseVM:              s.baseVMForRequest(r, "/outlines/"+o.ID+"/events?view=outline"),
		Outline:             *o,
		UseOutlineComponent: useComponent,
		ItemsJSON:           itemsJSON,
		StatusLabelsJSON:    statusJSON,
		AssigneesJSON:       assigneesJSON,
		TagsJSON:            tagsJSON,
		Items:               items,
		NativeNodes:         nodes,
		ToggleOpenTo:        openTo,
		ToggleEndTo:         endTo,
		ToggleOpenLbl:       openLbl,
		ToggleEndLbl:        endLbl,
		StatusOptionsJSON:   statusOptions,
		ActorOptionsJSON:    actorOptions,
		OutlineOptionsJSON:  outlineOptions,
	}
	vm.ActorID = actorID
	s.writeHTMLTemplate(w, "outline.html", vm)
}

func (s *Server) handleOutlineRename(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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
	_ = r.ParseForm()
	name := strings.TrimSpace(r.Form.Get("name"))
	var namePtr *string
	if name != "" {
		tmp := name
		namePtr = &tmp
	}

	st := store.Store{Dir: s.dir()}
	db, err := st.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := db.FindActor(actorID); !ok {
		http.Error(w, "unknown actor", http.StatusForbidden)
		return
	}
	o, ok := db.FindOutline(outlineID)
	if !ok || o == nil || o.Archived {
		http.NotFound(w, r)
		return
	}

	prev := ""
	if o.Name != nil {
		prev = strings.TrimSpace(*o.Name)
	}
	next := ""
	if namePtr != nil {
		next = strings.TrimSpace(*namePtr)
	}
	if prev != next {
		o.Name = namePtr
		if err := st.AppendEvent(actorID, "outline.rename", o.ID, map[string]any{"name": o.Name}); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err := st.Save(db); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if c := s.committer(); c != nil {
			actorLabel := strings.TrimSpace(actorID)
			if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
				actorLabel = strings.TrimSpace(a.Name)
			}
			c.Notify(actorLabel)
		}
	}

	redirectBack(w, r, "/outlines/"+outlineID)
}

func (s *Server) handleOutlineArchive(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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
	_ = r.ParseForm()
	archived := true
	if strings.TrimSpace(r.Form.Get("unarchive")) != "" {
		archived = false
	}

	st := store.Store{Dir: s.dir()}
	db, err := st.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := db.FindActor(actorID); !ok {
		http.Error(w, "unknown actor", http.StatusForbidden)
		return
	}
	o, ok := db.FindOutline(outlineID)
	if !ok || o == nil {
		http.NotFound(w, r)
		return
	}

	if o.Archived != archived {
		o.Archived = archived
		if err := st.AppendEvent(actorID, "outline.archive", o.ID, map[string]any{"archived": o.Archived}); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err := st.Save(db); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if c := s.committer(); c != nil {
			actorLabel := strings.TrimSpace(actorID)
			if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
				actorLabel = strings.TrimSpace(a.Name)
			}
			c.Notify(actorLabel)
		}
	}

	http.Redirect(w, r, "/projects/"+o.ProjectID, http.StatusSeeOther)
}

func (s *Server) handleOutlineSetDescription(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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
	_ = r.ParseForm()
	desc := strings.TrimSpace(r.Form.Get("description"))

	st := store.Store{Dir: s.dir()}
	db, err := st.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := db.FindActor(actorID); !ok {
		http.Error(w, "unknown actor", http.StatusForbidden)
		return
	}
	o, ok := db.FindOutline(outlineID)
	if !ok || o == nil || o.Archived {
		http.NotFound(w, r)
		return
	}

	if strings.TrimSpace(o.Description) != desc {
		o.Description = desc
		if err := st.AppendEvent(actorID, "outline.set_description", o.ID, map[string]any{"description": o.Description}); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err := st.Save(db); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if c := s.committer(); c != nil {
			actorLabel := strings.TrimSpace(actorID)
			if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
				actorLabel = strings.TrimSpace(a.Name)
			}
			c.Notify(actorLabel)
		}
	}

	redirectBack(w, r, "/outlines/"+outlineID)
}

type outlineStatusAddReq struct {
	Label        string `json:"label"`
	IsEndState   bool   `json:"isEndState"`
	RequiresNote bool   `json:"requiresNote"`
}

func (s *Server) handleOutlineStatusAdd(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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
	var req outlineStatusAddReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		http.Error(w, "missing label", http.StatusBadRequest)
		return
	}

	st := store.Store{Dir: s.dir()}
	db, err := st.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := db.FindActor(actorID); !ok {
		http.Error(w, "unknown actor", http.StatusForbidden)
		return
	}
	o, ok := db.FindOutline(outlineID)
	if !ok || o == nil || o.Archived {
		http.NotFound(w, r)
		return
	}

	id := store.NewStatusIDFromLabel(o, label)
	o.StatusDefs = append(o.StatusDefs, model.OutlineStatusDef{
		ID:           id,
		Label:        label,
		IsEndState:   req.IsEndState,
		RequiresNote: req.RequiresNote,
	})
	if err := st.AppendEvent(actorID, "outline.status.add", o.ID, map[string]any{
		"id":           id,
		"label":        label,
		"isEndState":   req.IsEndState,
		"requiresNote": req.RequiresNote,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := st.Save(db); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if c := s.committer(); c != nil {
		actorLabel := strings.TrimSpace(actorID)
		if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
			actorLabel = strings.TrimSpace(a.Name)
		}
		c.Notify(actorLabel)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
}

type outlineStatusUpdateReq struct {
	ID           string `json:"id"`
	Label        string `json:"label,omitempty"`
	IsEndState   *bool  `json:"isEndState,omitempty"`
	RequiresNote *bool  `json:"requiresNote,omitempty"`
}

func (s *Server) handleOutlineStatusUpdate(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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
	var req outlineStatusUpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	st := store.Store{Dir: s.dir()}
	db, err := st.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := db.FindActor(actorID); !ok {
		http.Error(w, "unknown actor", http.StatusForbidden)
		return
	}
	o, ok := db.FindOutline(outlineID)
	if !ok || o == nil || o.Archived {
		http.NotFound(w, r)
		return
	}

	idx := -1
	for i := range o.StatusDefs {
		if strings.TrimSpace(o.StatusDefs[i].ID) == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		http.Error(w, "unknown status id", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Label) != "" {
		o.StatusDefs[idx].Label = strings.TrimSpace(req.Label)
	}
	if req.IsEndState != nil {
		o.StatusDefs[idx].IsEndState = *req.IsEndState
	}
	if req.RequiresNote != nil {
		o.StatusDefs[idx].RequiresNote = *req.RequiresNote
	}

	payload := map[string]any{"id": id}
	if strings.TrimSpace(req.Label) != "" {
		payload["label"] = strings.TrimSpace(req.Label)
	}
	if req.IsEndState != nil {
		payload["isEndState"] = *req.IsEndState
	}
	if req.RequiresNote != nil {
		payload["requiresNote"] = *req.RequiresNote
	}
	if err := st.AppendEvent(actorID, "outline.status.update", o.ID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := st.Save(db); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if c := s.committer(); c != nil {
		actorLabel := strings.TrimSpace(actorID)
		if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
			actorLabel = strings.TrimSpace(a.Name)
		}
		c.Notify(actorLabel)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

type outlineStatusRemoveReq struct {
	ID string `json:"id"`
}

func (s *Server) handleOutlineStatusRemove(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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
	var req outlineStatusRemoveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	st := store.Store{Dir: s.dir()}
	db, err := st.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := db.FindActor(actorID); !ok {
		http.Error(w, "unknown actor", http.StatusForbidden)
		return
	}
	o, ok := db.FindOutline(outlineID)
	if !ok || o == nil || o.Archived {
		http.NotFound(w, r)
		return
	}

	next := make([]model.OutlineStatusDef, 0, len(o.StatusDefs))
	for _, d := range o.StatusDefs {
		if strings.TrimSpace(d.ID) == id {
			continue
		}
		next = append(next, d)
	}
	o.StatusDefs = next

	if err := st.AppendEvent(actorID, "outline.status.remove", o.ID, map[string]any{"id": id}); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := st.Save(db); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if c := s.committer(); c != nil {
		actorLabel := strings.TrimSpace(actorID)
		if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
			actorLabel = strings.TrimSpace(a.Name)
		}
		c.Notify(actorLabel)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

type outlineStatusReorderReq struct {
	Labels []string `json:"labels"`
}

func (s *Server) handleOutlineStatusReorder(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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
	var req outlineStatusReorderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	st := store.Store{Dir: s.dir()}
	db, err := st.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := db.FindActor(actorID); !ok {
		http.Error(w, "unknown actor", http.StatusForbidden)
		return
	}
	o, ok := db.FindOutline(outlineID)
	if !ok || o == nil || o.Archived {
		http.NotFound(w, r)
		return
	}

	labels := make([]string, 0, len(req.Labels))
	seen := map[string]bool{}
	for _, l := range req.Labels {
		l = strings.TrimSpace(l)
		if l == "" || seen[l] {
			continue
		}
		labels = append(labels, l)
		seen[l] = true
	}
	if err := st.AppendEvent(actorID, "outline.status.reorder", o.ID, map[string]any{"labels": labels}); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	byLabel := map[string]model.OutlineStatusDef{}
	for _, d := range o.StatusDefs {
		byLabel[strings.TrimSpace(d.Label)] = d
	}
	out := make([]model.OutlineStatusDef, 0, len(o.StatusDefs))
	seen2 := map[string]bool{}
	for _, l := range labels {
		if d, ok := byLabel[l]; ok {
			out = append(out, d)
			seen2[l] = true
		}
	}
	for _, d := range o.StatusDefs {
		l := strings.TrimSpace(d.Label)
		if l == "" || seen2[l] {
			continue
		}
		out = append(out, d)
	}
	o.StatusDefs = out

	if err := st.Save(db); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if c := s.committer(); c != nil {
		actorLabel := strings.TrimSpace(actorID)
		if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
			actorLabel = strings.TrimSpace(a.Name)
		}
		c.Notify(actorLabel)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleOutlineItemCreate(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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

	st := store.Store{Dir: s.dir()}
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
	if c := s.committer(); c != nil {
		actorLabel := strings.TrimSpace(actorID)
		if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
			actorLabel = strings.TrimSpace(a.Name)
		}
		c.Notify(actorLabel)
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

func outlineComponentPayload(db *store.DB, o model.Outline, actorID string) (items template.HTMLAttr, statusLabels template.HTMLAttr, assignees template.HTMLAttr, tags template.HTMLAttr) {
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
	return template.HTMLAttr(bTodos), template.HTMLAttr(bStatus), template.HTMLAttr(bAssignees), template.HTMLAttr(bTags)
}

type outlineApplyReq struct {
	// Single-op request (legacy / simple clients).
	Type   string          `json:"type"`
	Detail json.RawMessage `json:"detail"`
	// Batch request (preferred for debounced outline moves).
	Ops []outlineApplyOp `json:"ops"`
}

type outlineApplyOp struct {
	Type   string          `json:"type"`
	Detail json.RawMessage `json:"detail"`
}

type outlineApplyResp struct {
	Items        json.RawMessage `json:"items"`
	StatusLabels json.RawMessage `json:"statusLabels"`
	Assignees    json.RawMessage `json:"assignees"`
	Tags         json.RawMessage `json:"tags"`
	Created      []createdItem   `json:"created,omitempty"`
}

type createdItem struct {
	TempID string `json:"tempId,omitempty"`
	ID     string `json:"id"`
}

func (s *Server) handleOutlineApply(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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
	for i := range req.Ops {
		req.Ops[i].Type = strings.TrimSpace(req.Ops[i].Type)
	}

	st := store.Store{Dir: s.dir()}
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

	ops := make([]outlineApplyOp, 0, 1+len(req.Ops))
	if len(req.Ops) > 0 {
		ops = append(ops, req.Ops...)
	} else {
		if req.Type == "" {
			http.Error(w, "missing type", http.StatusBadRequest)
			return
		}
		ops = append(ops, outlineApplyOp{Type: req.Type, Detail: req.Detail})
	}

	changed := false
	created := make([]createdItem, 0)
	now := time.Now().UTC()
	for _, op := range ops {
		if op.Type == "" {
			http.Error(w, "missing type", http.StatusBadRequest)
			return
		}

		switch op.Type {
		case "outline:new_sibling":
			var d struct {
				Title   string `json:"title"`
				AfterID string `json:"afterId"`
				TempID  string `json:"tempId"`
				// Compatibility aliases.
				Text  string `json:"text"`
				ForID string `json:"forId"`
			}
			if err := json.Unmarshal(op.Detail, &d); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			title := strings.TrimSpace(d.Title)
			if title == "" {
				title = strings.TrimSpace(d.Text)
			}
			afterID := strings.TrimSpace(d.AfterID)
			if afterID == "" {
				afterID = strings.TrimSpace(d.ForID)
			}
			if title == "" {
				http.Error(w, "missing title", http.StatusBadRequest)
				return
			}

			parentPtr := (*string)(nil)
			if afterID != "" {
				ref, ok := db.FindItem(afterID)
				if !ok || ref == nil || ref.Archived || ref.OutlineID != outlineID {
					http.NotFound(w, r)
					return
				}
				parentPtr = ref.ParentID
			}
			rank := nextAppendRank(db, outlineID, parentPtr)
			if afterID != "" {
				if r2, ok := rankAfterSiblingWeb(db, outlineID, parentPtr, afterID); ok {
					rank = r2
				}
			}

			statusID := store.FirstStatusID(o.StatusDefs)
			if strings.TrimSpace(statusID) == "" {
				statusID = "todo"
			}
			assigned := defaultAssignedActorIDWeb(db, actorID)
			itemID := st.NextID(db, "item")
			it := model.Item{
				ID:                itemID,
				ProjectID:         o.ProjectID,
				OutlineID:         o.ID,
				ParentID:          parentPtr,
				Rank:              rank,
				Title:             title,
				Description:       "",
				StatusID:          statusID,
				Priority:          false,
				OnHold:            false,
				Due:               nil,
				Schedule:          nil,
				LegacyDueAt:       nil,
				LegacyScheduledAt: nil,
				Tags:              nil,
				Archived:          false,
				OwnerActorID:      actorID,
				AssignedActorID:   assigned,
				CreatedBy:         actorID,
				CreatedAt:         now,
				UpdatedAt:         now,
			}
			db.Items = append(db.Items, it)
			if err := st.AppendEvent(actorID, "item.create", it.ID, it); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			if strings.TrimSpace(d.TempID) != "" {
				created = append(created, createdItem{TempID: strings.TrimSpace(d.TempID), ID: itemID})
			} else {
				created = append(created, createdItem{ID: itemID})
			}
			changed = true

		case "outline:new_child":
			var d struct {
				Title    string `json:"title"`
				ParentID string `json:"parentId"`
				TempID   string `json:"tempId"`
				// Compatibility aliases.
				Text  string `json:"text"`
				ForID string `json:"forId"`
			}
			if err := json.Unmarshal(op.Detail, &d); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			title := strings.TrimSpace(d.Title)
			if title == "" {
				title = strings.TrimSpace(d.Text)
			}
			pid := strings.TrimSpace(d.ParentID)
			if pid == "" {
				pid = strings.TrimSpace(d.ForID)
			}
			if title == "" {
				http.Error(w, "missing title", http.StatusBadRequest)
				return
			}
			if pid == "" {
				http.Error(w, "missing parentId", http.StatusBadRequest)
				return
			}
			pit, ok := db.FindItem(pid)
			if !ok || pit == nil || pit.Archived || pit.OutlineID != outlineID {
				http.NotFound(w, r)
				return
			}
			parentPtr := &pid

			statusID := store.FirstStatusID(o.StatusDefs)
			if strings.TrimSpace(statusID) == "" {
				statusID = "todo"
			}
			assigned := defaultAssignedActorIDWeb(db, actorID)
			rank := nextAppendRank(db, outlineID, parentPtr)

			itemID := st.NextID(db, "item")
			it := model.Item{
				ID:                itemID,
				ProjectID:         o.ProjectID,
				OutlineID:         o.ID,
				ParentID:          parentPtr,
				Rank:              rank,
				Title:             title,
				Description:       "",
				StatusID:          statusID,
				Priority:          false,
				OnHold:            false,
				Due:               nil,
				Schedule:          nil,
				LegacyDueAt:       nil,
				LegacyScheduledAt: nil,
				Tags:              nil,
				Archived:          false,
				OwnerActorID:      actorID,
				AssignedActorID:   assigned,
				CreatedBy:         actorID,
				CreatedAt:         now,
				UpdatedAt:         now,
			}
			db.Items = append(db.Items, it)
			if err := st.AppendEvent(actorID, "item.create", it.ID, it); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			if strings.TrimSpace(d.TempID) != "" {
				created = append(created, createdItem{TempID: strings.TrimSpace(d.TempID), ID: itemID})
			} else {
				created = append(created, createdItem{ID: itemID})
			}
			changed = true

		case "outline:edit:save":
			var d struct {
				ID      string `json:"id"`
				NewText string `json:"newText"`
				Text    string `json:"text"`
			}
			if err := json.Unmarshal(op.Detail, &d); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			itemID := strings.TrimSpace(d.ID)
			title := strings.TrimSpace(d.NewText)
			if title == "" {
				// Compatibility: older client payload used `text`.
				title = strings.TrimSpace(d.Text)
			}
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

		case "outline:set_description":
			var d struct {
				ID          string `json:"id"`
				Description string `json:"description"`
			}
			if err := json.Unmarshal(op.Detail, &d); err != nil {
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
			if !perm.CanEditItem(db, actorID, it) {
				http.Error(w, "owner-only", http.StatusForbidden)
				return
			}
			if it.Description != d.Description {
				it.Description = d.Description
				it.UpdatedAt = now
				if err := st.AppendEvent(actorID, "item.set_description", it.ID, map[string]any{"description": it.Description}); err != nil {
					http.Error(w, err.Error(), http.StatusConflict)
					return
				}
				changed = true
			}

		case "outline:toggle_priority":
			var d struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(op.Detail, &d); err != nil {
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
			if !perm.CanEditItem(db, actorID, it) {
				http.Error(w, "owner-only", http.StatusForbidden)
				return
			}
			it.Priority = !it.Priority
			it.UpdatedAt = now
			if err := st.AppendEvent(actorID, "item.set_priority", it.ID, map[string]any{"priority": it.Priority}); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			changed = true

		case "outline:toggle_on_hold":
			var d struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(op.Detail, &d); err != nil {
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
			if !perm.CanEditItem(db, actorID, it) {
				http.Error(w, "owner-only", http.StatusForbidden)
				return
			}
			it.OnHold = !it.OnHold
			it.UpdatedAt = now
			if err := st.AppendEvent(actorID, "item.set_on_hold", it.ID, map[string]any{"onHold": it.OnHold}); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			changed = true

		case "outline:set_due":
			var d struct {
				ID   string `json:"id"`
				Date string `json:"date"`
				Time string `json:"time"`
			}
			if err := json.Unmarshal(op.Detail, &d); err != nil {
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
			if !perm.CanEditItem(db, actorID, it) {
				http.Error(w, "owner-only", http.StatusForbidden)
				return
			}
			dt, err := parseDateTimeWeb(d.Date, d.Time)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if !sameDateTime(it.Due, dt) {
				it.Due = dt
				it.UpdatedAt = now
				if err := st.AppendEvent(actorID, "item.set_due", it.ID, map[string]any{"due": it.Due}); err != nil {
					http.Error(w, err.Error(), http.StatusConflict)
					return
				}
				changed = true
			}

		case "outline:set_schedule":
			var d struct {
				ID   string `json:"id"`
				Date string `json:"date"`
				Time string `json:"time"`
			}
			if err := json.Unmarshal(op.Detail, &d); err != nil {
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
			if !perm.CanEditItem(db, actorID, it) {
				http.Error(w, "owner-only", http.StatusForbidden)
				return
			}
			dt, err := parseDateTimeWeb(d.Date, d.Time)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if !sameDateTime(it.Schedule, dt) {
				it.Schedule = dt
				it.UpdatedAt = now
				if err := st.AppendEvent(actorID, "item.set_schedule", it.ID, map[string]any{"schedule": it.Schedule}); err != nil {
					http.Error(w, err.Error(), http.StatusConflict)
					return
				}
				changed = true
			}

		case "outline:set_tags":
			var d struct {
				ID   string   `json:"id"`
				Tags []string `json:"tags"`
			}
			if err := json.Unmarshal(op.Detail, &d); err != nil {
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
			if !perm.CanEditItem(db, actorID, it) {
				http.Error(w, "owner-only", http.StatusForbidden)
				return
			}
			next := uniqueSortedStringsWeb(normalizeTagsWeb(d.Tags))
			cur := uniqueSortedStringsWeb(normalizeTagsWeb(it.Tags))
			if equalStrings(cur, next) {
				break
			}
			curSet := map[string]bool{}
			nextSet := map[string]bool{}
			for _, t := range cur {
				curSet[t] = true
			}
			for _, t := range next {
				nextSet[t] = true
			}
			for _, t := range cur {
				if !nextSet[t] {
					it.Tags = removeString(it.Tags, t)
					it.Tags = uniqueSortedStringsWeb(normalizeTagsWeb(it.Tags))
					it.UpdatedAt = now
					if err := st.AppendEvent(actorID, "item.tags_remove", it.ID, map[string]any{"tag": t}); err != nil {
						http.Error(w, err.Error(), http.StatusConflict)
						return
					}
					changed = true
				}
			}
			for _, t := range next {
				if !curSet[t] {
					it.Tags = append(it.Tags, t)
					it.Tags = uniqueSortedStringsWeb(normalizeTagsWeb(it.Tags))
					it.UpdatedAt = now
					if err := st.AppendEvent(actorID, "item.tags_add", it.ID, map[string]any{"tag": t}); err != nil {
						http.Error(w, err.Error(), http.StatusConflict)
						return
					}
					changed = true
				}
			}

		case "outline:archive":
			var d struct {
				ID       string `json:"id"`
				Archived *bool  `json:"archived"`
			}
			if err := json.Unmarshal(op.Detail, &d); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			itemID := strings.TrimSpace(d.ID)
			if itemID == "" {
				http.Error(w, "missing id", http.StatusBadRequest)
				return
			}
			it, ok := db.FindItem(itemID)
			if !ok || it == nil || it.OutlineID != outlineID {
				http.NotFound(w, r)
				return
			}
			target := true
			if d.Archived != nil {
				target = *d.Archived
			}
			res, err := mutate.SetItemArchived(db, actorID, it.ID, target)
			if err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			if res.Changed {
				it.UpdatedAt = now
				if err := st.AppendEvent(actorID, "item.archive", it.ID, res.EventPayload); err != nil {
					http.Error(w, err.Error(), http.StatusConflict)
					return
				}
				changed = true
			}

		case "outline:toggle":
			var d struct {
				ID string `json:"id"`
				To string `json:"to"`
				// Compatibility: older client payload used `status`.
				Status string `json:"status"`
				Note   string `json:"note"`
			}
			if err := json.Unmarshal(op.Detail, &d); err != nil {
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
			to := strings.TrimSpace(d.To)
			if to == "" {
				to = strings.TrimSpace(d.Status)
			}
			statusID, err := statusIDFromOutlineToggle(*o, to)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			var note *string
			if strings.TrimSpace(d.Note) != "" {
				n := strings.TrimSpace(d.Note)
				note = &n
			}
			res, err := mutate.SetItemStatus(db, actorID, it.ID, statusID, note)
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

		case "outline:set_assign":
			var d struct {
				ID              string  `json:"id"`
				AssignedActorID *string `json:"assignedActorId"`
				Assigned        *string `json:"assigned"`
			}
			if err := json.Unmarshal(op.Detail, &d); err != nil {
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
			target := d.AssignedActorID
			if target == nil {
				target = d.Assigned
			}
			if target != nil && strings.TrimSpace(*target) == "" {
				target = nil
			}
			res, err := mutate.SetAssignedActor(db, actorID, it.ID, target, mutate.AssignOpts{TakeAssigned: false})
			if err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			if res.Changed {
				it.UpdatedAt = now
				if err := st.AppendEvent(actorID, "item.set_assign", it.ID, res.EventPayload); err != nil {
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
			if err := json.Unmarshal(op.Detail, &d); err != nil {
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

		case "outline:move_outline":
			var d struct {
				ID                      string `json:"id"`
				ToOutlineID             string `json:"toOutlineId"`
				To                      string `json:"to"`
				StatusOverride          string `json:"status"`
				SetStatus               string `json:"setStatus"`
				ApplyStatusToInvalidSub bool   `json:"applyStatusToInvalidSubtree"`
			}
			if err := json.Unmarshal(op.Detail, &d); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			itemID := strings.TrimSpace(d.ID)
			if itemID == "" {
				http.Error(w, "missing id", http.StatusBadRequest)
				return
			}
			to := strings.TrimSpace(d.ToOutlineID)
			if to == "" {
				to = strings.TrimSpace(d.To)
			}
			if to == "" {
				http.Error(w, "missing toOutlineId", http.StatusBadRequest)
				return
			}
			it, ok := db.FindItem(itemID)
			if !ok || it == nil || it.Archived || it.OutlineID != outlineID {
				http.NotFound(w, r)
				return
			}
			statusOverride := strings.TrimSpace(d.StatusOverride)
			if statusOverride == "" {
				statusOverride = strings.TrimSpace(d.SetStatus)
			}
			changed2, payload, err := mutate.MoveItemToOutline(db, actorID, itemID, to, statusOverride, d.ApplyStatusToInvalidSub, now)
			if err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			if changed2 {
				if err := st.AppendEvent(actorID, "item.move_outline", itemID, payload); err != nil {
					http.Error(w, err.Error(), http.StatusConflict)
					return
				}
				changed = true
			}

		default:
			http.Error(w, "unsupported type: "+op.Type, http.StatusBadRequest)
			return
		}
	}

	if changed {
		if err := st.Save(db); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if c := s.committer(); c != nil {
			actorLabel := strings.TrimSpace(actorID)
			if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
				actorLabel = strings.TrimSpace(a.Name)
			}
			c.Notify(actorLabel)
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
		Created:      created,
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

func defaultAssignedActorIDWeb(db *store.DB, actorID string) *string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" || db == nil {
		return nil
	}
	a, ok := db.FindActor(actorID)
	if !ok || a == nil {
		return nil
	}
	if a.Kind == model.ActorKindAgent {
		tmp := actorID
		return &tmp
	}
	return nil
}

func normalizeTagWeb(tag string) string {
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(tag, "#")
	return strings.TrimSpace(tag)
}

func normalizeTagsWeb(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = normalizeTagWeb(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseTagsInputWeb(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\r', ',':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = normalizeTagWeb(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func uniqueSortedStringsWeb(xs []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool {
		ai := strings.ToLower(out[i])
		aj := strings.ToLower(out[j])
		if ai == aj {
			return out[i] < out[j]
		}
		return ai < aj
	})
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func removeString(xs []string, s string) []string {
	s = strings.TrimSpace(s)
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if strings.TrimSpace(x) == s {
			continue
		}
		out = append(out, x)
	}
	return out
}

func parseDateTimeWeb(date, timeStr string) (*model.DateTime, error) {
	date = strings.TrimSpace(date)
	timeStr = strings.TrimSpace(timeStr)
	if date == "" {
		return nil, nil
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, errors.New("invalid date (expected YYYY-MM-DD)")
	}
	if timeStr == "" {
		return &model.DateTime{Date: date, Time: nil}, nil
	}
	if _, err := time.Parse("15:04", timeStr); err != nil {
		return nil, errors.New("invalid time (expected HH:MM, 24h)")
	}
	tmp := timeStr
	return &model.DateTime{Date: date, Time: &tmp}, nil
}

func sameDateTime(a, b *model.DateTime) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if strings.TrimSpace(a.Date) != strings.TrimSpace(b.Date) {
		return false
	}
	at := ""
	bt := ""
	if a.Time != nil {
		at = strings.TrimSpace(*a.Time)
	}
	if b.Time != nil {
		bt = strings.TrimSpace(*b.Time)
	}
	return at == bt
}

func rankAfterSiblingWeb(db *store.DB, outlineID string, parentPtr *string, afterID string) (string, bool) {
	afterID = strings.TrimSpace(afterID)
	if afterID == "" || db == nil {
		return "", false
	}
	sibs := siblingItemsWeb(db, outlineID, parentPtr, "")
	idx := indexOfItemWeb(sibs, afterID)
	if idx < 0 {
		return "", false
	}
	lower := strings.TrimSpace(sibs[idx].Rank)
	upper := ""
	if idx+1 < len(sibs) {
		upper = strings.TrimSpace(sibs[idx+1].Rank)
	}
	if r, err := store.RankBetween(lower, upper); err == nil && strings.TrimSpace(r) != "" {
		return r, true
	}
	if r, err := store.RankAfter(lower); err == nil && strings.TrimSpace(r) != "" {
		return r, true
	}
	if lower != "" {
		return lower + "0", true
	}
	return "h", true
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
	baseVM
	Item          model.Item
	AssignedID    string
	AssignedLabel string
	StatusLabel   string
	ActorOptions  []actorOption
	Comments      []itemCommentVM
	ReplyTo       string
	Worklog       []itemWorklogVM

	DescriptionHTML template.HTML

	TagsInput          string
	DueDate            string
	DueTime            string
	SchDate            string
	SchTime            string
	StatusOptionsJSON  template.HTMLAttr
	ActorOptionsJSON   template.HTMLAttr
	OutlineOptionsJSON template.HTMLAttr

	ParentRow    *itemOutlineRowVM
	Children     []itemOutlineRowVM
	ChildrenMore int

	CommentsCount int
	LastComment   string

	WorklogCount int
	LastWorklog  string

	HistoryCount int
	LastHistory  string
	History      []itemHistoryRowVM

	CanEdit        bool
	StatusDefs     []model.OutlineStatusDef
	ErrorMessage   string
	SuccessMessage string
}

type itemCommentVM struct {
	ID               string
	AuthorID         string
	AuthorLabel      string
	ReplyToCommentID *string
	CreatedAt        time.Time
	BodyHTML         template.HTML
}

type itemWorklogVM struct {
	ID        string
	CreatedAt time.Time
	BodyHTML  template.HTML
}

type itemOutlineRowVM struct {
	ID            string
	Title         string
	StatusID      string
	StatusLabel   string
	IsEndState    bool
	AssignedLabel string
	Priority      bool
	OnHold        bool
	DueDate       string
	DueTime       string
	SchDate       string
	SchTime       string
	Tags          []string
	HasChildren   bool
	DoneChildren  int
	TotalChildren int
}

type itemHistoryRowVM struct {
	TS   string
	Type string
}

func itemOutlineRowVMFromItem(db *store.DB, outline *model.Outline, statusLabelByID map[string]string, it model.Item) itemOutlineRowVM {
	statusID := strings.TrimSpace(it.StatusID)
	statusLabel := statusID
	if statusLabelByID != nil {
		if mapped, ok := statusLabelByID[statusID]; ok && strings.TrimSpace(mapped) != "" {
			statusLabel = strings.TrimSpace(mapped)
		}
	}
	if statusLabel == "" {
		statusLabel = "(none)"
	}

	assignedLabel := ""
	if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) != "" {
		assignedLabel = actorDisplayLabel(db, strings.TrimSpace(*it.AssignedActorID))
	}

	dueDate := ""
	dueTime := ""
	if it.Due != nil {
		dueDate = strings.TrimSpace(it.Due.Date)
		if it.Due.Time != nil {
			dueTime = strings.TrimSpace(*it.Due.Time)
		}
	}
	schDate := ""
	schTime := ""
	if it.Schedule != nil {
		schDate = strings.TrimSpace(it.Schedule.Date)
		if it.Schedule.Time != nil {
			schTime = strings.TrimSpace(*it.Schedule.Time)
		}
	}

	doneChildren := 0
	totalChildren := 0
	kids := db.ChildrenOf(it.ID)
	if len(kids) > 0 {
		totalChildren = len(kids)
		for _, ch := range kids {
			if outline != nil && statusutil.IsEndState(*outline, ch.StatusID) {
				doneChildren++
			}
		}
	}

	isEnd := false
	if outline != nil {
		isEnd = statusutil.IsEndState(*outline, it.StatusID)
	}

	return itemOutlineRowVM{
		ID:            it.ID,
		Title:         it.Title,
		StatusID:      it.StatusID,
		StatusLabel:   statusLabel,
		IsEndState:    isEnd,
		AssignedLabel: assignedLabel,
		Priority:      it.Priority,
		OnHold:        it.OnHold,
		DueDate:       dueDate,
		DueTime:       dueTime,
		SchDate:       schDate,
		SchTime:       schTime,
		Tags:          it.Tags,
		HasChildren:   totalChildren > 0,
		DoneChildren:  doneChildren,
		TotalChildren: totalChildren,
	}
}

func itemHistoryRowsForItem(dir string, itemID string, limit int) (rows []itemHistoryRowVM, last string) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, ""
	}
	if limit <= 0 {
		limit = 200
	}
	evs, err := store.ReadEventsTail(dir, limit)
	if err != nil || len(evs) == 0 {
		return nil, ""
	}
	filtered := make([]model.Event, 0, len(evs))
	for _, ev := range evs {
		if strings.TrimSpace(ev.EntityID) == itemID {
			filtered = append(filtered, ev)
			continue
		}
		// comment.add / worklog.add store itemId in payload; include those too.
		if p, ok := ev.Payload.(map[string]any); ok {
			if v, ok := p["itemId"].(string); ok && strings.TrimSpace(v) == itemID {
				filtered = append(filtered, ev)
			}
		}
	}
	if len(filtered) == 0 {
		return nil, ""
	}
	// Newest first (events tail is newest-first already, but keep stable).
	rows = make([]itemHistoryRowVM, 0, len(filtered))
	for _, ev := range filtered {
		ts := ev.TS.UTC().Format(time.RFC3339)
		rows = append(rows, itemHistoryRowVM{TS: ts, Type: strings.TrimSpace(ev.Type)})
	}
	last = rows[0].TS
	return rows, last
}

func (s *Server) handleItem(w http.ResponseWriter, r *http.Request) {
	itemID := strings.TrimSpace(r.PathValue("itemId"))
	if itemID == "" {
		http.Error(w, "missing item id", http.StatusBadRequest)
		return
	}

	dir := s.dir()
	db, err := (store.Store{Dir: dir}).Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	it, ok := db.FindItem(itemID)
	if !ok || it == nil {
		http.NotFound(w, r)
		return
	}
	itemReadOnly := it.Archived

	comments := db.CommentsForItem(itemID)
	actorID := strings.TrimSpace(s.actorForRequest(r))
	canEdit := !itemReadOnly && actorID != "" && perm.CanEditItem(db, actorID, it)
	statusDefs := []model.OutlineStatusDef{}
	statusOptionsJSON := template.HTMLAttr("[]")
	var outline *model.Outline
	if o, ok := db.FindOutline(it.OutlineID); ok && o != nil {
		outline = o
		statusDefs = o.StatusDefs
		statusOptionsJSON = outlineStatusOptionsJSON(*o)
	}
	statusLabelByID := map[string]string{}
	if outline != nil {
		for _, sd := range outline.StatusDefs {
			sid := strings.TrimSpace(sd.ID)
			if sid == "" {
				continue
			}
			lbl := strings.TrimSpace(sd.Label)
			if lbl == "" {
				lbl = sid
			}
			statusLabelByID[sid] = lbl
		}
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

	commentVMs := make([]itemCommentVM, 0, len(comments))
	for _, c := range comments {
		authorID := strings.TrimSpace(c.AuthorID)
		authorLabel := actorDisplayLabel(db, authorID)
		commentVMs = append(commentVMs, itemCommentVM{
			ID:               c.ID,
			AuthorID:         authorID,
			AuthorLabel:      authorLabel,
			ReplyToCommentID: c.ReplyToCommentID,
			CreatedAt:        c.CreatedAt,
			BodyHTML:         renderMarkdownHTML(c.Body),
		})
	}

	worklogVMs := make([]itemWorklogVM, 0, len(worklog))
	for _, wle := range worklog {
		worklogVMs = append(worklogVMs, itemWorklogVM{
			ID:        wle.ID,
			CreatedAt: wle.CreatedAt,
			BodyHTML:  renderMarkdownHTML(wle.Body),
		})
	}
	commentsCount := len(comments)
	lastComment := ""
	for _, c := range comments {
		if lastComment == "" || c.CreatedAt.UTC().Format(time.RFC3339) > lastComment {
			lastComment = c.CreatedAt.UTC().Format(time.RFC3339)
		}
	}
	worklogCount := len(worklog)
	lastWorklog := ""
	for _, wle := range worklog {
		if lastWorklog == "" || wle.CreatedAt.UTC().Format(time.RFC3339) > lastWorklog {
			lastWorklog = wle.CreatedAt.UTC().Format(time.RFC3339)
		}
	}
	historyRows, lastHistory := itemHistoryRowsForItem(dir, itemID, 250)

	assignedID := ""
	if it.AssignedActorID != nil {
		assignedID = strings.TrimSpace(*it.AssignedActorID)
	}
	assignedLabel := ""
	if assignedID != "" {
		assignedLabel = actorDisplayLabel(db, assignedID)
	}
	statusLabel := strings.TrimSpace(it.StatusID)
	if mapped, ok := statusLabelByID[statusLabel]; ok && strings.TrimSpace(mapped) != "" {
		statusLabel = strings.TrimSpace(mapped)
	}
	if statusLabel == "" {
		statusLabel = "(none)"
	}
	tagsInput := ""
	if len(it.Tags) > 0 {
		parts := make([]string, 0, len(it.Tags))
		for _, t := range it.Tags {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			parts = append(parts, "#"+t)
		}
		tagsInput = strings.Join(parts, " ")
	}
	dueDate := ""
	dueTime := ""
	if it.Due != nil {
		dueDate = strings.TrimSpace(it.Due.Date)
		if it.Due.Time != nil {
			dueTime = strings.TrimSpace(*it.Due.Time)
		}
	}
	schDate := ""
	schTime := ""
	if it.Schedule != nil {
		schDate = strings.TrimSpace(it.Schedule.Date)
		if it.Schedule.Time != nil {
			schTime = strings.TrimSpace(*it.Schedule.Time)
		}
	}
	vm := itemVM{
		baseVM:             s.baseVMForRequest(r, "/items/"+it.ID+"/events?view=item"),
		Item:               *it,
		AssignedID:         assignedID,
		AssignedLabel:      assignedLabel,
		StatusLabel:        statusLabel,
		Comments:           commentVMs,
		ReplyTo:            replyTo,
		Worklog:            worklogVMs,
		DescriptionHTML:    renderMarkdownHTML(it.Description),
		TagsInput:          tagsInput,
		DueDate:            dueDate,
		DueTime:            dueTime,
		SchDate:            schDate,
		SchTime:            schTime,
		StatusOptionsJSON:  statusOptionsJSON,
		ActorOptionsJSON:   actorOptionsJSON(db),
		OutlineOptionsJSON: outlineOptionsJSON(db),
		CanEdit:            canEdit,
		StatusDefs:         statusDefs,
		ErrorMessage:       errMsg,
		SuccessMessage:     okMsg,
		CommentsCount:      commentsCount,
		LastComment:        lastComment,
		WorklogCount:       worklogCount,
		LastWorklog:        lastWorklog,
		HistoryCount:       len(historyRows),
		LastHistory:        lastHistory,
		History:            historyRows,
	}
	vm.ActorID = actorID

	// Parent + Children (TUI parity).
	if it.ParentID != nil && strings.TrimSpace(*it.ParentID) != "" {
		if pit, ok := db.FindItem(strings.TrimSpace(*it.ParentID)); ok && pit != nil && !pit.Archived {
			row := itemOutlineRowVMFromItem(db, outline, statusLabelByID, *pit)
			vm.ParentRow = &row
		}
	}
	{
		kids := db.ChildrenOf(it.ID)
		kptrs := make([]*model.Item, 0, len(kids))
		for i := range kids {
			if kids[i].Archived {
				continue
			}
			kptrs = append(kptrs, &kids[i])
		}
		store.SortItemsByRankOrder(kptrs)
		maxRows := 8
		if len(kptrs) > maxRows {
			vm.ChildrenMore = len(kptrs) - maxRows
			kptrs = kptrs[:maxRows]
		}
		vm.Children = make([]itemOutlineRowVM, 0, len(kptrs))
		for _, p := range kptrs {
			if p == nil {
				continue
			}
			vm.Children = append(vm.Children, itemOutlineRowVMFromItem(db, outline, statusLabelByID, *p))
		}
	}
	if itemReadOnly {
		vm.ReadOnly = true
	} else {
		// Record "recent item visits" (TUI parity). Best-effort local-only state.
		st, err := (store.Store{Dir: dir}).LoadTUIState()
		if err == nil && st != nil {
			id := strings.TrimSpace(it.ID)
			next := make([]string, 0, 5)
			if id != "" {
				next = append(next, id)
			}
			for _, cur := range st.RecentItemIDs {
				cur = strings.TrimSpace(cur)
				if cur == "" || cur == id {
					continue
				}
				next = append(next, cur)
				if len(next) >= 5 {
					break
				}
			}
			st.RecentItemIDs = next
			_ = (store.Store{Dir: dir}).SaveTUIState(st)
		}
	}
	vm.ActorOptions = nil
	if db != nil {
		opts := make([]actorOption, 0, len(db.Actors))
		for _, a := range db.Actors {
			id := strings.TrimSpace(a.ID)
			if id == "" {
				continue
			}
			opts = append(opts, actorOption{ID: id, Label: actorDisplayLabel(db, id), Kind: string(a.Kind)})
		}
		sort.Slice(opts, func(i, j int) bool {
			if opts[i].Kind != opts[j].Kind {
				if opts[i].Kind == string(model.ActorKindHuman) {
					return true
				}
				if opts[j].Kind == string(model.ActorKindHuman) {
					return false
				}
				return opts[i].Kind < opts[j].Kind
			}
			if opts[i].Label != opts[j].Label {
				return opts[i].Label < opts[j].Label
			}
			return opts[i].ID < opts[j].ID
		})
		vm.ActorOptions = opts
	}
	s.writeHTMLTemplate(w, "item.html", vm)
}

func (s *Server) handleItemPreview(w http.ResponseWriter, r *http.Request) {
	itemID := strings.TrimSpace(r.PathValue("itemId"))
	if itemID == "" {
		http.Error(w, "missing item id", http.StatusBadRequest)
		return
	}

	db, err := (store.Store{Dir: s.dir()}).Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	it, ok := db.FindItem(itemID)
	if !ok || it == nil || it.Archived {
		http.NotFound(w, r)
		return
	}

	actorID := strings.TrimSpace(s.actorForRequest(r))
	canEdit := actorID != "" && perm.CanEditItem(db, actorID, it)
	statusDefs := []model.OutlineStatusDef{}
	statusOptionsJSON := template.HTMLAttr("[]")
	statusLabelByID := map[string]string{}
	if o, ok := db.FindOutline(it.OutlineID); ok && o != nil {
		statusDefs = o.StatusDefs
		statusOptionsJSON = outlineStatusOptionsJSON(*o)
		for _, sd := range o.StatusDefs {
			sid := strings.TrimSpace(sd.ID)
			if sid == "" {
				continue
			}
			lbl := strings.TrimSpace(sd.Label)
			if lbl == "" {
				lbl = sid
			}
			statusLabelByID[sid] = lbl
		}
	}

	assignedID := ""
	if it.AssignedActorID != nil {
		assignedID = strings.TrimSpace(*it.AssignedActorID)
	}
	assignedLabel := ""
	if assignedID != "" {
		assignedLabel = actorDisplayLabel(db, assignedID)
	}
	statusLabel := strings.TrimSpace(it.StatusID)
	if mapped, ok := statusLabelByID[statusLabel]; ok && strings.TrimSpace(mapped) != "" {
		statusLabel = strings.TrimSpace(mapped)
	}
	if statusLabel == "" {
		statusLabel = "(none)"
	}
	tagsInput := ""
	if len(it.Tags) > 0 {
		parts := make([]string, 0, len(it.Tags))
		for _, t := range it.Tags {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			parts = append(parts, "#"+t)
		}
		tagsInput = strings.Join(parts, " ")
	}
	dueDate := ""
	dueTime := ""
	if it.Due != nil {
		dueDate = strings.TrimSpace(it.Due.Date)
		if it.Due.Time != nil {
			dueTime = strings.TrimSpace(*it.Due.Time)
		}
	}
	schDate := ""
	schTime := ""
	if it.Schedule != nil {
		schDate = strings.TrimSpace(it.Schedule.Date)
		if it.Schedule.Time != nil {
			schTime = strings.TrimSpace(*it.Schedule.Time)
		}
	}

	vm := itemVM{
		baseVM:            s.baseVMForRequest(r, ""),
		Item:              *it,
		AssignedID:        assignedID,
		AssignedLabel:     assignedLabel,
		StatusLabel:       statusLabel,
		ActorOptions:      nil,
		Comments:          nil,
		ReplyTo:           "",
		Worklog:           nil,
		DescriptionHTML:   renderMarkdownHTML(it.Description),
		TagsInput:         tagsInput,
		DueDate:           dueDate,
		DueTime:           dueTime,
		SchDate:           schDate,
		SchTime:           schTime,
		StatusOptionsJSON: statusOptionsJSON,
		ActorOptionsJSON:  actorOptionsJSON(db),
		CanEdit:           canEdit,
		StatusDefs:        statusDefs,
	}
	vm.ActorID = actorID

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "item_preview.html", vm); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleItemEdit(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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
	assignedActorIDRaw := strings.TrimSpace(r.Form.Get("assignedActorId"))
	action := strings.TrimSpace(r.Form.Get("action"))

	priority := strings.TrimSpace(r.Form.Get("priority")) != ""
	onHold := strings.TrimSpace(r.Form.Get("onHold")) != ""
	dueDate := strings.TrimSpace(r.Form.Get("dueDate"))
	dueTime := strings.TrimSpace(r.Form.Get("dueTime"))
	schDate := strings.TrimSpace(r.Form.Get("schDate"))
	schTime := strings.TrimSpace(r.Form.Get("schTime"))
	tagsRaw := r.Form.Get("tags")

	st := store.Store{Dir: s.dir()}
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

	if action == "archive" {
		res, err := mutate.SetItemArchived(db, actorID, it.ID, true)
		if err != nil {
			http.Redirect(w, r, "/items/"+itemID+"?err="+urlQueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		if res.Changed {
			it.UpdatedAt = now
			if err := st.AppendEvent(actorID, "item.archive", it.ID, res.EventPayload); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			changed = true
		}
		if changed {
			if err := st.Save(db); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			if c := s.committer(); c != nil {
				actorLabel := strings.TrimSpace(actorID)
				if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
					actorLabel = strings.TrimSpace(a.Name)
				}
				c.Notify(actorLabel)
			}
		}
		http.Redirect(w, r, "/outlines/"+it.OutlineID+"?ok=archived", http.StatusSeeOther)
		return
	}

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

	// Assignment (optional).
	if assignedActorIDRaw != "" || it.AssignedActorID != nil {
		var target *string
		if assignedActorIDRaw != "" {
			tmp := assignedActorIDRaw
			target = &tmp
		}
		res, err := mutate.SetAssignedActor(db, actorID, it.ID, target, mutate.AssignOpts{TakeAssigned: false})
		if err != nil {
			http.Redirect(w, r, "/items/"+itemID+"?err="+urlQueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		if res.Changed {
			it.UpdatedAt = now
			if err := st.AppendEvent(actorID, "item.set_assign", it.ID, res.EventPayload); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			changed = true
		}
	}

	// Priority.
	if it.Priority != priority {
		it.Priority = priority
		it.UpdatedAt = now
		if err := st.AppendEvent(actorID, "item.set_priority", it.ID, map[string]any{"priority": it.Priority}); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		changed = true
	}

	// On-hold.
	if it.OnHold != onHold {
		it.OnHold = onHold
		it.UpdatedAt = now
		if err := st.AppendEvent(actorID, "item.set_on_hold", it.ID, map[string]any{"onHold": it.OnHold}); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		changed = true
	}

	// Due / Schedule.
	if dt, err := parseDateTimeWeb(dueDate, dueTime); err != nil {
		http.Redirect(w, r, "/items/"+itemID+"?err="+urlQueryEscape(err.Error()), http.StatusSeeOther)
		return
	} else if !sameDateTime(it.Due, dt) {
		it.Due = dt
		it.UpdatedAt = now
		if err := st.AppendEvent(actorID, "item.set_due", it.ID, map[string]any{"due": it.Due}); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		changed = true
	}
	if dt, err := parseDateTimeWeb(schDate, schTime); err != nil {
		http.Redirect(w, r, "/items/"+itemID+"?err="+urlQueryEscape(err.Error()), http.StatusSeeOther)
		return
	} else if !sameDateTime(it.Schedule, dt) {
		it.Schedule = dt
		it.UpdatedAt = now
		if err := st.AppendEvent(actorID, "item.set_schedule", it.ID, map[string]any{"schedule": it.Schedule}); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		changed = true
	}

	// Tags.
	nextTags := parseTagsInputWeb(tagsRaw)
	nextTags = uniqueSortedStringsWeb(normalizeTagsWeb(nextTags))
	curTags := uniqueSortedStringsWeb(normalizeTagsWeb(it.Tags))
	if !equalStrings(curTags, nextTags) {
		curSet := map[string]bool{}
		nextSet := map[string]bool{}
		for _, t := range curTags {
			curSet[t] = true
		}
		for _, t := range nextTags {
			nextSet[t] = true
		}
		for _, t := range curTags {
			if !nextSet[t] {
				it.Tags = removeString(it.Tags, t)
				it.Tags = uniqueSortedStringsWeb(normalizeTagsWeb(it.Tags))
				it.UpdatedAt = now
				if err := st.AppendEvent(actorID, "item.tags_remove", it.ID, map[string]any{"tag": t}); err != nil {
					http.Error(w, err.Error(), http.StatusConflict)
					return
				}
				changed = true
			}
		}
		for _, t := range nextTags {
			if !curSet[t] {
				it.Tags = append(it.Tags, t)
				it.Tags = uniqueSortedStringsWeb(normalizeTagsWeb(it.Tags))
				it.UpdatedAt = now
				if err := st.AppendEvent(actorID, "item.tags_add", it.ID, map[string]any{"tag": t}); err != nil {
					http.Error(w, err.Error(), http.StatusConflict)
					return
				}
				changed = true
			}
		}
	}

	if changed {
		if err := st.Save(db); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if c := s.committer(); c != nil {
			actorLabel := strings.TrimSpace(actorID)
			if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
				actorLabel = strings.TrimSpace(a.Name)
			}
			c.Notify(actorLabel)
		}
	}

	http.Redirect(w, r, "/items/"+itemID+"?ok=updated", http.StatusSeeOther)
}

type itemMetaResp struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Priority    bool            `json:"priority"`
	OnHold      bool            `json:"onHold"`
	Due         *model.DateTime `json:"due,omitempty"`
	Schedule    *model.DateTime `json:"schedule,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	CanEdit     bool            `json:"canEdit"`
}

func (s *Server) handleItemMeta(w http.ResponseWriter, r *http.Request) {
	itemID := strings.TrimSpace(r.PathValue("itemId"))
	if itemID == "" {
		http.Error(w, "missing item id", http.StatusBadRequest)
		return
	}
	actorID := strings.TrimSpace(s.actorForRequest(r))

	db, err := (store.Store{Dir: s.dir()}).Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	it, ok := db.FindItem(itemID)
	if !ok || it == nil {
		http.NotFound(w, r)
		return
	}
	canEdit := false
	if actorID != "" {
		canEdit = perm.CanEditItem(db, actorID, it)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(itemMetaResp{
		ID:          it.ID,
		Title:       it.Title,
		Description: it.Description,
		Priority:    it.Priority,
		OnHold:      it.OnHold,
		Due:         it.Due,
		Schedule:    it.Schedule,
		Tags:        it.Tags,
		CanEdit:     canEdit,
	})
}

func urlQueryEscape(s string) string {
	return url.QueryEscape(s)
}

func (s *Server) handleItemCommentAdd(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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

	st := store.Store{Dir: s.dir()}
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
	if c2 := s.committer(); c2 != nil {
		actorLabel := strings.TrimSpace(actorID)
		if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
			actorLabel = strings.TrimSpace(a.Name)
		}
		c2.Notify(actorLabel)
	}

	http.Redirect(w, r, "/items/"+itemID, http.StatusSeeOther)
}

func (s *Server) handleItemWorklogAdd(w http.ResponseWriter, r *http.Request) {
	if s.readOnly() {
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

	st := store.Store{Dir: s.dir()}
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
	if c2 := s.committer(); c2 != nil {
		actorLabel := strings.TrimSpace(actorID)
		if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
			actorLabel = strings.TrimSpace(a.Name)
		}
		c2.Notify(actorLabel)
	}

	http.Redirect(w, r, "/items/"+itemID, http.StatusSeeOther)
}

const sessionCookieName = "clarity_web_session"

func (s *Server) actorForRequest(r *http.Request) string {
	cfg := s.cfgSnapshot()
	// Fixed actor override is useful for local-only usage and early automation.
	if strings.TrimSpace(cfg.ActorID) != "" {
		return strings.TrimSpace(cfg.ActorID)
	}

	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	secret, err := loadOrInitSecretKey(cfg.Dir)
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

type agendaFlatRow struct {
	Item        model.Item
	Depth       int
	HasChildren bool
}

func agendaCompareItems(a, b model.Item) int {
	ra := strings.TrimSpace(a.Rank)
	rb := strings.TrimSpace(b.Rank)
	if ra != "" && rb != "" {
		if ra < rb {
			return -1
		}
		if ra > rb {
			return 1
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	}
	if a.CreatedAt.Before(b.CreatedAt) {
		return -1
	}
	if a.CreatedAt.After(b.CreatedAt) {
		return 1
	}
	if a.ID < b.ID {
		return -1
	}
	if a.ID > b.ID {
		return 1
	}
	return 0
}

func agendaFlattenOutline(o model.Outline, items []model.Item) []agendaFlatRow {
	children := map[string][]model.Item{}
	hasChildren := map[string]bool{}
	var roots []model.Item
	present := map[string]bool{}
	for _, it := range items {
		present[it.ID] = true
	}
	for _, it := range items {
		if it.ParentID == nil || strings.TrimSpace(*it.ParentID) == "" {
			roots = append(roots, it)
			continue
		}
		if !present[*it.ParentID] {
			roots = append(roots, it)
			continue
		}
		children[*it.ParentID] = append(children[*it.ParentID], it)
	}
	for pid, ch := range children {
		if len(ch) > 0 {
			hasChildren[pid] = true
		}
	}
	sort.Slice(roots, func(i, j int) bool { return agendaCompareItems(roots[i], roots[j]) < 0 })
	for pid := range children {
		sibs := children[pid]
		sort.Slice(sibs, func(i, j int) bool { return agendaCompareItems(sibs[i], sibs[j]) < 0 })
		children[pid] = sibs
	}

	out := make([]agendaFlatRow, 0, len(items))
	var walk func(it model.Item, depth int)
	walk = func(it model.Item, depth int) {
		out = append(out, agendaFlatRow{Item: it, Depth: depth, HasChildren: hasChildren[it.ID]})
		for _, ch := range children[it.ID] {
			walk(ch, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	return out
}

func agendaRowsWeb(db *store.DB, actorID string) []agendaRow {
	actorID = strings.TrimSpace(actorID)
	if db == nil || actorID == "" {
		return nil
	}

	// Sort projects by name for stable ordering (match TUI).
	projects := make([]model.Project, 0, len(db.Projects))
	for _, p := range db.Projects {
		if p.Archived {
			continue
		}
		projects = append(projects, p)
	}
	sort.Slice(projects, func(i, j int) bool {
		pi := strings.ToLower(strings.TrimSpace(projects[i].Name))
		pj := strings.ToLower(strings.TrimSpace(projects[j].Name))
		if pi == pj {
			return projects[i].ID < projects[j].ID
		}
		if pi == "" {
			return false
		}
		if pj == "" {
			return true
		}
		return pi < pj
	})

	outlinesByProject := map[string][]model.Outline{}
	for _, o := range db.Outlines {
		if o.Archived {
			continue
		}
		outlinesByProject[o.ProjectID] = append(outlinesByProject[o.ProjectID], o)
	}
	for pid := range outlinesByProject {
		outs := outlinesByProject[pid]
		sort.Slice(outs, func(i, j int) bool {
			ni := ""
			nj := ""
			if outs[i].Name != nil {
				ni = strings.ToLower(strings.TrimSpace(*outs[i].Name))
			}
			if outs[j].Name != nil {
				nj = strings.ToLower(strings.TrimSpace(*outs[j].Name))
			}
			if ni == nj {
				return outs[i].ID < outs[j].ID
			}
			if ni == "" {
				return false
			}
			if nj == "" {
				return true
			}
			return ni < nj
		})
		outlinesByProject[pid] = outs
	}

	rows := make([]agendaRow, 0, 64)
	for _, p := range projects {
		outs := outlinesByProject[p.ID]
		for _, o := range outs {
			// Filter items for this outline (match TUI: not archived, not on-hold, not end-state).
			its := make([]model.Item, 0, 64)
			for _, it := range db.Items {
				if it.Archived || it.OnHold {
					continue
				}
				if it.ProjectID != p.ID || it.OutlineID != o.ID {
					continue
				}
				if strings.TrimSpace(it.StatusID) == "" {
					continue
				}
				if isEndState(db, o.ID, it.StatusID) {
					continue
				}
				// Owned or assigned to current actor.
				if strings.TrimSpace(it.OwnerActorID) == actorID {
					its = append(its, it)
					continue
				}
				if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) == actorID {
					its = append(its, it)
					continue
				}
			}
			if len(its) == 0 {
				continue
			}

			projectName := strings.TrimSpace(p.Name)
			if projectName == "" {
				projectName = p.ID
			}
			outName := outlineDisplayNameWeb(o)

			statusLabelByID := map[string]string{}
			for _, sd := range o.StatusDefs {
				sid := strings.TrimSpace(sd.ID)
				if sid == "" {
					continue
				}
				lbl := strings.TrimSpace(sd.Label)
				if lbl == "" {
					lbl = sid
				}
				statusLabelByID[sid] = lbl
			}

			rows = append(rows, agendaRow{
				Kind:        "heading",
				ProjectID:   p.ID,
				ProjectName: projectName,
				OutlineID:   o.ID,
				OutlineName: outName,
			})

			flat := agendaFlattenOutline(o, its)
			for _, fr := range flat {
				it := fr.Item
				title := strings.TrimSpace(it.Title)
				if title == "" {
					title = "(untitled)"
				}
				sid := strings.TrimSpace(it.StatusID)
				lbl := sid
				if mapped, ok := statusLabelByID[sid]; ok && strings.TrimSpace(mapped) != "" {
					lbl = strings.TrimSpace(mapped)
				}
				if lbl == "" {
					lbl = "(none)"
				}
				assignedLabel := ""
				if it.AssignedActorID != nil && strings.TrimSpace(*it.AssignedActorID) != "" {
					assignedLabel = actorDisplayLabel(db, strings.TrimSpace(*it.AssignedActorID))
				}
				dueDate := ""
				dueTime := ""
				if it.Due != nil {
					dueDate = strings.TrimSpace(it.Due.Date)
					if it.Due.Time != nil {
						dueTime = strings.TrimSpace(*it.Due.Time)
					}
				}
				schDate := ""
				schTime := ""
				if it.Schedule != nil {
					schDate = strings.TrimSpace(it.Schedule.Date)
					if it.Schedule.Time != nil {
						schTime = strings.TrimSpace(*it.Schedule.Time)
					}
				}
				indent := 12 + (fr.Depth * 18)
				canEdit := actorID != "" && perm.CanEditItem(db, actorID, &it)
				rows = append(rows, agendaRow{
					Kind:          "item",
					ProjectID:     p.ID,
					ProjectName:   projectName,
					OutlineID:     o.ID,
					OutlineName:   outName,
					ItemID:        it.ID,
					Title:         title,
					StatusID:      it.StatusID,
					StatusLabel:   lbl,
					IsEndState:    false,
					CanEdit:       canEdit,
					AssignedLabel: assignedLabel,
					Priority:      it.Priority,
					OnHold:        it.OnHold,
					DueDate:       dueDate,
					DueTime:       dueTime,
					SchDate:       schDate,
					SchTime:       schTime,
					Tags:          uniqueSortedStringsWeb(normalizeTagsWeb(it.Tags)),
					Depth:         fr.Depth,
					IndentPx:      indent,
					HasChildren:   fr.HasChildren,
				})
			}
		}
	}
	return rows
}

func (s *Server) handleOutlineMeta(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("outlineId"))
	if id == "" {
		http.Error(w, "missing outline id", http.StatusBadRequest)
		return
	}
	db, err := (store.Store{Dir: s.dir()}).Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	o, ok := db.FindOutline(id)
	if !ok || o == nil || o.Archived {
		http.NotFound(w, r)
		return
	}

	name := "(unnamed outline)"
	if o.Name != nil && strings.TrimSpace(*o.Name) != "" {
		name = strings.TrimSpace(*o.Name)
	}

	type opt struct {
		ID           string `json:"id"`
		Label        string `json:"label"`
		IsEndState   bool   `json:"isEndState"`
		RequiresNote bool   `json:"requiresNote,omitempty"`
	}
	out := make([]opt, 0, len(o.StatusDefs)+1)
	if len(o.StatusDefs) > 0 {
		for _, sd := range o.StatusDefs {
			sid := strings.TrimSpace(sd.ID)
			if sid == "" {
				continue
			}
			lbl := strings.TrimSpace(sd.Label)
			if lbl == "" {
				lbl = sid
			}
			out = append(out, opt{ID: sid, Label: lbl, IsEndState: sd.IsEndState, RequiresNote: sd.RequiresNote})
		}
	} else {
		out = append(out, opt{ID: "todo", Label: "TODO", IsEndState: false})
		out = append(out, opt{ID: "doing", Label: "DOING", IsEndState: false})
		out = append(out, opt{ID: "done", Label: "DONE", IsEndState: true})
	}

	res := map[string]any{
		"id":            o.ID,
		"name":          name,
		"projectId":     o.ProjectID,
		"statusOptions": out,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(res)
}

func isEndState(db *store.DB, outlineID, statusID string) bool {
	o, ok := db.FindOutline(outlineID)
	if ok && o != nil {
		return statusutil.IsEndState(*o, statusID)
	}
	return statusutil.IsEndState(model.Outline{}, statusID)
}
