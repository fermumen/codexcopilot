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
}

func TestDefaultFallsBackToHomeDotCodex(t *testing.T) {
	t.Setenv("CODEX_HOME", "")
	p := Default()
	if filepath.Base(p.CodexDir) != ".codex" {
		t.Fatalf("CodexDir = %q, want a ~/.codex path", p.CodexDir)
	}
}
