package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultUsesCodexHomeEnv(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, "custom-codex")
	t.Setenv("CODEX_HOME", codexHome)

	p := Default()
	if p.CodexDir != codexHome {
		t.Fatalf("CodexDir = %q, want %q", p.CodexDir, codexHome)
	}
	if want := filepath.Join(codexHome, ConfigFile); p.CodexConfig != want {
		t.Fatalf("CodexConfig = %q, want %q", p.CodexConfig, want)
	}
	if want := filepath.Join(codexHome, "codexcopilot-codex-app.config.toml"); p.ProfileConfig != want {
		t.Fatalf("ProfileConfig = %q, want %q", p.ProfileConfig, want)
	}
	if want := filepath.Join(codexHome, CatalogName); p.ModelCatalog != want {
		t.Fatalf("ModelCatalog = %q, want %q", p.ModelCatalog, want)
	}
	if filepath.Dir(p.RestoreFile) == p.StateDir {
		t.Fatalf("RestoreFile = %q, want CODEX_HOME-scoped state below %q", p.RestoreFile, p.StateDir)
	}
	if filepath.Dir(p.BackupDir) == p.StateDir {
		t.Fatalf("BackupDir = %q, want CODEX_HOME-scoped state below %q", p.BackupDir, p.StateDir)
	}
}

func TestDefaultScopesRestoreStateByCodexHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex-a"))
	a := Default()
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex-b"))
	b := Default()
	if a.RestoreFile == b.RestoreFile {
		t.Fatalf("RestoreFile should differ by CODEX_HOME, got %q", a.RestoreFile)
	}
	if a.AuthFile != b.AuthFile {
		t.Fatalf("AuthFile should remain app-scoped, got %q and %q", a.AuthFile, b.AuthFile)
	}
}

func TestDefaultFallsBackToHomeDotCodex(t *testing.T) {
	t.Setenv("CODEX_HOME", "")
	p := Default()
	if filepath.Base(p.CodexDir) != ".codex" {
		t.Fatalf("CodexDir = %q, want a ~/.codex path", p.CodexDir)
	}
}

func TestDefaultUsesClaudeConfigDirEnv(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))
	claudeDir := filepath.Join(root, "custom-claude")
	t.Setenv("CLAUDE_CONFIG_DIR", claudeDir)

	p := Default()
	if p.ClaudeDir != claudeDir {
		t.Fatalf("ClaudeDir = %q, want %q", p.ClaudeDir, claudeDir)
	}
	if want := filepath.Join(claudeDir, ClaudeSettingsFile); p.ClaudeSettings != want {
		t.Fatalf("ClaudeSettings = %q, want %q", p.ClaudeSettings, want)
	}
	if filepath.Dir(p.ClaudeRestoreFile) == p.StateDir || filepath.Dir(p.ClaudeRestoreFile) == filepath.Dir(p.RestoreFile) {
		t.Fatalf("ClaudeRestoreFile = %q, want Claude-scoped state separate from Codex state", p.ClaudeRestoreFile)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "other-claude"))
	if other := Default(); other.ClaudeRestoreFile == p.ClaudeRestoreFile {
		t.Fatalf("ClaudeRestoreFile should differ by CLAUDE_CONFIG_DIR, got %q", p.ClaudeRestoreFile)
	}
}

func TestDefaultFallsBackToHomeDotClaude(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	p := Default()
	if filepath.Base(p.ClaudeDir) != ".claude" {
		t.Fatalf("ClaudeDir = %q, want a ~/.claude path", p.ClaudeDir)
	}
}

func TestDefaultResolvesRelativeClaudeConfigDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))
	t.Setenv("CLAUDE_CONFIG_DIR", "claude-rel")
	for _, dir := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(filepath.Join(root, "a")); err != nil {
		t.Fatal(err)
	}
	a := Default()
	if err := os.Chdir(filepath.Join(root, "b")); err != nil {
		t.Fatal(err)
	}
	b := Default()
	if !filepath.IsAbs(a.ClaudeDir) {
		t.Fatalf("expected absolute Claude dir, got %q", a.ClaudeDir)
	}
	if a.ClaudeRestoreFile == b.ClaudeRestoreFile {
		t.Fatalf("relative dirs from different working directories must not share restore state")
	}
}
