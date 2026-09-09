package npmrc

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/fsutil"
)

// Golden input/output pairs, inline (not checked-in files) so git's line-
// ending normalization on checkout can never corrupt the CRLF case.
func TestClean_GoldenCases(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		out       string
		rewritten []string
	}{
		{
			name: "already clean",
			in:   "registry=https://registry.npmjs.org/\nstrict-ssl=false\n",
			out:  "registry=https://registry.npmjs.org/\nstrict-ssl=false\n",
		},
		{
			name: "registry already points at npmjs: GHP auth kept",
			in: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://registry.npmjs.org\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\n",
			out: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://registry.npmjs.org\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\n",
		},
		{
			// The private feed still serves the scoped packages internal developers
			// need, so private-feed auth lines are never touched, whatever host an
			// @rise-x:registry line names.
			name: "private-feed auth lines alone, no registry line: nothing changes",
			in: "registry=https://registry.npmjs.org/\n" +
				"; begin auth token\n" +
				"//packages.example.com/_packaging/Example/npm/registry/:username=rise-x\n" +
				"//packages.example.com/_packaging/Example/npm/registry/:_password=abcdef\n" +
				"; end auth token\n",
			out: "registry=https://registry.npmjs.org/\n" +
				"; begin auth token\n" +
				"//packages.example.com/_packaging/Example/npm/registry/:username=rise-x\n" +
				"//packages.example.com/_packaging/Example/npm/registry/:_password=abcdef\n" +
				"; end auth token\n",
		},
		{
			name: "GHP registry line is rewritten, its auth line is not",
			in: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://npm.pkg.github.com\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\n" +
				"other-line=kept\n",
			out: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://registry.npmjs.org/\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\n" +
				"other-line=kept\n",
			rewritten: []string{"@rise-x:registry=https://npm.pkg.github.com"},
		},
		{
			name: "GHP auth line kept when no registry line pointed there",
			in: "registry=https://registry.npmjs.org/\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\n",
			out: "registry=https://registry.npmjs.org/\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\n",
		},
		{
			// Igor's shape: a leftover @rise-x:registry line pointing at GHP,
			// six private-feed auth lines (still legitimate: that scope stays on the private feed),
			// and one GHP token. Only the registry line's value changes; every
			// other byte, both sets of auth lines included, is untouched.
			name: "Igor's shape: only the @rise-x:registry line changes",
			in: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://npm.pkg.github.com\n" +
				"//packages.example.com/_packaging/Example/npm/registry/:username=rise-x\n" +
				"//packages.example.com/_packaging/Example/npm/registry/:_password=p1\n" +
				"//packages.example.com/_packaging/Example/npm/registry/:email=e@x.com\n" +
				"//packages.example.com/_packaging/Example/npm/:username=rise-x\n" +
				"//packages.example.com/_packaging/Example/npm/:_password=p2\n" +
				"//packages.example.com/_packaging/Example/npm/:email=e@x.com\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\n",
			out: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://registry.npmjs.org/\n" +
				"//packages.example.com/_packaging/Example/npm/registry/:username=rise-x\n" +
				"//packages.example.com/_packaging/Example/npm/registry/:_password=p1\n" +
				"//packages.example.com/_packaging/Example/npm/registry/:email=e@x.com\n" +
				"//packages.example.com/_packaging/Example/npm/:username=rise-x\n" +
				"//packages.example.com/_packaging/Example/npm/:_password=p2\n" +
				"//packages.example.com/_packaging/Example/npm/:email=e@x.com\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\n",
			rewritten: []string{"@rise-x:registry=https://npm.pkg.github.com"},
		},
		{
			name: "private-feed registry line is rewritten, private-feed auth lines untouched",
			in: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://packages.example.com\n" +
				"//packages.example.com/:_password=abcdef\n",
			out: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://registry.npmjs.org/\n" +
				"//packages.example.com/:_password=abcdef\n",
			rewritten: []string{"@rise-x:registry=https://packages.example.com"},
		},
		{
			name: "registry already points at npmjs: nothing changes",
			in: "@rise-x:registry=https://registry.npmjs.org/\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\n",
			out: "@rise-x:registry=https://registry.npmjs.org/\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\n",
		},
		{
			name: "CRLF preserved, no final newline",
			in: "registry=https://registry.npmjs.org/\r\n" +
				"@rise-x:registry=https://npm.pkg.github.com\r\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\r\n" +
				"last-line=kept",
			out: "registry=https://registry.npmjs.org/\r\n" +
				"@rise-x:registry=https://registry.npmjs.org/\r\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\r\n" +
				"last-line=kept",
			rewritten: []string{"@rise-x:registry=https://npm.pkg.github.com"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, rewritten := clean(tc.in)
			if out != tc.out {
				t.Errorf("output mismatch:\n got:  %q\n want: %q", out, tc.out)
			}
			if len(rewritten) != len(tc.rewritten) {
				t.Fatalf("rewritten = %v, want %v", rewritten, tc.rewritten)
			}
			for i := range rewritten {
				if rewritten[i] != tc.rewritten[i] {
					t.Errorf("rewritten[%d] = %q, want %q", i, rewritten[i], tc.rewritten[i])
				}
			}
		})
	}
}

func TestClean_WritesBackupAndFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".npmrc")
	original := "registry=https://registry.npmjs.org/\n" +
		"@rise-x:registry=https://packages.example.com\n" +
		"//packages.example.com/:_password=x\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Clean(path)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if res.Backup == "" {
		t.Fatal("expected a backup path")
	}
	if len(res.Rewritten) != 1 {
		t.Fatalf("Rewritten = %v, want 1 line", res.Rewritten)
	}
	if res.Rewritten[0] != "@rise-x:registry=…" {
		t.Fatalf("Rewritten[0] = %q, want the value masked", res.Rewritten[0])
	}

	backupBody, err := os.ReadFile(res.Backup)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(backupBody) != original {
		t.Fatal("backup does not match original content")
	}

	cleaned, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "registry=https://registry.npmjs.org/\n" +
		"@rise-x:registry=https://registry.npmjs.org/\n" +
		"//packages.example.com/:_password=x\n"
	if string(cleaned) != want {
		t.Fatalf("cleaned file = %q, want %q", cleaned, want)
	}
}

func TestClean_NothingToDo_NoBackupNoRewrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".npmrc")
	original := "registry=https://registry.npmjs.org/\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Clean(path)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if res.Backup != "" || len(res.Rewritten) != 0 {
		t.Fatalf("expected no-op result, got %+v", res)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected only the original file, got %v", entries)
	}
}

func TestClean_MissingFile(t *testing.T) {
	res, err := Clean(filepath.Join(t.TempDir(), ".npmrc"))
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if res.Backup != "" || len(res.Rewritten) != 0 {
		t.Fatalf("expected no-op for missing file, got %+v", res)
	}
}

func TestAnalyze_MatchesClean(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".npmrc")
	original := "registry=https://registry.npmjs.org/\n" +
		"@rise-x:registry=https://packages.example.com\n" +
		"//packages.example.com/:_password=x\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(report.Lines) != 1 {
		t.Fatalf("Analyze lines = %v, want 1", report.Lines)
	}
	// The masked line and the host come from the same scan, so a message built
	// from one always describes the file the other was read from.
	if report.Host != "packages.example.com" {
		t.Fatalf("Analyze host = %q", report.Host)
	}
	if want := "@rise-x:registry=…"; report.Lines[0] != want {
		t.Fatalf("Analyze line = %q, want %q", report.Lines[0], want)
	}
	// Analyze must not touch the file.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != original {
		t.Fatal("Analyze modified the file")
	}
}

func TestAnalyze_Host(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".npmrc")

	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("@rise-x:registry=https://npm.pkg.github.com\n")
	if got, err := Analyze(path); err != nil || got.Host != "npm.pkg.github.com" {
		t.Fatalf("host = %q, %v, want npm.pkg.github.com", got.Host, err)
	}

	// A value with no readable host still needs fixing, and the message needs
	// something to name.
	write("@rise-x:registry=\n")
	if got, err := Analyze(path); err != nil || got.Host != "another registry" || len(got.Lines) != 1 {
		t.Fatalf("report = %+v, %v, want one line at \"another registry\"", got, err)
	}

	write("@rise-x:registry=https://registry.npmjs.org/\n")
	if got, err := Analyze(path); err != nil || got.Host != "" || got.Lines != nil {
		t.Fatalf("report = %+v, %v, want empty", got, err)
	}

	if got, err := Analyze(filepath.Join(dir, "missing")); err != nil || got.Host != "" {
		t.Fatalf("Analyze(missing) = %+v, %v, want empty", got, err)
	}
}

// A registry URL with an explicit port still names the host the message and
// the mask apply to, and an empty value is not npmjs either.
func TestClean_RegistryValueEdges(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		out       string
		rewritten int
	}{
		{
			name:      "explicit port still matches, auth line untouched",
			in:        "@rise-x:registry=https://npm.pkg.github.com:443\n//npm.pkg.github.com/:_authToken=x\n",
			out:       "@rise-x:registry=https://registry.npmjs.org/\n//npm.pkg.github.com/:_authToken=x\n",
			rewritten: 1,
		},
		{
			name:      "empty value is not npmjs, so it is rewritten",
			in:        "@rise-x:registry=\nkeep=1\n",
			out:       "@rise-x:registry=https://registry.npmjs.org/\nkeep=1\n",
			rewritten: 1,
		},
		{
			name:      "npmjs with an explicit port is kept",
			in:        "@rise-x:registry=https://registry.npmjs.org:443\n",
			out:       "@rise-x:registry=https://registry.npmjs.org:443\n",
			rewritten: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, rewritten := clean(tc.in)
			if out != tc.out {
				t.Errorf("output = %q, want %q", out, tc.out)
			}
			if len(rewritten) != tc.rewritten {
				t.Errorf("rewritten = %v, want %d lines", rewritten, tc.rewritten)
			}
		})
	}
}

// The backup holds the pre-fix file, so it must not be created more readable
// than the file it copies.
func TestClean_PreservesFileMode(t *testing.T) {
	requirePOSIXModes(t)
	dir := t.TempDir()
	path := filepath.Join(dir, ".npmrc")
	body := "registry=https://registry.npmjs.org/\n" +
		"@rise-x:registry=https://packages.example.com\n" +
		"//packages.example.com/:_password=x\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := Clean(path)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf(".npmrc mode = %v, want 0600", fi.Mode().Perm())
	}
	bfi, err := os.Stat(res.Backup)
	if err != nil {
		t.Fatal(err)
	}
	if bfi.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode = %v, want 0600", bfi.Mode().Perm())
	}
}

func TestMask(t *testing.T) {
	cases := map[string]string{
		"//npm.pkg.github.com/:_authToken=ghp_secret": "//npm.pkg.github.com/:_authToken=…",
		"//packages.example.com/:_password=b64":       "//packages.example.com/:_password=…",
		"@rise-x:registry=https://npm.pkg.github.com": "@rise-x:registry=…",
		"//host/:key":  "//host/:…",
		"nothing-here": "nothing-here",
	}
	for in, want := range cases {
		if got := mask(in); got != want {
			t.Errorf("mask(%q) = %q, want %q", in, got, want)
		}
	}
}

// A stow or chezmoi partner has ~/.npmrc as a symlink into a tracked repo.
// WriteAtomic renames over its target, so writing the link's own path would
// sever it: the repo would keep the bad line and the next re-link would bring
// it back, while the doctor reported the problem fixed.
func TestClean_KeepsASymlinkedNpmrcLinked(t *testing.T) {
	dir := t.TempDir()
	tracked := filepath.Join(dir, "dotfiles", "npmrc")
	if err := os.MkdirAll(filepath.Dir(tracked), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tracked, []byte("@rise-x:registry=https://pkgs.example/npm/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, ".npmrc")
	if err := os.Symlink(tracked, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	res, err := Clean(link)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rewritten) != 1 {
		t.Fatalf("Rewritten = %v, want the one registry line", res.Rewritten)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Clean replaced the symlink with a plain file")
	}
	got, _ := os.ReadFile(tracked)
	if !strings.Contains(string(got), npmjsRegistryURL) {
		t.Fatalf("tracked file = %q, want the rewritten line", got)
	}
}

// The rewrite landed; only the directory flush after it failed. Reporting
// that as a failed write pointed the partner at a backup and invited them to
// restore it over a correct fix.
func TestClean_LateFlushFailureIsNotAFailedWrite(t *testing.T) {
	real := fsutil.SyncDir
	calls := 0
	fsutil.SyncDir = func(dir string) error {
		calls++
		if calls == 1 { // the backup's own flush
			return real(dir)
		}
		return errors.New("fsync: operation not supported")
	}
	t.Cleanup(func() { fsutil.SyncDir = real })

	dir := t.TempDir()
	path := filepath.Join(dir, ".npmrc")
	body := "@rise-x:registry=https://rise-x.pkgs.example.com/_packaging/Example/npm/registry/\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := Clean(path)
	if err != nil {
		t.Fatalf("err = %v, want the rewrite to count as done", err)
	}
	if len(res.Rewritten) != 1 {
		t.Fatalf("rewritten = %v, want the one line", res.Rewritten)
	}
	got, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !strings.Contains(string(got), "registry.npmjs.org") {
		t.Fatalf(".npmrc = %q, want the rewrite the rename installed", got)
	}
}

// requirePOSIXModes skips a test that asserts exact permission bits. Windows
// models only the read-only flag, so os.FileMode there is 0666 or 0444 and
// these assertions cannot hold; the behaviour they pin is a unix one.
func requirePOSIXModes(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not modelled on Windows")
	}
}

// On a mount where a directory fsync is unsupported, every write went through
// Backup and Backup failed hard, so Clean, Apply and the settings toggle were
// all impossible there. The backup it had already written was left on disk
// and reported as "". Now the copy comes back with the error and the rewrite
// goes ahead.
func TestClean_UnflushableDirectoryStillRewrites(t *testing.T) {
	real := fsutil.SyncDir
	fsutil.SyncDir = func(string) error { return errors.New("fsync: operation not supported") }
	t.Cleanup(func() { fsutil.SyncDir = real })

	dir := t.TempDir()
	path := filepath.Join(dir, ".npmrc")
	body := "@rise-x:registry=https://packages.example.com/_packaging/Example/npm/registry/\n" +
		"//packages.example.com/_packaging/Example/npm/registry/:_authToken=keep-me\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := Clean(path)
	if err != nil {
		t.Fatalf("err = %v, want the rewrite to go ahead", err)
	}
	if res.Backup == "" {
		t.Fatal("no backup reported, but one was written")
	}
	if _, serr := os.Stat(res.Backup); serr != nil {
		t.Fatalf("the reported backup is not on disk: %v", serr)
	}
	got, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !strings.Contains(string(got), "registry.npmjs.org") {
		t.Fatalf(".npmrc = %q, want the rewrite", got)
	}
	if !strings.Contains(string(got), "_authToken=keep-me") {
		t.Fatalf(".npmrc lost its auth line: %q", got)
	}
}
