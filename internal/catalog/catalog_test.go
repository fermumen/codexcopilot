package catalog

import (
	"encoding/json"
	"testing"

	"github.com/fermumen/codexcopilot/internal/copilot"
)

func TestBuildWritesReasoningEffortPresets(t *testing.T) {
	models := []copilot.Model{{
		"id":                  "gpt-5.4",
		"supported_endpoints": []any{"/v1/responses"},
		"capabilities": map[string]any{
			"supports": map[string]any{
				"reasoning_effort": []any{"low", "medium", "high", "xhigh"},
			},
		},
	}}
	data, err := Build(models, "gpt-5.4")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Models []struct {
			Slug                       string   `json:"slug"`
			DisplayName                string   `json:"display_name"`
			ShellType                  string   `json:"shell_type"`
			Visibility                 string   `json:"visibility"`
			SupportedInAPI             bool     `json:"supported_in_api"`
			BaseInstructions           string   `json:"base_instructions"`
			InputModalities            []string `json:"input_modalities"`
			WebSearchToolType          string   `json:"web_search_tool_type"`
			DefaultReasoningLevel      string   `json:"default_reasoning_level"`
			SupportsReasoningSummaries bool     `json:"supports_reasoning_summaries"`
			DefaultReasoningSummary    string   `json:"default_reasoning_summary"`
			SupportedReasoningLevels   []struct {
				Effort      string `json:"effort"`
				Description string `json:"description"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Models) != 1 {
		t.Fatalf("expected one model, got %d", len(payload.Models))
	}
	model := payload.Models[0]
	if model.Slug != "gpt-5.4" {
		t.Fatalf("unexpected slug %q", model.Slug)
	}
	if model.DisplayName != "gpt-5.4" {
		t.Fatalf("unexpected display name %q", model.DisplayName)
	}
	if model.WebSearchToolType != "text" {
		t.Fatalf("unexpected web search tool type %q", model.WebSearchToolType)
	}
	if model.ShellType == "" || model.Visibility == "" || !model.SupportedInAPI {
		t.Fatalf("missing required Codex catalog fields: %#v", model)
	}
	if model.BaseInstructions == "" {
		t.Fatal("expected base instructions")
	}
	if len(model.InputModalities) == 0 || model.InputModalities[0] != "text" {
		t.Fatalf("unexpected input modalities: %#v", model.InputModalities)
	}
	if model.DefaultReasoningLevel != "medium" {
		t.Fatalf("unexpected default reasoning level %q", model.DefaultReasoningLevel)
	}
	if model.SupportsReasoningSummaries {
		t.Fatal("expected reasoning summaries to be disabled for Copilot catalog models")
	}
	if model.DefaultReasoningSummary != "none" {
		t.Fatalf("unexpected default reasoning summary %q", model.DefaultReasoningSummary)
	}
	if len(model.SupportedReasoningLevels) != 4 {
		t.Fatalf("unexpected reasoning preset count %d", len(model.SupportedReasoningLevels))
	}
	if model.SupportedReasoningLevels[0].Effort != "low" || model.SupportedReasoningLevels[0].Description == "" {
		t.Fatalf("unexpected first reasoning preset: %#v", model.SupportedReasoningLevels[0])
	}
	if model.SupportedReasoningLevels[3].Effort != "xhigh" {
		t.Fatalf("unexpected final reasoning preset: %#v", model.SupportedReasoningLevels[3])
	}
}
