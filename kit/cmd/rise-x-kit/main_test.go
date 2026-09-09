package main

import (
	"flag"
	"testing"
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
