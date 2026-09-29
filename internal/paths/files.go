package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// AtomicWrite writes data to a sibling temp file and renames it into place.
// An existing symlink is followed so dotfile-managed configs stay linked.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := writeFresh(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// writeFresh removes any existing file first so it is created with perm (under
// the umask) instead of keeping a leftover file's broader mode.
func writeFresh(path string, data []byte, perm os.FileMode) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, data, perm)
}

// BackupFile copies path into backupDir with a timestamp suffix, keeping the
// five newest backups. A missing source file is not an error. Backups are
// user-only because Codex and Claude Code configs can hold credentials.
func BackupFile(path string, backupDir string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return err
	}
	target := filepath.Join(backupDir, fmt.Sprintf("%s.%d", filepath.Base(path), time.Now().Unix()))
	if err := writeFresh(target, data, 0o600); err != nil {
		return err
	}
	matches, _ := filepath.Glob(filepath.Join(backupDir, filepath.Base(path)+".*"))
	sort.Slice(matches, func(i, j int) bool {
		ii, _ := os.Stat(matches[i])
		jj, _ := os.Stat(matches[j])
		return ii.ModTime().After(jj.ModTime())
	})
	if len(matches) > 5 {
		for _, stale := range matches[5:] {
			_ = os.Remove(stale)
		}
	}
	return nil
}
