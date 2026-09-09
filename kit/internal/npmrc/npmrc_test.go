package npmrc

import (
	"os"
	"path/filepath"
	"testing"
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
			// The ADO feed still serves @diana/* packages internal developers
			// need, so ADO auth lines are never touched, whatever host an
			// @rise-x:registry line names.
			name: "ADO auth lines alone, no registry line: nothing changes",
			in: "registry=https://registry.npmjs.org/\n" +
				"; begin auth token\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/Example/npm/registry/:username=rise-x\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/Example/npm/registry/:_password=abcdef\n" +
				"; end auth token\n",
			out: "registry=https://registry.npmjs.org/\n" +
				"; begin auth token\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/Example/npm/registry/:username=rise-x\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/Example/npm/registry/:_password=abcdef\n" +
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
			// six ADO auth lines (still legitimate — @diana/* stays on ADO),
			// and one GHP token. Only the registry line's value changes; every
			// other byte, both sets of auth lines included, is untouched.
			name: "Igor's shape: only the @rise-x:registry line changes",
			in: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://npm.pkg.github.com\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/registry/:username=rise-x\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/registry/:_password=p1\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/registry/:email=e@x.com\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/:username=rise-x\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/:_password=p2\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/:email=e@x.com\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\n",
			out: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://registry.npmjs.org/\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/registry/:username=rise-x\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/registry/:_password=p1\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/registry/:email=e@x.com\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/:username=rise-x\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/:_password=p2\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/:email=e@x.com\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\n",
			rewritten: []string{"@rise-x:registry=https://npm.pkg.github.com"},
		},
		{
			name: "ADO-pointed registry line is rewritten, ADO auth lines untouched",
			in: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://rise-x.pkgs.visualstudio.com\n" +
				"//rise-x.pkgs.visualstudio.com/:_password=abcdef\n",
			out: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://registry.npmjs.org/\n" +
				"//rise-x.pkgs.visualstudio.com/:_password=abcdef\n",
			rewritten: []string{"@rise-x:registry=https://rise-x.pkgs.visualstudio.com"},
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
		"@rise-x:registry=https://rise-x.pkgs.visualstudio.com\n" +
		"//rise-x.pkgs.visualstudio.com/:_password=x\n"
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
		"//rise-x.pkgs.visualstudio.com/:_password=x\n"
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
		"@rise-x:registry=https://rise-x.pkgs.visualstudio.com\n" +
		"//rise-x.pkgs.visualstudio.com/:_password=x\n"
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
	if report.Host != "rise-x.pkgs.visualstudio.com" {
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
	dir := t.TempDir()
	path := filepath.Join(dir, ".npmrc")
	body := "registry=https://registry.npmjs.org/\n" +
		"@rise-x:registry=https://rise-x.pkgs.visualstudio.com\n" +
		"//rise-x.pkgs.visualstudio.com/:_password=x\n"
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
		"//npm.pkg.github.com/:_authToken=ghp_secret":   "//npm.pkg.github.com/:_authToken=…",
		"//rise-x.pkgs.visualstudio.com/:_password=b64": "//rise-x.pkgs.visualstudio.com/:_password=…",
		"@rise-x:registry=https://npm.pkg.github.com":   "@rise-x:registry=…",
		"//host/:key":  "//host/:…",
		"nothing-here": "nothing-here",
	}
	for in, want := range cases {
		if got := mask(in); got != want {
			t.Errorf("mask(%q) = %q, want %q", in, got, want)
		}
	}
}
