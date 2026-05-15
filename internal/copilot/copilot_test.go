package copilot

import "testing"

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
