package main

import (
	"flag"
	"net"
	"testing"
	"time"
)

// The reopen/lock ladder, as a table. The -port rows are the ones that went
// wrong: a launch that named a port was handed a window on some other port,
// silently, whenever the lock was held.
func TestDecide(t *testing.T) {
	cases := []struct {
		name                              string
		portRequested, running, lockTaken bool
		want                              action
	}{
		{"no record, lock free", false, false, true, startServer},
		{"a kit is running", false, true, false, reopenWindow},
		{"a kit is running, -port given", true, true, true, startServer},
		{"lock held, -port given", true, true, false, refuse},
		{"lock held, nothing serving yet", false, false, false, refuse},
		{"lock held, kit came up during the probe", false, true, false, reopenWindow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decide(c.portRequested, c.running, c.lockTaken); got != c.want {
				t.Errorf("decide(%v, %v, %v) = %v, want %v",
					c.portRequested, c.running, c.lockTaken, got, c.want)
			}
		})
	}
}

// The refusal has to name the port, or a partner who typed one is told to
// wait for a launch that is not coming.
func TestRefusal_NamesThePortFlag(t *testing.T) {
	if got := refusal(true); got == refusal(false) {
		t.Fatal("refusal says the same thing with and without -port")
	}
}

// A reopen ignores every flag that only means something for a launch that
// starts a server, so it has to say which ones it dropped.
func TestGivenOf_ReportsOnlyTheFlagsActuallyGiven(t *testing.T) {
	set := flag.NewFlagSet("kit", flag.ContinueOnError)
	old := flag.CommandLine
	flag.CommandLine = set
	defer func() { flag.CommandLine = old }()

	set.Int("port", 0, "")
	set.String("claude-dir", "/default", "")
	set.Bool("print-token", false, "")
	if err := set.Parse([]string{"-claude-dir", "/elsewhere"}); err != nil {
		t.Fatal(err)
	}

	got := givenOf(launchOnlyFlags)
	if len(got) != 1 || got[0] != "-claude-dir" {
		t.Fatalf("givenOf = %v, want just -claude-dir", got)
	}
	if given("port") {
		t.Error("given reported -port for a flag left at its default")
	}
}

// A port somebody else holds still fails, and only after the retry window: the
// point of the retry is that it waits, so a test that just asserts the error
// would pass with the retry deleted.
func TestListenLoopback_NamedPortRetriesThenFails(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	port := held.Addr().(*net.TCPAddr).Port

	real := listenRetryWindow
	listenRetryWindow = 300 * time.Millisecond
	t.Cleanup(func() { listenRetryWindow = real })

	start := time.Now()
	ln, err := listenLoopback(port)
	if err == nil {
		ln.Close()
		t.Fatal("bound a port another listener holds")
	}
	if waited := time.Since(start); waited < listenRetryWindow {
		t.Fatalf("gave up after %s, want at least the %s window", waited, listenRetryWindow)
	}
}

// Port 0 is the OS picking, so it binds first time and never enters the loop.
func TestListenLoopback_PortZeroBinds(t *testing.T) {
	real := listenRetryWindow
	listenRetryWindow = time.Minute
	t.Cleanup(func() { listenRetryWindow = real })

	start := time.Now()
	ln, err := listenLoopback(0)
	if err != nil {
		t.Fatalf("listenLoopback(0): %v", err)
	}
	defer ln.Close()
	if waited := time.Since(start); waited > 5*time.Second {
		t.Fatalf("took %s, so it went through the retry loop", waited)
	}
}
