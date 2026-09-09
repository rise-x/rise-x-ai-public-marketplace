// Command rise-x-kit is a single static binary that opens a local web page
// letting Rise-X partners manage the rise-x-public Claude Code plugins,
// check the Rise-X MCP connection, and run a setup doctor - without a
// terminal.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/buildinfo"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/claudemd"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/instance"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/server"
)

// shutdownGrace bounds both halves of the shutdown: waiting for the running
// job to notice its cancelled context, then draining HTTP connections.
const shutdownGrace = 5 * time.Second

// main is a thin wrapper so run's defers - releasing the single-instance lock
// above all - still run on every failure. log.Fatal skips them, which is what
// left a lock behind after a bad -port.
func main() { os.Exit(run()) }

func run() int {
	port := flag.Int("port", 0, "port to listen on (0 = pick any free port)")
	noBrowser := flag.Bool("no-browser", false, "don't open the browser automatically")
	claudeDir := flag.String("claude-dir", defaultClaudeDir(), "Claude Code config directory")
	showVersion := flag.Bool("version", false, "print the version and exit")
	printToken := flag.Bool("print-token", false, "print the API token on stdout (for scripting against the API by hand)")
	writeClaudeMD := flag.Bool("write-claude-md", false, "add the \"how to open Rise-X Kit\" block to ~/.claude/CLAUDE.md and exit")
	removeClaudeMD := flag.Bool("remove-claude-md", false, "take that block back out and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(buildinfo.String())
		return 0
	}

	if *writeClaudeMD || *removeClaudeMD {
		if err := applyClaudeMD(*claudeDir, *removeClaudeMD); err != nil {
			log.Printf("claude.md: %v", err)
			return 1
		}
		return 0
	}

	// A partner who closed the tab has no way back to a port that was picked
	// at random, so a second launch reopens the running window rather than
	// leaving another server behind. An explicit -port asks for a server on
	// that port, so it takes the lock instead of reopening.
	requested := given("port")
	found := instance.Running()

	var releaseLock func()
	locked := false
	if found == nil || requested {
		// One kit at a time: the jobs slot and the write slot live in this
		// process, so a second one would write ~/.claude with neither aware of
		// the other. The lock also covers the moment before a kit records
		// itself, when the probe still sees nothing.
		var lockErr error
		releaseLock, lockErr = instance.Lock()
		locked = releaseLock != nil
		if lockErr != nil {
			// Not contention: nothing to wait for, and refusing would leave
			// the partner with a kit that never starts on this machine. Say
			// what failed and serve anyway, single-instance unenforced.
			log.Printf("could not take the single-instance lock, so a second launch may start its own: %v", lockErr)
			releaseLock, locked = func() {}, true
		}
		if !locked && found == nil {
			// Whoever holds it may have finished starting since the probe.
			found = instance.Running()
		}
	}

	switch decide(requested, found != nil, locked) {
	case reopenWindow:
		reportReopen(found)
		fmt.Println(found.URL)
		if !*noBrowser {
			openBrowser(found.URL)
		}
		if *printToken {
			// Nothing was started here, so there is no token to print, and a
			// script must be able to tell that from a successful run.
			return 1
		}
		return 0
	case refuse:
		log.Print(refusal(requested))
		return 1
	}
	defer releaseLock()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Printf("listen: %v", err)
		return 1
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port

	token, err := randomToken()
	if err != nil {
		log.Printf("generate token: %v", err)
		return 1
	}

	srv := server.New(server.Config{
		Port:      actualPort,
		Token:     token,
		ClaudeDir: *claudeDir,
		Version:   buildinfo.Version,
	})

	// Recorded before the browser opens, so the next launch finds this window.
	if err := instance.Record(actualPort); err != nil {
		log.Printf("could not record this instance, so a second launch will start its own: %v", err)
	}
	defer instance.Clear()

	// Printing the token is opt-in: stdout lands in scrollback and launcher
	// logs, and the page gets it injected into index.html anyway.
	url := instance.URL(actualPort)
	fmt.Println(url)
	if *printToken {
		fmt.Println("token:", token)
	}

	if !*noBrowser {
		openBrowser(url)
	}

	httpServer := &http.Server{
		Handler: srv.Handler(),
		// A page in any browser can open a connection to this port and simply
		// not finish its request; requireToken only refuses it once the header
		// is read. Without these the connection is held forever and the
		// partner's own tab loses its share of the browser's per-origin
		// budget. WriteTimeout stays unset: starting a job legitimately takes
		// seconds, and a gather behind it longer.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			serveErr <- err
		}
	}()

	sigCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	code := 0
	select {
	case <-sigCtx.Done():
	case <-srv.Quit():
	case err := <-serveErr:
		log.Printf("serve: %v", err)
		code = 1
	}

	// Cancel the running job before we stop serving: http.Server.Shutdown
	// waits for connections, not for jobs, and the children run in their own
	// process group, so a Ctrl-C in the launching terminal never reaches them.
	srv.Jobs().CancelAll()
	if !srv.Jobs().WaitIdle(shutdownGrace) {
		log.Printf("a job was still running after %s; exiting anyway", shutdownGrace)
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	return code
}

// action is what a launch does once it knows what else is on the machine.
type action int

const (
	// startServer: this process holds the lock, so it serves.
	startServer action = iota
	// reopenWindow: a kit is already serving and this launch named no port,
	// so it hands back that window instead of starting a rival.
	reopenWindow
	// refuse: another kit holds the lock and its window is not what was
	// asked for.
	refuse
)

// decide picks between the three. It is a function of its own so the ladder
// can be tested: an explicit -port must never be answered with somebody
// else's window, which is what the lock-held path used to do.
func decide(portRequested, kitRunning, lockTaken bool) action {
	if kitRunning && !portRequested {
		return reopenWindow
	}
	if lockTaken {
		return startServer
	}
	return refuse
}

// refusal says why nothing happened, in the terms the partner used.
func refusal(portRequested bool) string {
	if portRequested {
		return "another Rise-X Kit holds the single-instance lock, so -port cannot be honoured; quit it from its page first."
	}
	return "another Rise-X Kit is starting; try again in a moment."
}

// launchOnlyFlags are the flags that only mean something for a launch that
// starts a server. A reopen silently ignores them, so it says so.
var launchOnlyFlags = []string{"port", "claude-dir", "print-token"}

// reportReopen says what the reopen actually did.
func reportReopen(found *instance.Found) {
	if found.Version != buildinfo.Version {
		fmt.Printf("reopening the Rise-X Kit already running (version %s; this binary is %s).\n",
			found.Version, buildinfo.Version)
		fmt.Println("quit it from its page to start this version instead.")
	}
	if ignored := givenOf(launchOnlyFlags); len(ignored) > 0 {
		fmt.Printf("ignored, because this reopened a kit this command did not start: %s\n",
			strings.Join(ignored, ", "))
	}
}

// given reports whether a flag was set on the command line, as opposed to
// left at its default.
func given(name string) bool { return len(givenOf([]string{name})) == 1 }

// givenOf returns which of names were set on the command line, as "-name".
func givenOf(names []string) []string {
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	var out []string
	flag.Visit(func(f *flag.Flag) {
		if want[f.Name] {
			out = append(out, "-"+f.Name)
		}
	})
	sort.Strings(out)
	return out
}

// applyClaudeMD is the installer's step: it teaches a Claude Code session how
// to open the app, and says what it changed.
func applyClaudeMD(claudeDir string, remove bool) error {
	apply := claudemd.Apply
	if remove {
		apply = claudemd.Remove
	}
	res, err := apply(claudeDir)
	if err != nil {
		return err
	}
	fmt.Printf("%s: %s\n", res.Path, res.Action)
	if res.Backup != "" {
		fmt.Println("previous file saved at", res.Backup)
	}
	return nil
}

func defaultClaudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// openBrowser opens url with the OS's default handler. Errors are logged,
// not fatal: the printed URL is still there for the partner to click.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("could not open browser automatically: %v", err)
	}
}
