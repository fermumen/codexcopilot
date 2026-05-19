package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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
	if !strings.Contains(text, `profile = "codexcopilot-codex-app"`) {
		t.Fatalf("config was not patched:\n%s", text)
	}
	if !strings.Contains(text, `base_url = "`+server.URL+`/v1/"`) {
		t.Fatalf("config missing proxy base URL:\n%s", text)
	}
}
