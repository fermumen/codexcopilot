package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fermumen/codexcopilot/internal/copilot"
	"github.com/fermumen/codexcopilot/internal/paths"
)

func testPaths(root string) paths.Paths {
	codexDir := filepath.Join(root, ".codex")
	stateDir := filepath.Join(root, ".config", "codexcopilot")
	return paths.Paths{
		CodexDir:      codexDir,
		CodexConfig:   filepath.Join(codexDir, "config.toml"),
		ProfileConfig: filepath.Join(codexDir, "codexcopilot-codex-app.config.toml"),
		ModelCatalog:  filepath.Join(codexDir, "codexcopilot-models.json"),
		StateDir:      stateDir,
		AuthFile:      filepath.Join(stateDir, "auth.json"),
		RestoreFile:   filepath.Join(stateDir, "codex-app-restore.json"),
		BackupDir:     filepath.Join(stateDir, "backup"),
	}
}

func TestNormalizeProviderBaseURL(t *testing.T) {
	cases := map[string]string{
		"http://127.0.0.1:11435":     "http://127.0.0.1:11435/v1/",
		"http://127.0.0.1:11435/":    "http://127.0.0.1:11435/v1/",
		"http://127.0.0.1:11435/v1":  "http://127.0.0.1:11435/v1/",
		"http://127.0.0.1:11435/v1/": "http://127.0.0.1:11435/v1/",
	}
	for input, want := range cases {
		if got := NormalizeProviderBaseURL(input); got != want {
			t.Fatalf("NormalizeProviderBaseURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestConfigureWritesCodexCopilotProviderNames(t *testing.T) {
	p := testPaths(t.TempDir())
	models := []copilot.Model{{"id": "gpt-5.4", "supported_endpoints": []any{"/v1/responses"}}}
	if err := Configure(p, "gpt-5.4", models, "http://127.0.0.1:11435/v1/"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p.CodexConfig)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, `profile = "codexcopilot-codex-app"`) || strings.Contains(text, `[profiles.codexcopilot-codex-app]`) {
		t.Fatalf("config contains legacy profile settings:\n%s", text)
	}
	for _, want := range []string{
		`model_provider = "codexcopilot-codex-app"`,
		`[model_providers.codexcopilot-codex-app]`,
		`model_catalog_json = "` + p.ModelCatalog + `"`,
		`base_url = "http://127.0.0.1:11435/v1/"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("config missing %q:\n%s", want, text)
		}
	}
	profileData, err := os.ReadFile(p.ProfileConfig)
	if err != nil {
		t.Fatal(err)
	}
	profileText := string(profileData)
	for _, want := range []string{
		`model = "gpt-5.4"`,
		`model_provider = "codexcopilot-codex-app"`,
		`[model_providers.codexcopilot-codex-app]`,
		`base_url = "http://127.0.0.1:11435/v1/"`,
	} {
		if !strings.Contains(profileText, want) {
			t.Fatalf("profile config missing %q:\n%s", want, profileText)
		}
	}
	if _, err := os.Stat(p.ModelCatalog); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreRemovesGeneratedProfileConfig(t *testing.T) {
	p := testPaths(t.TempDir())
	models := []copilot.Model{{"id": "gpt-5.4", "supported_endpoints": []any{"/v1/responses"}}}
	if err := Configure(p, "gpt-5.4", models, "http://127.0.0.1:11435/v1/"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.ProfileConfig); err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(p)
	if err != nil {
		t.Fatal(err)
	}
	if !restored {
		t.Fatal("expected restore to report changes")
	}
	if _, err := os.Stat(p.ProfileConfig); !os.IsNotExist(err) {
		t.Fatalf("expected profile config to be removed, got %v", err)
	}
}
