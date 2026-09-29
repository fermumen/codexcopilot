package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/fermumen/codexcopilot/internal/copilot"
	"github.com/fermumen/codexcopilot/internal/paths"
)

func testPaths(root string) paths.Paths {
	claudeDir := filepath.Join(root, ".claude")
	stateDir := filepath.Join(root, ".config", "codexcopilot", "claude-test")
	return paths.Paths{
		ClaudeDir:         claudeDir,
		ClaudeSettings:    filepath.Join(claudeDir, "settings.json"),
		ClaudeRestoreFile: filepath.Join(stateDir, "claude-code-restore.json"),
		ClaudeBackupDir:   filepath.Join(stateDir, "backup"),
	}
}

func messagesModel(id string, name string) copilot.Model {
	return copilot.Model{"id": id, "name": name, "supported_endpoints": []any{"/v1/messages"}, "model_picker_enabled": true}
}

func testModels() []copilot.Model {
	return []copilot.Model{
		messagesModel("claude-opus-4.8", "Claude Opus 4.8"),
		messagesModel("claude-opus-4.6", "Claude Opus 4.6"),
		messagesModel("claude-sonnet-5", "Claude Sonnet 5"),
		messagesModel("claude-haiku-4.5", "Claude Haiku 4.5"),
		{"id": "gpt-5.5", "supported_endpoints": []any{"/responses"}, "model_picker_enabled": true},
	}
}

const azureSettings = `{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "azure-secret",
    "ANTHROPIC_BASE_URL": "https://example.services.ai.azure.com/anthropic/",
    "ANTHROPIC_DEFAULT_FABLE_MODEL": "claude-fable-5-1",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "claude-haiku-4-5",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "claude-opus-5.5[1m]",
    "ANTHROPIC_CUSTOM_HEADERS": "api-key: azure-secret",
    "AZURE_API_KEY": "azure-secret",
    "CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS": "1",
    "CLAUDE_CODE_USE_MANTLE": "1"
  },
  "includeCoAuthoredBy": false,
  "permissions": {
    "defaultMode": "acceptEdits"
  },
  "model": "opus",
  "modelPicker": {
    "options": [
      {
        "model": "opus",
        "label": "Opus 5.5 (1M)",
        "description": "Azure · default"
      }
    ],
    "replaceBuiltInOptions": true
  },
  "hooks": {
    "SessionStart": [
      {
        "matcher": "^(startup|resume)$",
        "hooks": [
          {
            "type": "command",
            "command": "bash '/home/user/.claude/hooks/state.sh' session",
            "timeout": 10
          }
        ]
      }
    ]
  },
  "effortLevel": "high"
}
`

func writeSettingsFile(t *testing.T, p paths.Paths, text string) {
	t.Helper()
	if err := os.MkdirAll(p.ClaudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ClaudeSettings, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readSettingsMap(t *testing.T, p paths.Paths) map[string]any {
	t.Helper()
	data, err := os.ReadFile(p.ClaudeSettings)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("settings are not valid JSON: %v\n%s", err, data)
	}
	return out
}

func topLevelKeys(t *testing.T, p paths.Paths) []string {
	t.Helper()
	data, err := os.ReadFile(p.ClaudeSettings)
	if err != nil {
		t.Fatal(err)
	}
	o, err := parseObject(data)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, m := range o {
		keys = append(keys, m.Key)
	}
	return keys
}

func TestSlotsPickNewestModelPerFamily(t *testing.T) {
	models := append(testModels(),
		messagesModel("claude-opus-4.8-fast", "Claude Opus 4.8 Fast"),
		messagesModel("claude-3.5-sonnet", "Claude 3.5 Sonnet"),
		copilot.Model{"id": "claude-fable-5.1", "supported_endpoints": []any{"/v1/messages"}, "model_picker_enabled": true, "policy": map[string]any{"state": "disabled"}},
		copilot.Model{"id": "claude-sonnet-9", "supported_endpoints": []any{"/chat/completions"}, "model_picker_enabled": true},
	)
	got := Slots(models)
	want := []Slot{
		{Alias: "opus", Model: "claude-opus-4.8", Label: "Claude Opus 4.8"},
		{Alias: "sonnet", Model: "claude-sonnet-5", Label: "Claude Sonnet 5"},
		{Alias: "haiku", Model: "claude-haiku-4.5", Label: "Claude Haiku 4.5"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Slots() = %#v, want %#v", got, want)
	}
}

func TestNormalizeBaseURLDropsV1(t *testing.T) {
	for _, in := range []string{"http://127.0.0.1:11435/v1/", "http://127.0.0.1:11435/v1", "http://127.0.0.1:11435/"} {
		if got := NormalizeBaseURL(in); got != "http://127.0.0.1:11435" {
			t.Fatalf("NormalizeBaseURL(%q) = %q", in, got)
		}
	}
}

func TestConfigurePatchesManagedKeysAndPreservesTheRest(t *testing.T) {
	p := testPaths(t.TempDir())
	writeSettingsFile(t, p, azureSettings)

	selected, err := Configure(p, testModels(), "http://127.0.0.1:11435/v1/", "")
	if err != nil {
		t.Fatal(err)
	}
	if selected != "opus" {
		t.Fatalf("selected = %q, want opus", selected)
	}
	settings := readSettingsMap(t, p)
	env := settings["env"].(map[string]any)
	wantEnv := map[string]any{
		"ANTHROPIC_AUTH_TOKEN":                   AuthTokenPlaceholder,
		"ANTHROPIC_BASE_URL":                     "http://127.0.0.1:11435",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":           "claude-opus-4.8",
		"ANTHROPIC_DEFAULT_SONNET_MODEL":         "claude-sonnet-5",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":          "claude-haiku-4.5",
		"AZURE_API_KEY":                          "azure-secret",
		"CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS": "1",
	}
	if !reflect.DeepEqual(env, wantEnv) {
		t.Fatalf("env = %#v, want %#v", env, wantEnv)
	}
	if settings["model"] != "opus" {
		t.Fatalf("model = %#v", settings["model"])
	}
	picker := settings["modelPicker"].(map[string]any)
	options := picker["options"].([]any)
	if len(options) != 3 || picker["replaceBuiltInOptions"] != true {
		t.Fatalf("unexpected modelPicker: %#v", picker)
	}
	first := options[0].(map[string]any)
	if first["model"] != "opus" || first["label"] != "Claude Opus 4.8" || !strings.Contains(first["description"].(string), "GitHub Copilot") {
		t.Fatalf("unexpected first picker option: %#v", first)
	}
	if settings["effortLevel"] != "high" || settings["includeCoAuthoredBy"] != false {
		t.Fatalf("unmanaged settings changed: %#v", settings)
	}
	if _, ok := settings["hooks"].(map[string]any)["SessionStart"]; !ok {
		t.Fatalf("hooks were not preserved: %#v", settings["hooks"])
	}
	wantKeys := []string{"env", "includeCoAuthoredBy", "permissions", "model", "modelPicker", "hooks", "effortLevel"}
	if got := topLevelKeys(t, p); !reflect.DeepEqual(got, wantKeys) {
		t.Fatalf("top-level key order = %v, want %v", got, wantKeys)
	}
	info, err := os.Stat(p.ClaudeSettings)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("settings mode = %v, want 0600 preserved", info.Mode().Perm())
	}
}

func TestRestoreBringsBackOriginalSettings(t *testing.T) {
	p := testPaths(t.TempDir())
	writeSettingsFile(t, p, azureSettings)
	if _, err := Configure(p, testModels(), "http://127.0.0.1:11435/v1/", ""); err != nil {
		t.Fatal(err)
	}
	// A second configure (service restart after a crash) must keep the
	// original Azure values as the restore target.
	if _, err := Configure(p, testModels(), "http://127.0.0.1:11435/v1/", "sonnet"); err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(p)
	if err != nil {
		t.Fatal(err)
	}
	if !restored {
		t.Fatal("expected restore to report true")
	}
	data, err := os.ReadFile(p.ClaudeSettings)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != azureSettings {
		t.Fatalf("settings were not restored byte-for-byte:\n%s", data)
	}
	if _, err := os.Stat(p.ClaudeRestoreFile); !os.IsNotExist(err) {
		t.Fatalf("expected restore state to be removed, got %v", err)
	}
	restored, err = Restore(p)
	if err != nil || restored {
		t.Fatalf("second restore = %v, %v; want false, nil", restored, err)
	}
}

func TestRestoreKeepsUnrelatedEditsMadeWhilePatched(t *testing.T) {
	p := testPaths(t.TempDir())
	writeSettingsFile(t, p, `{"env":{"FOO":"bar"},"theme":"dark"}`)
	if _, err := Configure(p, testModels(), "http://127.0.0.1:11435/v1/", ""); err != nil {
		t.Fatal(err)
	}
	settings := readSettingsMap(t, p)
	settings["theme"] = "light"
	settings["model"] = "haiku"
	data, _ := json.Marshal(settings)
	writeSettingsFile(t, p, string(data))

	if _, err := Restore(p); err != nil {
		t.Fatal(err)
	}
	got := readSettingsMap(t, p)
	want := map[string]any{"env": map[string]any{"FOO": "bar"}, "theme": "light"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("restored settings = %#v, want %#v", got, want)
	}
}

func TestRestoreRemovesSettingsFileThatDidNotExist(t *testing.T) {
	p := testPaths(t.TempDir())
	if _, err := Configure(p, testModels(), "http://127.0.0.1:11435", "claude-opus-4.6"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(p.ClaudeSettings); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("new settings file mode = %v, want user-only", info.Mode().Perm())
	}
	settings := readSettingsMap(t, p)
	if settings["model"] != "claude-opus-4.6" {
		t.Fatalf("model = %#v, want pinned Copilot id", settings["model"])
	}
	options := settings["modelPicker"].(map[string]any)["options"].([]any)
	if last := options[len(options)-1].(map[string]any); last["model"] != "claude-opus-4.6" {
		t.Fatalf("pinned model missing from picker: %#v", options)
	}
	if _, err := Restore(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.ClaudeSettings); !os.IsNotExist(err) {
		t.Fatalf("expected settings file to be removed, got %v", err)
	}
}

func TestRestoreWithoutStateIsNoop(t *testing.T) {
	p := testPaths(t.TempDir())
	writeSettingsFile(t, p, azureSettings)
	restored, err := Restore(p)
	if err != nil || restored {
		t.Fatalf("Restore() = %v, %v; want false, nil", restored, err)
	}
	data, _ := os.ReadFile(p.ClaudeSettings)
	if string(data) != azureSettings {
		t.Fatalf("settings changed without restore state:\n%s", data)
	}
}

func TestConfigureRejectsInvalidSettingsWithoutWriting(t *testing.T) {
	p := testPaths(t.TempDir())
	writeSettingsFile(t, p, `{"env": {`)
	if _, err := Configure(p, testModels(), "http://127.0.0.1:11435", ""); err == nil {
		t.Fatal("expected invalid JSON error")
	}
	data, _ := os.ReadFile(p.ClaudeSettings)
	if string(data) != `{"env": {` {
		t.Fatalf("invalid settings were rewritten: %s", data)
	}
	if _, err := os.Stat(p.ClaudeRestoreFile); !os.IsNotExist(err) {
		t.Fatalf("restore state should not be written, got %v", err)
	}
}

func TestConfigureRequiresClaudeMessagesModels(t *testing.T) {
	p := testPaths(t.TempDir())
	models := []copilot.Model{{"id": "gpt-5.5", "supported_endpoints": []any{"/responses"}}}
	if _, err := Configure(p, models, "http://127.0.0.1:11435", ""); err == nil {
		t.Fatal("expected error without Claude models")
	}
	if _, err := Configure(p, testModels(), "http://127.0.0.1:11435", "claude-unknown-1"); err == nil {
		t.Fatal("expected error for unknown requested model")
	}
	if _, err := os.Stat(p.ClaudeSettings); !os.IsNotExist(err) {
		t.Fatalf("settings should not be written on error, got %v", err)
	}
}
