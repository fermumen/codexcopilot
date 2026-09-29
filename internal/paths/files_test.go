package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func TestBackupFileIsUserOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	root := t.TempDir()
	source := filepath.Join(root, "settings.json")
	if err := os.WriteFile(source, []byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"secret"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(root, "backup")
	if err := BackupFile(source, backupDir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("backup dir mode = %v, want 0700", info.Mode().Perm())
	}
	matches, _ := filepath.Glob(filepath.Join(backupDir, "settings.json.*"))
	if len(matches) != 1 {
		t.Fatalf("expected one backup, got %v", matches)
	}
	// Overwriting a same-second backup left by an older version must not keep
	// its broader mode.
	if err := os.Chmod(matches[0], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := BackupFile(source, backupDir); err != nil {
		t.Fatal(err)
	}
	matches, _ = filepath.Glob(filepath.Join(backupDir, "settings.json.*"))
	sort.Strings(matches)
	newest := matches[len(matches)-1]
	info, err = os.Stat(newest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("backup %s mode = %v, want 0600", newest, info.Mode().Perm())
	}
}

func TestAtomicWriteEnforcesPermOverLeftoverTemp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path+".tmp", []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestAtomicWriteFollowsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "dotfiles", "settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "settings.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(link, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced by a regular file")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"a":1}` {
		t.Fatalf("target = %s", data)
	}
}
