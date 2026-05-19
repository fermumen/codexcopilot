package copilot

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCodexAppModelsKeepsOpenAIResponsesModels(t *testing.T) {
	models := []Model{
		{"id": "gpt-5.1-codex", "supported_endpoints": []any{"/v1/responses"}, "model_picker_enabled": true},
		{"id": "claude-sonnet-4.5", "supported_endpoints": []any{"/v1/messages"}, "model_picker_enabled": true},
		{"id": "gpt-5-mini", "supported_endpoints": []any{"/chat/completions"}, "model_picker_enabled": true},
		{"id": "gpt-5-disabled", "supported_endpoints": []any{"/responses"}, "model_picker_enabled": true, "policy": map[string]any{"state": "disabled"}},
	}
	got := CodexAppModels(models)
	if len(got) != 1 {
		t.Fatalf("expected 1 model, got %d", len(got))
	}
	if got[0]["id"] != "gpt-5.1-codex" {
		t.Fatalf("unexpected model %v", got[0]["id"])
	}
}

func TestReasoningEffortsPreservesExplicitXHigh(t *testing.T) {
	model := Model{
		"id": "gpt-5.4",
		"capabilities": map[string]any{
			"supports": map[string]any{
				"reasoning_effort": []any{"low", "medium", "high", "xhigh"},
			},
		},
	}
	got := ReasoningEfforts(model)
	if len(got) != 4 || got[3] != "xhigh" {
		t.Fatalf("expected explicit xhigh support, got %#v", got)
	}
}

func TestReasoningEffortsAddsXHighFallbackForGPT54(t *testing.T) {
	model := Model{
		"id": "gpt-5.4",
		"capabilities": map[string]any{
			"supports": map[string]any{"reasoning": true},
		},
	}
	got := ReasoningEfforts(model)
	if len(got) != 4 || got[3] != "xhigh" {
		t.Fatalf("expected xhigh fallback for gpt-5.4, got %#v", got)
	}
}

func TestFetchModelsFromBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.4","supported_endpoints":["/v1/responses"]}]}`))
	}))
	defer server.Close()

	models, err := FetchModelsFromBaseURL(server.URL + "/v1/")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0]["id"] != "gpt-5.4" {
		t.Fatalf("unexpected models: %#v", models)
	}
}
