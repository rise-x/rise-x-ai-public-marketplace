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
		name    string
		in      string
		out     string
		removed []string
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
			// need, so ADO auth lines alone are not a leftover to remove.
			name: "ADO auth lines alone, no registry line: nothing removed",
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
			name: "GHP registry line removes matching auth lines too",
			in: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://npm.pkg.github.com\n" +
				"//npm.pkg.github.com/:_authToken=ghp_xxx\n" +
				"other-line=kept\n",
			out: "registry=https://registry.npmjs.org/\n" +
				"other-line=kept\n",
			removed: []string{
				"@rise-x:registry=https://npm.pkg.github.com",
				"//npm.pkg.github.com/:_authToken=ghp_xxx",
			},
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
			// and one GHP token. Only the registry line and the GHP token go.
			name: "Igor's shape: GHP registry leftover, ADO lines untouched",
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
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/registry/:username=rise-x\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/registry/:_password=p1\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/registry/:email=e@x.com\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/:username=rise-x\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/:_password=p2\n" +
				"//rise-x.pkgs.visualstudio.com/_packaging/diana/npm/:email=e@x.com\n",
			removed: []string{
				"@rise-x:registry=https://npm.pkg.github.com",
				"//npm.pkg.github.com/:_authToken=ghp_xxx",
			},
		},
		{
			name: "registry leftover pointing at ADO removes its auth lines",
			in: "registry=https://registry.npmjs.org/\n" +
				"@rise-x:registry=https://rise-x.pkgs.visualstudio.com\n" +
				"//rise-x.pkgs.visualstudio.com/:_password=abcdef\n",
			out: "registry=https://registry.npmjs.org/\n",
			removed: []string{
				"@rise-x:registry=https://rise-x.pkgs.visualstudio.com",
				"//rise-x.pkgs.visualstudio.com/:_password=abcdef",
			},
		},
		{
			name: "registry already points at npmjs: nothing removed",
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
				"last-line=kept",
			removed: []string{
				"@rise-x:registry=https://npm.pkg.github.com",
				"//npm.pkg.github.com/:_authToken=ghp_xxx",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, removed := clean(tc.in)
			if out != tc.out {
				t.Errorf("output mismatch:\n got:  %q\n want: %q", out, tc.out)
			}
			if len(removed) != len(tc.removed) {
				t.Fatalf("removed = %v, want %v", removed, tc.removed)
			}
			for i := range removed {
				if removed[i] != tc.removed[i] {
					t.Errorf("removed[%d] = %q, want %q", i, removed[i], tc.removed[i])
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
	if len(res.Removed) != 2 {
		t.Fatalf("Removed = %v, want 2 lines", res.Removed)
	}
	// The value never leaves the package: the response says which host and
	// key went, not the credential.
	if res.Removed[1] != "//rise-x.pkgs.visualstudio.com/:_password=…" {
		t.Fatalf("Removed[1] = %q, want the value masked", res.Removed[1])
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
	if string(cleaned) != "registry=https://registry.npmjs.org/\n" {
		t.Fatalf("cleaned file = %q", cleaned)
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
	if res.Backup != "" || len(res.Removed) != 0 {
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
	if res.Backup != "" || len(res.Removed) != 0 {
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

	lines, err := Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("Analyze lines = %v, want 2", lines)
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

// A registry URL with an explicit port still names the host the auth line
// belongs to, and an empty value is not a reason to keep the line.
func TestClean_RegistryValueEdges(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		out     string
		removed int
	}{
		{
			name:    "explicit port still matches the auth line",
			in:      "@rise-x:registry=https://npm.pkg.github.com:443\n//npm.pkg.github.com/:_authToken=x\n",
			out:     "",
			removed: 2,
		},
		{
			name:    "empty value is not npmjs, so it goes",
			in:      "@rise-x:registry=\nkeep=1\n",
			out:     "keep=1\n",
			removed: 1,
		},
		{
			name:    "npmjs with an explicit port is kept",
			in:      "@rise-x:registry=https://registry.npmjs.org:443\n",
			out:     "@rise-x:registry=https://registry.npmjs.org:443\n",
			removed: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, removed := clean(tc.in)
			if out != tc.out {
				t.Errorf("output = %q, want %q", out, tc.out)
			}
			if len(removed) != tc.removed {
				t.Errorf("removed = %v, want %d lines", removed, tc.removed)
			}
		})
	}
}

// The backup holds the very credentials this action removes, so it must not be
// created more readable than the file it copies.
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
