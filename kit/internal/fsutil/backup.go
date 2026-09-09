// Package fsutil holds the small filesystem helpers more than one package
// needs.
package fsutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ErrNotDurable reports that a write landed and only its durability barrier
// did not: the rename succeeded, the directory entry could not be flushed.
// It is a unix-only condition in practice, since SyncDir is a no-op on
// Windows - there the barrier is absent rather than reported, which is the
// weaker of the two guarantees.
// The file on disk holds the new bytes, so a caller must not treat this as
// "nothing happened" - undoing it, or dropping the backup that describes the
// state before it, is the wrong move. Directory fsync is unsupported on some
// network and FUSE mounts, which is where this shows up.
var ErrNotDurable = errors.New("the write landed but the directory entry could not be flushed")

// SyncDir flushes a directory entry. It is a variable rather than a plain
// function so a test can make the flush fail: what happens after the rename
// is the part worth pinning, and no filesystem available to the tests refuses
// this on demand.
var SyncDir = syncDirOS

// BackupPath names the backup copy of path: "<path>.bak-<UTC timestamp>".
// Its granularity is one second, so Backup, not this, is what guarantees a
// distinct file.
func BackupPath(path string) string {
	return fmt.Sprintf("%s.bak-%s", path, time.Now().UTC().Format("20060102T150405Z"))
}

// maxBackupAttempts bounds the search for an unused name. One second holds
// far fewer real backups than this.
const maxBackupAttempts = 100

// Backup writes data beside path as a new file, and returns the name it used.
//
// It creates the backup with O_EXCL and O_NOFOLLOW, so it can neither write
// through a symlink somebody planted at the predictable name nor overwrite an
// earlier backup that shares this second's timestamp: a taken name is retried
// with a counter. The mode is applied explicitly, because O_CREATE's perm
// argument is masked by the umask and would leave a copy of a 0600 file
// readable.
func Backup(path string, data []byte, mode os.FileMode) (string, error) {
	base := BackupPath(path)
	for attempt := 0; attempt < maxBackupAttempts; attempt++ {
		name := base
		if attempt > 0 {
			name = fmt.Sprintf("%s.%d", base, attempt)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|oNoFollow, mode)
		if os.IsExist(err) {
			continue // this second's name is taken, or a symlink sits on it
		}
		if err != nil {
			return "", fmt.Errorf("backup %s: %w", path, err)
		}
		if err := write(f, name, data, mode); err != nil {
			if errors.Is(err, ErrNotDurable) {
				// The copy exists and is complete; only its directory entry
				// is unflushed. The name goes back with the error because
				// that file is exactly what the caller must keep.
				return name, fmt.Errorf("backup %s: %w", path, err)
			}
			return "", fmt.Errorf("backup %s: %w", path, err)
		}
		return name, nil
	}
	return "", fmt.Errorf("backup %s: no free name after %d attempts", path, maxBackupAttempts)
}

// write fills the already-created backup and puts it on disk, removing a
// partial file rather than leaving one that looks like a good copy.
func write(f *os.File, name string, data []byte, mode os.FileMode) error {
	fail := func(err error) error {
		f.Close()
		_ = os.Remove(name)
		return err
	}
	// O_CREATE's perm is masked by the umask, so set the real mode here: the
	// backup of a 0600 secrets file must not be world-readable.
	if err := f.Chmod(mode); err != nil {
		return fail(err)
	}
	if _, err := f.Write(data); err != nil {
		return fail(err)
	}
	// The backup exists to survive a failed write of the original, so it has
	// to reach the disk before that write starts - the name included, which
	// is the directory's entry rather than the file's own bytes.
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		return fail(err)
	}
	// The backup's own bytes are on disk by here, so a failed directory flush
	// is a durability warning about its *name*, not a reason to delete it -
	// fail() would remove a complete copy at the moment the caller needs one.
	// Reported the way WriteAtomic reports the same condition.
	if err := SyncDir(filepath.Dir(name)); err != nil {
		return fmt.Errorf("%w: %v", ErrNotDurable, err)
	}
	return nil
}

// WriteAtomic replaces path with data through a temp file in the same
// directory, so an interrupted write leaves the original intact rather than a
// truncated file. The rename is the only moment path changes.
func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() {
		tmp.Close()
		_ = os.Remove(name) // a no-op once the rename has moved it away
	}()

	// CreateTemp makes the file 0600; the original may be more permissive.
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	// Past the rename the change is installed. Anything that fails from here
	// is a durability warning, not a failed write, and says so.
	if err := SyncDir(dir); err != nil {
		return fmt.Errorf("%w: %v", ErrNotDurable, err)
	}
	return nil
}

// Resolve follows a symlinked path to the real file it names, so a dotfiles
// setup keeps its link and the change lands in the repo the partner tracks:
// WriteAtomic renames over its target, which would otherwise replace the link
// itself. A link whose target does not exist yet still names where the write
// belongs, and a path that is not a link resolves to itself.
func Resolve(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	target, err := os.Readlink(path)
	if err != nil {
		return path
	}
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(filepath.Dir(path), target)
}

// maxReadBytes caps ReadNoFollow. Its callers read small metadata files; a
// larger one is not the file they are looking for.
const maxReadBytes = 1 << 20

// ReadNoFollow reads path, refusing to follow a symlink at its last element.
// It is for files trusted only because of where they sit: following a link
// planted there would report some unrelated file's bytes instead. Both
// platforms refuse; see openNoFollow for how each does it.
func ReadNoFollow(path string) ([]byte, error) {
	f, err := openNoFollow(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxReadBytes))
}
