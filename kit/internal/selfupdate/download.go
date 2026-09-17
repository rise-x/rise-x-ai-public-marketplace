package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// maxAssetBytes caps one downloaded release asset and one extracted member.
const maxAssetBytes = 64 << 20

const checksumsName = "checksums.txt"

// workPrefix names the per-run directory Download works in.
const workPrefix = ".rise-x-kit-update-"

// Download fetches the release asset for goos/goarch plus checksums.txt,
// verifies the asset's sha256, extracts the single binary member with mode
// 0755, and returns its path. The work happens in a fresh directory under
// destDir, so nothing the install directory already holds is touched;
// destDir should be the running binary's own directory, so the later rename
// stays on one filesystem. The caller removes that directory once Apply has
// moved the binary out of it. onLine, when not nil, receives short progress
// lines. Nothing is left behind on failure.
func (c *Checker) Download(ctx context.Context, rel Release, goos, goarch, destDir string, onLine func(string)) (newPath string, err error) {
	say := func(s string) {
		if onLine != nil {
			onLine(s)
		}
	}

	assetName, ok := AssetName(rel.Version, goos, goarch)
	if !ok {
		return "", fmt.Errorf("no release asset for %s/%s", goos, goarch)
	}
	assetURL, ok := rel.Assets[assetName]
	if !ok {
		return "", fmt.Errorf("release %s has no asset %s", rel.Tag, assetName)
	}
	sumsURL, ok := rel.Assets[checksumsName]
	if !ok {
		return "", fmt.Errorf("release %s has no %s", rel.Tag, checksumsName)
	}

	work, err := os.MkdirTemp(destDir, workPrefix)
	if err != nil {
		return "", fmt.Errorf("prepare the update: %w", err)
	}
	archivePath := filepath.Join(work, assetName)
	sumsPath := filepath.Join(work, checksumsName)
	newPath = filepath.Join(work, BinaryName(goos)+".new")
	defer func() {
		if err != nil {
			os.RemoveAll(work)
			return
		}
		os.Remove(archivePath)
		os.Remove(sumsPath)
	}()

	say("Downloading " + assetName)
	sum, err := c.fetchFile(ctx, assetURL, archivePath)
	if err != nil {
		return "", err
	}
	if _, err = c.fetchFile(ctx, sumsURL, sumsPath); err != nil {
		return "", err
	}

	want, err := checksumFor(sumsPath, assetName)
	if err != nil {
		return "", err
	}
	if sum != want {
		return "", fmt.Errorf("checksum mismatch for %s: got %s, want %s", assetName, sum, want)
	}
	say("Checksum verified")

	member := BinaryName(goos)
	if err = extract(archivePath, member, newPath); err != nil {
		return "", err
	}
	say("Unpacked " + member)
	return newPath, nil
}

// fetchFile streams url into dest and returns the content's sha256 in hex.
func (c *Checker) fetchFile(ctx context.Context, url, dest string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.transferClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Assets come from the download host, where a 403 is an expired signed
	// URL, not the API's rate limit.
	if resp.StatusCode != http.StatusOK {
		return "", &StatusError{URL: url, Code: resp.StatusCode}
	}

	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxAssetBytes+1))
	closeErr := f.Close()
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n > maxAssetBytes {
		return "", fmt.Errorf("%s: larger than %d bytes", url, maxAssetBytes)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// checksumFor reads a `shasum -a 256` listing and returns name's digest.
func checksumFor(sumsPath, name string) (string, error) {
	data, err := os.ReadFile(sumsPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("%s does not list %s", checksumsName, name)
}

// extract writes the member named member from a .tar.gz or .zip archive to
// dest with mode 0755.
func extract(archivePath, member, dest string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, member, dest)
	}
	return extractTarGz(archivePath, member, dest)
}

func extractTarGz(archivePath, member, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("read %s: %w", archivePath, err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", archivePath, err)
		}
		if hdr.Typeflag == tar.TypeReg && path.Base(hdr.Name) == member {
			return writeBinary(dest, tr)
		}
	}
	return fmt.Errorf("%s does not contain %s", filepath.Base(archivePath), member)
}

func extractZip(archivePath, member, dest string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", archivePath, err)
	}
	defer zr.Close()

	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() || path.Base(zf.Name) != member {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return fmt.Errorf("read %s: %w", archivePath, err)
		}
		defer rc.Close()
		return writeBinary(dest, rc)
	}
	return fmt.Errorf("%s does not contain %s", filepath.Base(archivePath), member)
}

func writeBinary(dest string, src io.Reader) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(src, maxAssetBytes+1))
	if err != nil {
		f.Close()
		return err
	}
	if n > maxAssetBytes {
		f.Close()
		return fmt.Errorf("%s: extracted member larger than %d bytes", dest, maxAssetBytes)
	}
	// OpenFile's mode is masked by umask, so set the executable bits outright.
	if err := f.Chmod(0o755); err != nil {
		f.Close()
		return err
	}
	// The bytes have to reach the disk before Apply renames this file over the
	// running binary: the rename is durable long before the content is, so a
	// power loss in that window leaves the new name pointing at a truncated
	// file and the old inode already unlinked. There is no second copy to fall
	// back to on unix, and the app cannot report a failure it cannot start to
	// find. fsutil does the same thing everywhere else it writes.
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
