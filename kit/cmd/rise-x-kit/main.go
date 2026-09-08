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
	"syscall"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/buildinfo"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/server"
)

func main() {
	port := flag.Int("port", 0, "port to listen on (0 = pick any free port)")
	noBrowser := flag.Bool("no-browser", false, "don't open the browser automatically")
	claudeDir := flag.String("claude-dir", defaultClaudeDir(), "Claude Code config directory")
	showVersion := flag.Bool("version", false, "print the version and exit")
	printToken := flag.Bool("print-token", false, "print the API token on stdout (for scripting against the API by hand)")
	flag.Parse()

	if *showVersion {
		fmt.Println(buildinfo.String())
		return
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port

	token, err := randomToken()
	if err != nil {
		log.Fatalf("generate token: %v", err)
	}

	srv := server.New(server.Config{
		Port:      actualPort,
		Token:     token,
		ClaudeDir: *claudeDir,
		Version:   buildinfo.Version,
	})

	// Printing the token is opt-in: stdout lands in scrollback and launcher
	// logs, and the page gets it injected into index.html anyway.
	url := fmt.Sprintf("http://127.0.0.1:%d/", actualPort)
	fmt.Println(url)
	if *printToken {
		fmt.Println("token:", token)
	}

	if !*noBrowser {
		openBrowser(url)
	}

	httpServer := &http.Server{Handler: srv.Handler()}
	go func() {
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sig:
	case <-srv.Quit():
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
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
