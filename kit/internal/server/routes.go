package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/claudecli"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/doctor"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/jobs"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/mcp"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/npmrc"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/settings"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/web"
)

// Per-action deadlines. A job that outlives its deadline is failed and the
// jobs store freed, so one wedged claude process can't 409 every later action.
const (
	pluginJobTimeout      = 10 * time.Minute // install/update/uninstall a plugin
	cliInstallJobTimeout  = 10 * time.Minute // curl | bash, over the partner's link
	marketplaceJobTimeout = 3 * time.Minute  // a shallow clone or a git fetch
	mcpLoginJobTimeout    = 2 * time.Minute  // an interactive OAuth round trip
	mcpFixJobTimeout      = 2 * time.Minute  // a remove plus an add per connection
)

// maxActionBody caps an action's JSON body; every one of them is a couple of
// short fields.
const maxActionBody = 64 << 10

// writeSlotTimeout bounds how long a synchronous file edit may hold the jobs
// store's single-writer slot. Both edits are a read, a render and a rename.
const writeSlotTimeout = 30 * time.Second

// contentSecurityPolicy is sent with every response. The page carries the CSRF
// token, so it must never be framed; font-src allows data: only for the design
// system's web font.
const contentSecurityPolicy = "default-src 'self'; frame-ancestors 'none'; font-src 'self' data:; " +
	"base-uri 'none'; form-action 'none'; object-src 'none'"

// Handler builds the full HTTP handler: routes plus the Host and CSRF
// middleware, wrapping the embedded browser page.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/overview", s.handleOverview)
	mux.HandleFunc("GET /api/doctor", s.handleDoctor)
	mux.HandleFunc("POST /api/actions/{name}", s.handleAction)
	mux.HandleFunc("GET /api/jobs/{id}", s.handleJob)
	assets := web.Assets()
	for _, name := range web.AssetNames {
		mux.HandleFunc("GET /"+name, assets.ServeHTTP)
	}
	mux.HandleFunc("GET /", s.handleIndex)

	var h http.Handler = mux
	h = s.requireToken(h)
	h = s.requireHost(h)
	return securityHeaders(h)
}

// securityHeaders sets the headers every response needs, the middlewares' own
// error responses included.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// requireToken guards /api/* with the per-process CSRF token; the browser
// page and its assets load via plain navigation and carry no header.
func (s *Server) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			got := r.Header.Get("X-RiseX-Token")
			if s.token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				httpError(w, http.StatusForbidden, "forbidden")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requireHost blocks DNS-rebinding: only this exact loopback origin may
// talk to the kit, whatever hostname a page in the browser claims to be.
func (s *Server) requireHost(next http.Handler) http.Handler {
	allowed := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", s.port): true,
		fmt.Sprintf("localhost:%d", s.port): true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed[r.Host] {
			httpError(w, http.StatusBadRequest, "bad host")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	// The page carries the CSRF token, so no cache may keep a copy of it.
	h.Set("Cache-Control", "no-store")
	_, _ = w.Write(web.Index(s.token))
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	overview, _, err := s.cachedGather(r.Context())
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, overview)
}

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	_, facts, err := s.cachedGather(r.Context())
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, DoctorResponse{Checks: doctor.Run(facts)})
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	since := 0
	if v := r.URL.Query().Get("since"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			httpError(w, http.StatusBadRequest, "since must be an integer")
			return
		}
		since = n
	}
	snap, ok := s.jobs.Snapshot(id, since)
	if !ok {
		httpError(w, http.StatusNotFound, "unknown job")
		return
	}
	writeJSON(w, snap)
}

// actionBody is every field the actions read. Decoding into one typed struct
// with unknown fields rejected means a misspelled or wrongly typed field is a
// 400 the page can show, not a value silently read as its zero.
type actionBody struct {
	Name        string `json:"name"`
	Marketplace string `json:"marketplace"`
	Server      string `json:"server"`
	Scope       string `json:"scope"`
	ProjectPath string `json:"projectPath"`
	// Enabled is a pointer so "not sent" and false are different things.
	Enabled *bool `json:"enabled"`
	Confirm bool  `json:"confirm"`
}

// decodeActionBody reads the request body. An empty body is valid - most
// actions take none - but anything present must parse into actionBody.
func decodeActionBody(w http.ResponseWriter, r *http.Request) (actionBody, error) {
	var body actionBody
	if r.Body == nil {
		return body, nil
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxActionBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		return actionBody{}, err
	}
	return body, nil
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	body, err := decodeActionBody(w, r)
	if err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	switch name {
	case "marketplace.add":
		s.startJob(w, ctx, name, needsCLI, marketplaceJobTimeout, func(jctx context.Context, onLine func(string)) (int, error) {
			client, ok := s.client(jctx)
			if !ok {
				return -1, claudecli.ErrNotFound
			}
			return client.MarketplaceAdd(jctx, onLine)
		})
	case "marketplace.update":
		s.startJob(w, ctx, name, needsCLI, marketplaceJobTimeout, func(jctx context.Context, onLine func(string)) (int, error) {
			client, ok := s.client(jctx)
			if !ok {
				return -1, claudecli.ErrNotFound
			}
			return client.MarketplaceUpdate(jctx, onLine)
		})
	case "plugin.install", "plugin.update", "plugin.uninstall":
		s.handlePluginAction(w, ctx, name, body)
	case "autoupdate.set":
		s.handleAutoUpdateSet(w, ctx, body)
	case "npmrc.clean":
		s.handleNpmrcClean(w, body)
	case "reload-hint.dismiss":
		// The page dismissed the "restart Claude Code" banner; forget it
		// server-side so the next refresh doesn't bring it back.
		s.clearReloadHint()
		s.invalidateGather()
		writeJSON(w, map[string]any{"ok": true})
	case "cli.install":
		// The one action that must work with no CLI on the machine: it is
		// what puts one there.
		s.startJob(w, ctx, name, noCLI, cliInstallJobTimeout, func(jctx context.Context, onLine func(string)) (int, error) {
			code, err := runCLIInstall(jctx, s.runner, onLine)
			if err == nil && code == 0 {
				_, _ = s.relocate(jctx)
			}
			return code, err
		})
	case "mcp.login":
		s.handleMcpLogin(w, ctx, name, body)
	case "mcp.fix":
		s.handleMcpFix(w, ctx, name, body)
	case "cli.rescan":
		cli, _ := s.relocate(r.Context())
		s.nodeCache.invalidate()
		s.npmrcCache.invalidate()
		s.invalidateClaudeProbes()
		writeJSON(w, map[string]any{"found": cli != nil})
	case "quit":
		writeJSON(w, map[string]any{"ok": true})
		s.quitOnce.Do(func() { close(s.quitRequested) })
	default:
		httpError(w, http.StatusBadRequest, "unknown action: "+name)
	}
}

func (s *Server) handlePluginAction(w http.ResponseWriter, ctx context.Context, name string, body actionBody) {
	pluginName := body.Name
	if !s.isKnownPlugin(ctx, pluginName) {
		httpError(w, http.StatusBadRequest, "unknown plugin target")
		return
	}
	// A skill installed from a mirror of the public marketplace must be
	// updated from that mirror; the public one would install a second copy.
	marketplace := claudecli.MarketplaceName
	switch from := body.Marketplace; {
	case from != "" && from != claudecli.MarketplaceName:
		if name != "plugin.update" {
			httpError(w, http.StatusBadRequest, "marketplace is only valid for plugin.update")
			return
		}
		if _, ok := s.knownMarketplace(ctx, from); !ok {
			httpError(w, http.StatusBadRequest, "unknown marketplace: "+from)
			return
		}
		marketplace = from
	case name == "plugin.update" && !s.updatableFromPublic(ctx, pluginName):
		// A copy Claude Desktop or another marketplace owns must not be
		// updated from the public marketplace: that installs a second copy.
		httpError(w, http.StatusBadRequest,
			"this copy of "+pluginName+" did not come from the public marketplace; update it where it was installed from")
		return
	}
	s.startJob(w, ctx, name, needsCLI, pluginJobTimeout, func(jctx context.Context, onLine func(string)) (int, error) {
		client, ok := s.client(jctx)
		if !ok {
			return -1, claudecli.ErrNotFound
		}
		if name == "plugin.install" && !s.marketplaceRegistered(jctx) {
			if code, err := client.MarketplaceAdd(jctx, onLine); err != nil || code != 0 {
				return code, err
			}
		}
		switch name {
		case "plugin.install":
			return client.PluginInstall(jctx, pluginName, onLine)
		case "plugin.update":
			return client.PluginUpdate(jctx, pluginName, marketplace, onLine)
		default:
			return client.PluginUninstall(jctx, pluginName, onLine)
		}
	})
}

// handleMcpLogin validates the target against the server names the rise-x-mcp
// plugin actually declares in its .mcp.json, so nothing a page sends can shape
// the claude argv.
func (s *Server) handleMcpLogin(w http.ResponseWriter, ctx context.Context, name string, body actionBody) {
	target := body.Server
	if target == "" {
		httpError(w, http.StatusBadRequest, "server is required")
		return
	}
	if _, ok := s.client(ctx); !ok {
		httpError(w, http.StatusBadRequest, "claude CLI not found")
		return
	}
	configured := s.configuredMcpServers(ctx)
	if !slices.ContainsFunc(configured, func(cs mcp.ConfiguredServer) bool { return cs.Name == target }) {
		valid := make([]string, len(configured))
		for i, cs := range configured {
			valid[i] = cs.Name
		}
		httpError(w, http.StatusBadRequest,
			"unknown MCP server target; expected one of: "+strings.Join(valid, ", "))
		return
	}
	s.startJob(w, ctx, name, needsCLI, mcpLoginJobTimeout, func(jctx context.Context, onLine func(string)) (int, error) {
		client, ok := s.client(jctx)
		if !ok {
			return -1, claudecli.ErrNotFound
		}
		return client.McpLogin(jctx, target, onLine)
	})
}

// handleMcpFix repoints stale Rise-X connections at the address Rise-X serves
// today: the one the body names, or every fixable one when it names none. The
// targets come from the scan rather than the request, so nothing a page sends
// can shape the claude argv.
func (s *Server) handleMcpFix(w http.ResponseWriter, ctx context.Context, action string, body actionBody) {
	targets := s.staleMcp()
	if name := body.Name; name != "" {
		match := findStale(targets, name, body.Scope, body.ProjectPath)
		if match == nil {
			httpError(w, http.StatusBadRequest, "unknown stale connection target")
			return
		}
		targets = []mcp.Stale{*match}
	}

	var fixable []mcp.Stale
	for _, st := range targets {
		if st.Scope != mcp.ScopeDesktop {
			fixable = append(fixable, st)
		}
	}
	if len(fixable) == 0 {
		httpError(w, http.StatusBadRequest,
			"nothing the CLI can change; update this one in Claude Desktop under Settings, Connectors")
		return
	}

	s.startJob(w, ctx, action, needsCLI, mcpFixJobTimeout, func(jctx context.Context, onLine func(string)) (int, error) {
		client, ok := s.client(jctx)
		if !ok {
			return -1, claudecli.ErrNotFound
		}
		for _, st := range fixable {
			dir := ""
			if st.Scope == mcp.ScopeLocal {
				dir = st.ProjectPath // -s local writes to whichever project it runs in
				// ~/.claude.json keeps an entry for every project ever opened,
				// so a directory that is gone must not abort the rest.
				if _, err := os.Stat(dir); err != nil {
					onLine(fmt.Sprintf("skipping %s: %s no longer exists", st.Name, dir))
					continue
				}
			}
			// A remove that exits non-zero has nothing to remove, which is no
			// reason to skip the add.
			if code, err := client.McpRemove(jctx, dir, st.Name, st.Scope, onLine); err != nil {
				return code, err
			}
			if code, err := client.McpAdd(jctx, dir, st.Name, st.SuggestedURL, st.Scope, onLine); err != nil || code != 0 {
				return code, err
			}
		}
		return 0, nil
	})
}

func findStale(list []mcp.Stale, name, scope, projectPath string) *mcp.Stale {
	for i := range list {
		st := &list[i]
		if st.Name == name && st.Scope == scope &&
			(st.Scope != mcp.ScopeLocal || st.ProjectPath == projectPath) {
			return st
		}
	}
	return nil
}

// knownMarketplace looks a marketplace name up in `claude plugin marketplace
// list --json`, so an action can only name one Claude Code already knows.
func (s *Server) knownMarketplace(ctx context.Context, name string) (*claudecli.Marketplace, bool) {
	client, ok := s.client(ctx)
	if !ok {
		return nil, false
	}
	list, err := client.MarketplaceList(ctx)
	if err != nil {
		return nil, false
	}
	mp := findMarketplace(list, name)
	return mp, mp != nil
}

// updatableFromPublic reports whether name's copy on this machine is the one
// `claude plugin update <name>@rise-x-public` would change: a CLI install
// record from the public marketplace, or no install record at all (the
// reinstall the page offers for a copy with no version stamp). A copy Claude
// Desktop synced from the account is not the CLI's to update.
func (s *Server) updatableFromPublic(ctx context.Context, name string) bool {
	client, ok := s.client(ctx)
	if !ok {
		return false
	}
	res, err := client.PluginListAvailable(ctx)
	if err != nil {
		return true // unreadable, not "wrong source": leave the update alone
	}
	if installed, market := findInstalled(res.Installed, name); installed != nil {
		return market == claudecli.MarketplaceName
	}
	_, synced := pickSynced(s.synced(), name)
	return !synced
}

// configuredMcpServers reads the rise-x-mcp plugin's bundled .mcp.json, the
// same source /api/overview reports as mcp.configured.
func (s *Server) configuredMcpServers(ctx context.Context) []mcp.ConfiguredServer {
	client, ok := s.client(ctx)
	if !ok {
		return nil
	}
	res, err := client.PluginListAvailable(ctx)
	if err != nil {
		return nil
	}
	return riseXMcpConfig(res.Installed)
}

func (s *Server) marketplaceRegistered(ctx context.Context) bool {
	client, ok := s.client(ctx)
	if !ok {
		return false
	}
	marketplaces, err := client.MarketplaceList(ctx)
	if err != nil {
		return false
	}
	return findMarketplace(marketplaces, claudecli.MarketplaceName) != nil
}

func (s *Server) handleAutoUpdateSet(w http.ResponseWriter, ctx context.Context, body actionBody) {
	if body.Enabled == nil {
		httpError(w, http.StatusBadRequest, "enabled must be true or false")
		return
	}
	// settings.Read fails only when the file itself isn't valid JSON - the
	// same case the doctor row reports as SettingsError - and the write below
	// would refuse it anyway, just with a less friendly message. It is not a
	// 409: nothing is running, the file needs fixing by hand.
	if _, err := settings.Read(s.settingsPath()); err != nil {
		httpError(w, http.StatusUnprocessableEntity, doctor.SettingsUnreadableMessage)
		return
	}
	name, repo := claudecli.MarketplaceName, claudecli.MarketplaceRepo
	// Skills can come from a mirror of the public marketplace, and it is that
	// mirror's autoUpdate flag that then governs them.
	if from := body.Marketplace; from != "" && from != claudecli.MarketplaceName {
		mp, ok := s.knownMarketplace(ctx, from)
		if !ok {
			httpError(w, http.StatusBadRequest, "unknown marketplace: "+from)
			return
		}
		name, repo = mp.Name, mp.Repo
	}
	var backup string
	err := s.withWriteSlot("autoupdate.set", func() (err error) {
		backup, err = s.writer.SetAutoUpdate(name, repo, *body.Enabled)
		return err
	})
	if err != nil {
		writeSlotError(w, err)
		return
	}
	s.invalidateGather()
	writeJSON(w, map[string]any{"backup": backup})
}

func (s *Server) handleNpmrcClean(w http.ResponseWriter, body actionBody) {
	if !body.Confirm {
		httpError(w, http.StatusBadRequest, "confirm:true is required")
		return
	}
	// res.Removed is masked by the npmrc package: the response says which
	// host and key went, never the credential itself.
	var res npmrc.Result
	err := s.withWriteSlot("npmrc.clean", func() (err error) {
		res, err = npmrc.Clean(s.npmrcPath)
		return err
	})
	if err != nil {
		writeSlotError(w, err)
		return
	}
	s.npmrcCache.invalidate()
	s.invalidateGather()
	writeJSON(w, map[string]any{"removed": res.Removed, "backup": res.Backup})
}

// withWriteSlot runs fn while holding the jobs store's one running-job slot.
// autoupdate.set and npmrc.clean edit files a claude CLI job rewrites too -
// `claude plugin marketplace add` rewrites extraKnownMarketplaces - so they
// take the same slot every job takes instead of racing one. fn stays
// synchronous; the slot is only there to keep the two writers apart.
func (s *Server) withWriteSlot(action string, fn func() error) error {
	release := make(chan struct{})
	_, err := s.jobs.Start(action, writeSlotTimeout, func(jctx context.Context, _ func(string)) (int, error) {
		select {
		case <-release:
			return 0, nil
		case <-jctx.Done():
			return -1, jctx.Err()
		}
	})
	if err != nil {
		return err
	}
	defer close(release)
	return fn()
}

// writeSlotError answers a refused write: 409 while a job holds the slot,
// 500 for a write that actually failed.
func writeSlotError(w http.ResponseWriter, err error) {
	if errors.Is(err, jobs.ErrBusy) {
		httpError(w, http.StatusConflict, "a job is already running")
		return
	}
	httpError(w, http.StatusInternalServerError, err.Error())
}

// needsCLI/noCLI name startJob's gate at the call site.
const (
	needsCLI = true
	noCLI    = false
)

// startJob starts a job and writes {jobId} or a 409/500 error. On success it
// also sets the "restart Claude Code" reload hint, per the plan.
func (s *Server) startJob(w http.ResponseWriter, ctx context.Context, action string, requireCLI bool, timeout time.Duration, fn jobs.Func) {
	if requireCLI {
		if _, ok := s.client(ctx); !ok {
			httpError(w, http.StatusBadRequest, "claude CLI not found")
			return
		}
	}
	id, err := s.jobs.Start(action, timeout, func(jctx context.Context, onLine func(string)) (int, error) {
		code, err := fn(jctx, onLine)
		if err == nil && code == 0 {
			s.setReloadHint()
		}
		s.invalidateClaudeProbes()
		return code, err
	})
	if err != nil {
		if errors.Is(err, jobs.ErrBusy) {
			httpError(w, http.StatusConflict, "a job is already running")
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"jobId": id})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
