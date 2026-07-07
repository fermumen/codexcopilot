package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/fermumen/codexcopilot/internal/auth"
	"github.com/fermumen/codexcopilot/internal/copilot"
	"github.com/fermumen/codexcopilot/internal/paths"
)

func testCommandPaths(root string) paths.Paths {
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

func TestSplitLaunchArgsAllowsFlagsAfterTarget(t *testing.T) {
	flags, target := splitLaunchArgs([]string{"codex-app", "--model", "gpt-5.4"})
	if !reflect.DeepEqual(target, []string{"codex-app"}) {
		t.Fatalf("unexpected target args: %#v", target)
	}
	if !reflect.DeepEqual(flags, []string{"--model", "gpt-5.4"}) {
		t.Fatalf("unexpected flag args: %#v", flags)
	}
}

func TestSplitLaunchArgsAllowsMultiwordTarget(t *testing.T) {
	flags, target := splitLaunchArgs([]string{"--port=11436", "codex", "app"})
	if !reflect.DeepEqual(target, []string{"codex", "app"}) {
		t.Fatalf("unexpected target args: %#v", target)
	}
	if !reflect.DeepEqual(flags, []string{"--port=11436"}) {
		t.Fatalf("unexpected flag args: %#v", flags)
	}
}

func TestRejectOldLaunchFlags(t *testing.T) {
	for _, args := range [][]string{
		{"codex-app", "--server-only"},
		{"codex-app", "--config-only"},
		{"codex-app", "--no-launch"},
		{"codex-app", "--restore"},
	} {
		if err := rejectOldLaunchFlags(args); err == nil {
			t.Fatalf("expected %v to be rejected", args)
		}
	}
}

func TestSplitPassthroughArgs(t *testing.T) {
	toolArgs, codexArgs := splitPassthroughArgs([]string{"--model", "gpt-5.4", "--", "-C", "/work", "hello"})
	if !reflect.DeepEqual(toolArgs, []string{"--model", "gpt-5.4"}) {
		t.Fatalf("unexpected tool args: %#v", toolArgs)
	}
	if !reflect.DeepEqual(codexArgs, []string{"-C", "/work", "hello"}) {
		t.Fatalf("unexpected codex args: %#v", codexArgs)
	}
}

func TestDefaultBaseURLMatchesResponsesServer(t *testing.T) {
	if defaultBaseURL != "http://127.0.0.1:11435/v1/" {
		t.Fatalf("unexpected default base URL %q", defaultBaseURL)
	}
}

func TestProviderPatchUsesBaseURLModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.4","supported_endpoints":["/v1/responses"],"model_picker_enabled":true}]}`))
	}))
	defer server.Close()

	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("CODEX_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))

	if err := commandProvider([]string{"patch", "--base-url", server.URL + "/v1/", "--model", "gpt-5.4"}); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(root, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, `profile = "codexcopilot-codex-app"`) {
		t.Fatalf("config contains legacy profile setting:\n%s", text)
	}
	profilePath := filepath.Join(root, ".codex", "codexcopilot-codex-app.config.toml")
	if _, err := os.Stat(profilePath); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `base_url = "`+server.URL+`/v1/"`) {
		t.Fatalf("config missing proxy base URL:\n%s", text)
	}
}

func TestManagedResponsesServerPatchesAndRestores(t *testing.T) {
	root := t.TempDir()
	p := testCommandPaths(root)
	if err := os.MkdirAll(p.CodexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.CodexConfig, []byte(`model = "gpt-5.5"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	previousWait := waitForServerShutdown
	waitForServerShutdown = func(server *http.Server, errs <-chan error) error {
		data, err := os.ReadFile(p.CodexConfig)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, `model_provider = "codexcopilot-codex-app"`) {
			t.Fatalf("config was not patched while server was running:\n%s", text)
		}
		if _, err := os.Stat(p.ProfileConfig); err != nil {
			t.Fatal(err)
		}
		_ = server.Close()
		err = <-errs
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
	t.Cleanup(func() {
		waitForServerShutdown = previousWait
	})

	models := []copilot.Model{{"id": "gpt-5.4", "supported_endpoints": []any{"/v1/responses"}, "model_picker_enabled": true}}
	if err := runManagedResponsesServer(p, auth.Auth{AccessToken: "test-token"}, models, "gpt-5.4", "127.0.0.1", 0); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(p.CodexConfig)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `model = "gpt-5.5"`) {
		t.Fatalf("previous model was not restored:\n%s", text)
	}
	if strings.Contains(text, "codexcopilot-codex-app") {
		t.Fatalf("managed provider remained after restore:\n%s", text)
	}
	if _, err := os.Stat(p.ProfileConfig); !os.IsNotExist(err) {
		t.Fatalf("expected profile config to be removed, got %v", err)
	}
}

func TestServerServiceUnit(t *testing.T) {
	unit := serverServiceUnit(`/opt/codex copilot/codex%copilot`, "0.0.0.0", 11435, "")
	want := `ExecStart="/opt/codex copilot/codex%%copilot" responses-server --host "0.0.0.0" --port 11435`
	if !strings.Contains(unit, want) {
		t.Fatalf("unit missing ExecStart:\n%s", unit)
	}
	if !strings.Contains(unit, "Restart=on-failure") {
		t.Fatalf("unit missing restart policy:\n%s", unit)
	}
	withModel := serverServiceUnit(`/usr/local/bin/codexcopilot`, "127.0.0.1", 11435, "gpt-5.4")
	if !strings.Contains(withModel, `--model "gpt-5.4"`) {
		t.Fatalf("unit missing model flag:\n%s", withModel)
	}
}

func TestInstallServerServiceWritesAndEnablesUserUnit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("install-server-service is Linux-only")
	}

	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))
	authPath := filepath.Join(root, ".config", "codexcopilot", "auth.json")
	if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authPath, []byte(`{"access_token":"test-token"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var calls []string
	previousRunner := runExternalCommand
	runExternalCommand = func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	t.Cleanup(func() {
		runExternalCommand = previousRunner
	})

	if err := commandInstallServerService([]string{"--binary", "/usr/local/bin/codexcopilot", "--host", "0.0.0.0", "--port", "11436"}); err != nil {
		t.Fatal(err)
	}

	servicePath := filepath.Join(root, ".config", "systemd", "user", "codexcopilot.service")
	data, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `ExecStart="/usr/local/bin/codexcopilot" responses-server --host "0.0.0.0" --port 11436`) {
		t.Fatalf("unexpected unit:\n%s", text)
	}
	expectedCalls := []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable --now codexcopilot.service",
	}
	if !reflect.DeepEqual(calls, expectedCalls) {
		t.Fatalf("unexpected systemctl calls: %#v", calls)
	}
}

func TestInstallServerServiceRejectsBadPort(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("install-server-service is Linux-only")
	}
	err := commandInstallServerService([]string{"--port", "70000"})
	if err == nil || !strings.Contains(err.Error(), "--port") {
		t.Fatalf("expected bad port error, got %v", err)
	}
}

func TestInstallServerServiceRequiresAuth(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("install-server-service is Linux-only")
	}
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))

	err := commandInstallServerService([]string{"--binary", "/usr/local/bin/codexcopilot"})
	if err == nil || !strings.Contains(err.Error(), "auth login") {
		t.Fatalf("expected auth login error, got %v", err)
	}
}
