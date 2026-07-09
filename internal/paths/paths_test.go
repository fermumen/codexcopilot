package paths

import (
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
