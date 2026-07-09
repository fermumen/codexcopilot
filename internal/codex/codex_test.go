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
		`image_generation = false`,
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
		`image_generation = false`,
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

func TestRestoreRevertsGeneratedImageGenerationFeature(t *testing.T) {
	p := testPaths(t.TempDir())
	models := []copilot.Model{{"id": "gpt-5.4", "supported_endpoints": []any{"/v1/responses"}}}
	initial := strings.Join([]string{
		`model = "gpt-5.5"`,
		``,
		`[features]`,
		`multi_agent = true`,
		``,
	}, "\n")
	if err := os.MkdirAll(filepath.Dir(p.CodexConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.CodexConfig, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Configure(p, "gpt-5.4", models, "http://127.0.0.1:11435/v1/"); err != nil {
		t.Fatal(err)
	}
	configured, err := os.ReadFile(p.CodexConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configured), `image_generation = false`) {
		t.Fatalf("configured config missing image_generation override:\n%s", configured)
	}
	if _, err := Restore(p); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(p.CodexConfig)
	if err != nil {
		t.Fatal(err)
	}
	text := string(restored)
	if strings.Contains(text, `image_generation = false`) {
		t.Fatalf("restore should remove generated image_generation override:\n%s", text)
	}
	if !strings.Contains(text, `multi_agent = true`) {
		t.Fatalf("restore should preserve unrelated feature settings:\n%s", text)
	}
}

func TestRestorePreservesExistingImageGenerationFeature(t *testing.T) {
	p := testPaths(t.TempDir())
	models := []copilot.Model{{"id": "gpt-5.4", "supported_endpoints": []any{"/v1/responses"}}}
	initial := strings.Join([]string{
		`[features]`,
		`image_generation = true`,
		``,
	}, "\n")
	if err := os.MkdirAll(filepath.Dir(p.CodexConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.CodexConfig, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Configure(p, "gpt-5.4", models, "http://127.0.0.1:11435/v1/"); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(p); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(p.CodexConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(restored), `image_generation = true`) {
		t.Fatalf("restore should preserve existing image_generation setting:\n%s", restored)
	}
}

func TestConfigureCollapsesDuplicateImageGenerationFeature(t *testing.T) {
	p := testPaths(t.TempDir())
	models := []copilot.Model{{"id": "gpt-5.4", "supported_endpoints": []any{"/v1/responses"}}}
	initial := strings.Join([]string{
		`[features]`,
		`image_generation = true`,
		`image_generation = false`,
		`multi_agent = true`,
		``,
	}, "\n")
	if err := os.MkdirAll(filepath.Dir(p.CodexConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.CodexConfig, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Configure(p, "gpt-5.4", models, "http://127.0.0.1:11435/v1/"); err != nil {
		t.Fatal(err)
	}
	configured, err := os.ReadFile(p.CodexConfig)
	if err != nil {
		t.Fatal(err)
	}
	text := string(configured)
	if got := strings.Count(text, `image_generation = false`); got != 1 {
		t.Fatalf("expected one image_generation override, got %d:\n%s", got, text)
	}
	if !strings.Contains(text, `multi_agent = true`) {
		t.Fatalf("configure should preserve unrelated feature settings:\n%s", text)
	}
}

func TestConfigurePreservesFollowingSections(t *testing.T) {
	p := testPaths(t.TempDir())
	models := []copilot.Model{{"id": "gpt-5.4", "supported_endpoints": []any{"/v1/responses"}}}
	initial := strings.Join([]string{
		`[features]`,
		`js_repl = false`,
		``,
		`[desktop]`,
		`conversationDetailMode = "STEPS_COMMANDS"`,
		``,
	}, "\n")
	if err := os.MkdirAll(filepath.Dir(p.CodexConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.CodexConfig, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Configure(p, "gpt-5.4", models, "http://127.0.0.1:11435/v1/"); err != nil {
		t.Fatal(err)
	}
	configured, err := os.ReadFile(p.CodexConfig)
	if err != nil {
		t.Fatal(err)
	}
	text := string(configured)
	if !strings.Contains(text, `[desktop]`) || !strings.Contains(text, `conversationDetailMode = "STEPS_COMMANDS"`) {
		t.Fatalf("configure should preserve following sections:\n%s", text)
	}
	if got := strings.Count(text, `image_generation = false`); got != 1 {
		t.Fatalf("expected one image_generation override, got %d:\n%s", got, text)
	}
}

func TestConfigureMigratesLegacyRestoreStateForActivePatch(t *testing.T) {
	p := testPaths(t.TempDir())
	p.RestoreFile = filepath.Join(p.StateDir, "codex-scoped", "codex-app-restore.json")
	p.BackupDir = filepath.Join(filepath.Dir(p.RestoreFile), "backup")
	models := []copilot.Model{{"id": "gpt-5.4", "supported_endpoints": []any{"/v1/responses"}}}
	patched := strings.Join([]string{
		`model = "gpt-5.4"`,
		`model_provider = "codexcopilot-codex-app"`,
		`model_catalog_json = "` + p.ModelCatalog + `"`,
		``,
		`[features]`,
		`image_generation = false`,
		``,
	}, "\n")
	legacyRestore := filepath.Join(p.StateDir, paths.RestoreFileName)
	legacyData := []byte(`{"root":{"model":{"present":true,"value":"gpt-5.5"},"model_catalog_json":{"present":false},"model_provider":{"present":false},"profile":{"present":false}},"features":{"image_generation":{"present":false}}}` + "\n")
	if err := os.MkdirAll(p.CodexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyRestore), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.CodexConfig, []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyRestore, legacyData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Configure(p, "gpt-5.4", models, "http://127.0.0.1:11435/v1/"); err != nil {
		t.Fatal(err)
	}
	scopedData, err := os.ReadFile(p.RestoreFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(scopedData) != string(legacyData) {
		t.Fatalf("scoped restore file did not preserve legacy state:\n%s", scopedData)
	}
}

func TestRestoreUsesLegacyRestoreStateWhenScopedMissing(t *testing.T) {
	p := testPaths(t.TempDir())
	p.RestoreFile = filepath.Join(p.StateDir, "codex-scoped", "codex-app-restore.json")
	p.BackupDir = filepath.Join(filepath.Dir(p.RestoreFile), "backup")
	patched := strings.Join([]string{
		`model = "gpt-5.4"`,
		`model_provider = "codexcopilot-codex-app"`,
		`model_catalog_json = "` + p.ModelCatalog + `"`,
		``,
		`[features]`,
		`image_generation = false`,
		``,
		`[model_providers.codexcopilot-codex-app]`,
		`name = "GitHub Copilot"`,
		`base_url = "http://127.0.0.1:11435/v1/"`,
		`wire_api = "responses"`,
		``,
	}, "\n")
	legacyRestore := filepath.Join(p.StateDir, paths.RestoreFileName)
	legacyData := []byte(`{"root":{"model":{"present":true,"value":"gpt-5.5"},"model_catalog_json":{"present":false},"model_provider":{"present":false},"profile":{"present":false}},"features":{"image_generation":{"present":true,"raw":"true"}}}` + "\n")
	if err := os.MkdirAll(p.CodexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyRestore), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.CodexConfig, []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ProfileConfig, []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ModelCatalog, []byte(`{"models":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyRestore, legacyData, 0o600); err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(p)
	if err != nil {
		t.Fatal(err)
	}
	if !restored {
		t.Fatal("expected legacy restore state to be applied")
	}
	data, err := os.ReadFile(p.CodexConfig)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `model = "gpt-5.5"`) || !strings.Contains(text, `image_generation = true`) {
		t.Fatalf("legacy restore state was not applied:\n%s", text)
	}
	if strings.Contains(text, ProviderName) {
		t.Fatalf("provider settings should be removed:\n%s", text)
	}
	if _, err := os.Stat(legacyRestore); !os.IsNotExist(err) {
		t.Fatalf("expected legacy restore file to be removed, got %v", err)
	}
	if _, err := os.Stat(p.ProfileConfig); !os.IsNotExist(err) {
		t.Fatalf("expected profile config to be removed, got %v", err)
	}
	if _, err := os.Stat(p.ModelCatalog); !os.IsNotExist(err) {
		t.Fatalf("expected model catalog to be removed, got %v", err)
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
