package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/claudecli"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/doctor"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/instance"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/jobs"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/mcp"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/npmrc"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/selfupdate"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/settings"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/web"
)

// Per-action deadlines. A job that outlives its deadline is failed and the
// jobs store freed, so one wedged claude process can't 409 every later action.
const (
	pluginJobTimeout      = 10 * time.Minute // install/update/uninstall a plugin
	cliInstallJobTimeout  = 10 * time.Minute // curl | bash, over the partner's link
	marketplaceJobTimeout = 3 * time.Minute  // a shallow clone or a git fetch
	mcpFixJobTimeout      = 2 * time.Minute  // a remove plus an add per connection
	nodeInstallJobTimeout = 10 * time.Minute // nvm or winget, plus the download
	// mcpRestoreTimeout bounds putting a connection back after a failed add.
	// It runs on a context of its own, since the job's deadline expiring is
	// one of the reasons the add can have failed.
	mcpRestoreTimeout = 30 * time.Second
)

// maxActionBody caps an action's JSON body; every one of them is a couple of
// short fields.
const maxActionBody = 64 << 10

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
	// A second launch probes for this before deciding to reopen this window
	// instead of starting a rival server.
	h.Set(instance.HeaderName, s.version)
	// The page carries the CSRF token, so no cache may keep a copy of it.
	h.Set("Cache-Control", "no-store")
	_, _ = w.Write(web.Index(s.token))
}

// gatherFor answers one GET. ?fresh=1 drops every cache first, so the page's
// Refresh and Run again buttons re-probe the machine instead of being handed
// a three-second-old answer.
func (s *Server) gatherFor(r *http.Request) (OverviewResponse, doctor.Facts, error) {
	if r.URL.Query().Get("fresh") == "1" {
		s.invalidateProbes()
	}
	// Detached from the request: a browser reload mid-gather would otherwise
	// kill the claude child it is waiting on and cache that as a real answer.
	// Every command still carries its own timeout.
	return s.cachedGather(context.WithoutCancel(r.Context()))
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	overview, _, err := s.gatherFor(r)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, overview)
}

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	_, facts, err := s.gatherFor(r)
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
	Scope       string `json:"scope"`
	ProjectPath string `json:"projectPath"`
	// Enabled is a pointer so "not sent" and false are different things.
	Enabled *bool `json:"enabled"`
	Confirm bool  `json:"confirm"`
	// Targets names several connections at once, for mcp.remove: exactly the
	// rows the page showed, each re-matched against the server's own scan.
	Targets []connectionTarget `json:"targets"`
}

type connectionTarget struct {
	Name        string `json:"name"`
	Scope       string `json:"scope"`
	ProjectPath string `json:"projectPath"`
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

	// Once the update has swapped the binary in, this process is about to stop
	// serving, and a job started now would be orphaned by the relaunch.
	if s.restartPending() {
		httpError(w, http.StatusConflict, "Rise-X Kit is restarting into the new version")
		return
	}

	// Detached from the request for the same reason as gatherFor: an action's
	// pre-flight probes write to the same caches.
	ctx := context.WithoutCancel(r.Context())
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
	case "node.install":
		// Like cli.install, this one puts a missing tool on the machine, so
		// it must run without a claude CLI.
		s.handleNodeInstall(w, ctx, name)
	case "mcp.fix":
		s.handleMcpFix(w, ctx, name, body)
	case "mcp.remove":
		s.handleMcpRemove(w, ctx, name, body)
	case "kit.update":
		s.handleKitUpdate(w, name)
	case "cli.rescan":
		cli, _ := s.relocate(ctx)
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
	case !s.ownedByPublic(ctx, pluginName):
		// All three verbs run against @rise-x-public, so a copy Claude
		// Desktop, the organisation or another marketplace owns has to be
		// refused for every one of them: acting on it from here leaves the
		// machine with two copies. The page hides these buttons, but one
		// rendered before the synced probe caught up still offers them.
		httpError(w, http.StatusBadRequest, refuseForeignCopy(name, pluginName))
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
	headerBound := false
	for _, st := range targets {
		switch {
		case st.Fixable():
			fixable = append(fixable, st)
		case st.HasHeaders:
			headerBound = true
		}
	}
	if len(fixable) == 0 {
		if headerBound {
			httpError(w, http.StatusBadRequest,
				"this connection carries its own headers, which the fix cannot put back; repoint it yourself so its credentials are kept")
			return
		}
		httpError(w, http.StatusBadRequest,
			"nothing the CLI can change; update this one in Claude Desktop under Customize, Connectors")
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
			// The fix is a remove and an add, with no transaction around it.
			// Print what is about to be taken away first, so a failure
			// between the two leaves the partner the line they need to put it
			// back by hand rather than just a failed job.
			onLine(fmt.Sprintf("%s (%s): %s %s -> %s", st.Name, st.Scope, st.Transport, st.URL, st.SuggestedURL))
			// A remove that exits non-zero has nothing to remove, which is no
			// reason to skip the add.
			if code, err := client.McpRemove(jctx, dir, st.Name, st.Scope, onLine); err != nil {
				return code, err
			}
			code, err := client.McpAdd(jctx, dir, st.Name, st.SuggestedURL, st.Scope, st.Transport, onLine)
			if err != nil || code != 0 {
				// The remove landed and the add did not, so the connection is
				// gone. Put the original back before reporting, on a context
				// of its own: the job's may be the reason the add failed.
				onLine(fmt.Sprintf("restoring %s at its previous address", st.Name))
				rctx, cancel := context.WithTimeout(context.WithoutCancel(jctx), mcpRestoreTimeout)
				if rcode, rerr := client.McpAdd(rctx, dir, st.Name, st.URL, st.Scope, st.Transport, onLine); rerr != nil || rcode != 0 {
					onLine(fmt.Sprintf("could not restore %s: add it again with: claude mcp add --transport %s %s %s -s %s",
						st.Name, st.Transport, st.Name, st.URL, st.Scope))
				}
				cancel()
				return code, err
			}
		}
		return 0, nil
	})
}

// kitUpdateJobTimeout bounds the download of a release asset of a few MB.
const kitUpdateJobTimeout = 5 * time.Minute

// restartDelay is how long the finished job stays readable before this
// process stops serving, so the page's poll can see it succeed and switch to
// waiting for the new one.
const restartDelay = 1500 * time.Millisecond

// handleKitUpdate downloads the newest release for this platform, verifies it
// against the release's checksums, swaps it in for the running binary and asks
// main to relaunch. It goes through the jobs store directly: it needs no
// claude CLI, and finishing must not raise the "restart Claude Code" hint.
func (s *Server) handleKitUpdate(w http.ResponseWriter, action string) {
	if s.exePath == "" {
		httpError(w, http.StatusBadRequest, "cannot tell which file is running, so it cannot be replaced")
		return
	}
	// A failure the background check remembered was bounded by that check's
	// five seconds; the update has five minutes and asks again.
	s.updates.ForgetFailures()
	id, err := s.jobs.Start(action, kitUpdateJobTimeout, func(jctx context.Context, onLine func(string)) (int, error) {
		rel, newer, err := s.updates.Latest(jctx, s.version)
		if err != nil {
			return -1, fmt.Errorf("check for a newer version: %w", err)
		}
		if !newer {
			return -1, fmt.Errorf("%s is already the newest version", s.version)
		}
		onLine(fmt.Sprintf("updating Rise-X Kit %s -> %s", s.version, rel.Version))
		newPath, err := s.updates.Download(jctx, rel, runtime.GOOS, runtime.GOARCH, filepath.Dir(s.exePath), onLine)
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				return -1, fmt.Errorf("Rise-X Kit cannot replace itself in %s: %w. Download %s from %s and run the installer again",
					filepath.Dir(s.exePath), err, rel.Version, rel.URL)
			}
			return -1, err
		}
		if err := selfupdate.Apply(newPath, s.exePath); err != nil {
			os.RemoveAll(filepath.Dir(newPath))
			if errors.Is(err, fs.ErrPermission) {
				return -1, fmt.Errorf("Rise-X Kit cannot replace itself at %s: %w. Download %s from %s and run the installer again",
					s.exePath, err, rel.Version, rel.URL)
			}
			return -1, fmt.Errorf("replace %s: %w", s.exePath, err)
		}
		os.RemoveAll(filepath.Dir(newPath))
		onLine("restarting Rise-X Kit")
		time.AfterFunc(restartDelay, s.requestRestart)
		return 0, nil
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

// handleMcpRemove takes away the Rise-X connections Claude Code holds outside
// the plugin: the one the body names, or the ones it lists in targets. There
// is deliberately no "everything" form: what goes is exactly what the page
// showed. Like mcp.fix, each target is re-matched against the scan, so
// nothing from the request shapes the claude argv.
func (s *Server) handleMcpRemove(w http.ResponseWriter, ctx context.Context, action string, body actionBody) {
	known := s.connections()
	asked := body.Targets
	if body.Name != "" {
		asked = append(asked, connectionTarget{Name: body.Name, Scope: body.Scope, ProjectPath: body.ProjectPath})
	}
	if len(asked) == 0 {
		httpError(w, http.StatusBadRequest, "name the connection to remove")
		return
	}

	var removable []mcp.Connection
	for _, t := range asked {
		match := findConnection(known, t.Name, t.Scope, t.ProjectPath)
		switch {
		case match == nil:
			httpError(w, http.StatusBadRequest, "unknown connection target")
			return
		case match.Scope == mcp.ScopeDesktop:
			httpError(w, http.StatusBadRequest,
				"this connection is in Claude Desktop's own configuration file, which the CLI does not edit; remove it there")
			return
		case match.HasHeaders:
			httpError(w, http.StatusBadRequest,
				"this connection carries its own headers, which may hold a credential that exists nowhere else; remove it yourself if you mean to")
			return
		}
		removable = append(removable, *match)
	}

	s.startJob(w, ctx, action, needsCLI, mcpFixJobTimeout, func(jctx context.Context, onLine func(string)) (int, error) {
		client, ok := s.client(jctx)
		if !ok {
			return -1, claudecli.ErrNotFound
		}
		for _, c := range removable {
			dir := ""
			if c.Scope == mcp.ScopeLocal {
				dir = c.ProjectPath
				if _, err := os.Stat(dir); err != nil {
					onLine(fmt.Sprintf("skipping %s: %s no longer exists", c.Name, dir))
					continue
				}
			}
			onLine(fmt.Sprintf("removing %s (%s): %s", c.Name, c.Scope, c.URL))
			if code, err := client.McpRemove(jctx, dir, c.Name, c.Scope, onLine); err != nil || code != 0 {
				return code, err
			}
		}
		return 0, nil
	})
}

func findConnection(list []mcp.Connection, name, scope, projectPath string) *mcp.Connection {
	for i := range list {
		c := &list[i]
		if c.Name == name && c.Scope == scope &&
			(c.Scope != mcp.ScopeLocal || c.ProjectPath == projectPath) {
			return c
		}
	}
	return nil
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
	list, err := s.marketplaceList(ctx)
	if err != nil {
		return nil, false
	}
	mp := findMarketplace(list, name)
	return mp, mp != nil
}

// ownedByPublic reports whether name's copy on this machine is the one
// `claude plugin <verb> <name>@rise-x-public` would change: a CLI install
// record from the public marketplace, or no install record at all (a first
// install, and the reinstall the page offers for a copy with no version
// stamp). A copy Claude Desktop synced from the account is not the CLI's to
// touch. The predicate mirrors how gather derives PluginFact.InstallSource,
// so the guard and the badge can never disagree.
func (s *Server) ownedByPublic(ctx context.Context, name string) bool {
	// The Desktop and organisation copies, which are the ones this guard
	// exists for, are answered by the synced manifest: a file read, already
	// cached, no claude spawn.
	if _, synced := pickSynced(s.synced(), name); synced {
		return false
	}
	// A copy installed from another marketplace needs the CLI's own list, and
	// an action handler must not wait on a spawn to answer a click. Only a
	// list the page load already paid for is consulted; with none, the action
	// goes ahead, which is also the only state in which no rendered page
	// exists to have offered the button.
	res, ok := s.plListCache.peek()
	if !ok || res.err != nil {
		return true
	}
	if installed, market := findInstalled(res.res.Installed, name); installed != nil {
		return market == claudecli.MarketplaceName
	}
	return true
}

// refuseForeignCopy says why the action was refused, in the terms of the verb
// the partner pressed.
func refuseForeignCopy(action, plugin string) string {
	switch action {
	case "plugin.install":
		return plugin + " is already installed from somewhere else; installing it from the public marketplace would leave two copies"
	case "plugin.uninstall":
		return "this copy of " + plugin + " did not come from the public marketplace; remove it where it was installed from"
	default:
		return "this copy of " + plugin + " did not come from the public marketplace; update it where it was installed from"
	}
}

func (s *Server) marketplaceRegistered(ctx context.Context) bool {
	marketplaces, err := s.marketplaceList(ctx)
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
	// res.Rewritten is masked by the npmrc package: the response says which
	// line changed, never a credential (registry lines never carry one).
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
	writeJSON(w, map[string]any{"updated": res.Rewritten, "backup": res.Backup})
}

// withWriteSlot runs fn while holding the jobs store's one running-job slot.
// autoupdate.set and npmrc.clean edit files a claude CLI job rewrites too -
// `claude plugin marketplace add` rewrites extraKnownMarketplaces - so they
// take the same slot every job takes instead of racing one. fn stays
// synchronous; the slot is only there to keep the two writers apart.
func (s *Server) withWriteSlot(action string, fn func() error) error {
	release, err := s.jobs.Hold(action)
	if err != nil {
		return err
	}
	// Deferred, so a panic in fn frees the slot rather than wedging every
	// later action behind a 409, and records the job as failed rather than as
	// a write that succeeded. Start's own fn gets the same treatment.
	var werr error
	defer func() {
		if r := recover(); r != nil {
			release(fmt.Errorf("%s panicked: %v", action, r))
			panic(r)
		}
		release(werr)
	}()
	werr = fn()
	return werr
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
