package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func tarGz(t *testing.T, member string, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: member, Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func zipped(t *testing.T, member string, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(member)
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// serveRelease publishes archive as the platform asset plus a checksums.txt
// body, and returns the Release pointing at both.
func serveRelease(t *testing.T, goos, goarch string, archive []byte, checksums string) (*Checker, Release) {
	t.Helper()
	assetName, ok := AssetName("v0.1.0", goos, goarch)
	if !ok {
		t.Fatalf("no asset for %s/%s", goos, goarch)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/asset", func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	mux.HandleFunc("/checksums", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(checksums)) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := New("rise-x/rise-x-ai-public-marketplace")
	rel := Release{
		Tag:     "kit-v0.1.0",
		Version: "v0.1.0",
		Assets: map[string]string{
			assetName:       srv.URL + "/asset",
			"checksums.txt": srv.URL + "/checksums",
		},
	}
	return c, rel
}

func sumLine(data []byte, name string) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), name)
}

func TestDownload_TarGz(t *testing.T) {
	payload := []byte("mach-o universal binary")
	archive := tarGz(t, "rise-x-kit", payload)
	assetName, _ := AssetName("v0.1.0", "darwin", "arm64")
	c, rel := serveRelease(t, "darwin", "arm64", archive, sumLine(archive, assetName))

	dir := t.TempDir()
	var lines []string
	newPath, err := c.Download(context.Background(), rel, "darwin", "arm64", dir, func(s string) {
		lines = append(lines, s)
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if filepath.Dir(filepath.Dir(newPath)) != dir || !strings.HasPrefix(filepath.Base(filepath.Dir(newPath)), workPrefix) || filepath.Base(newPath) != "rise-x-kit.new" {
		t.Fatalf("newPath = %q, want <dir>/%s*/rise-x-kit.new", newPath, workPrefix)
	}

	got, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new binary: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("new binary = %q, want %q", got, payload)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(newPath)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("mode = %v, want -rwxr-xr-x", info.Mode().Perm())
		}
	}
	assertOnly(t, filepath.Dir(newPath), "rise-x-kit.new")
	assertOnly(t, dir, filepath.Base(filepath.Dir(newPath)))

	joined := strings.Join(lines, "|")
	for _, want := range []string{"Downloading " + assetName, "Checksum verified", "Unpacked rise-x-kit"} {
		if !strings.Contains(joined, want) {
			t.Errorf("progress %q missing %q", joined, want)
		}
	}
}

func TestDownload_Zip(t *testing.T) {
	payload := []byte("pe32+ binary")
	archive := zipped(t, "rise-x-kit.exe", payload)
	assetName, _ := AssetName("v0.1.0", "windows", "amd64")
	c, rel := serveRelease(t, "windows", "amd64", archive, sumLine(archive, assetName))

	dir := t.TempDir()
	newPath, err := c.Download(context.Background(), rel, "windows", "amd64", dir, nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if filepath.Dir(filepath.Dir(newPath)) != dir || filepath.Base(newPath) != "rise-x-kit.exe.new" {
		t.Fatalf("newPath = %q, want <dir>/%s*/rise-x-kit.exe.new", newPath, workPrefix)
	}
	got, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new binary: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("new binary = %q, want %q", got, payload)
	}
	assertOnly(t, filepath.Dir(newPath), "rise-x-kit.exe.new")
}

func TestDownload_ChecksumMismatch(t *testing.T) {
	archive := tarGz(t, "rise-x-kit", []byte("real"))
	assetName, _ := AssetName("v0.1.0", "darwin", "arm64")
	c, rel := serveRelease(t, "darwin", "arm64", archive, sumLine([]byte("something else"), assetName))

	dir := t.TempDir()
	if _, err := c.Download(context.Background(), rel, "darwin", "arm64", dir, nil); err == nil {
		t.Fatal("expected a checksum mismatch")
	}
	assertOnly(t, dir)
}

func TestDownload_AssetNotInChecksums(t *testing.T) {
	archive := tarGz(t, "rise-x-kit", []byte("real"))
	c, rel := serveRelease(t, "darwin", "arm64", archive, sumLine(archive, "some-other-file.tar.gz"))

	dir := t.TempDir()
	_, err := c.Download(context.Background(), rel, "darwin", "arm64", dir, nil)
	if err == nil || !strings.Contains(err.Error(), "does not list") {
		t.Fatalf("err = %v, want a 'does not list' error", err)
	}
	assertOnly(t, dir)
}

func TestDownload_ArchiveMissingMember(t *testing.T) {
	archive := tarGz(t, "README.md", []byte("not a binary"))
	assetName, _ := AssetName("v0.1.0", "darwin", "arm64")
	c, rel := serveRelease(t, "darwin", "arm64", archive, sumLine(archive, assetName))

	dir := t.TempDir()
	_, err := c.Download(context.Background(), rel, "darwin", "arm64", dir, nil)
	if err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("err = %v, want a 'does not contain' error", err)
	}
	assertOnly(t, dir)
}

func TestDownload_UnsupportedPlatform(t *testing.T) {
	c := New("rise-x/rise-x-ai-public-marketplace")
	_, err := c.Download(context.Background(), Release{Version: "v0.1.0"}, "linux", "amd64", t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "linux/amd64") {
		t.Fatalf("err = %v, want a missing-asset error", err)
	}
}

// assertOnly checks destDir holds exactly the named files.
func assertOnly(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if len(got) != len(want) {
		t.Fatalf("dir holds %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dir holds %v, want %v", got, want)
		}
	}
}

// A run that dies between Download and Apply leaves its work directory
// behind; the next launch's Cleanup removes it along with a Windows .old.
func TestCleanup_RemovesLeftoverWorkDir(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "rise-x-kit")
	if err := os.WriteFile(exe, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	work, err := os.MkdirTemp(dir, workPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "rise-x-kit.new"), []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "unrelated"), 0o755); err != nil {
		t.Fatal(err)
	}
	Cleanup(exe)
	assertOnly(t, dir, "rise-x-kit", "unrelated")
}
