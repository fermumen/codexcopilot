package catalog

import (
	"encoding/json"

	"github.com/fermumen/codexcopilot/internal/copilot"
)

const BaseInstructions = "You are Codex, a coding agent. Follow the user's instructions, inspect local files before editing, and keep changes narrowly scoped."

func Build(models []copilot.Model, selected string) ([]byte, error) {
	entries := make([]map[string]any, 0, len(models))
	for index, model := range models {
		id, _ := model["id"].(string)
		name, _ := model["name"].(string)
		if name == "" {
			name = id
		}
		efforts := copilot.ReasoningEfforts(model)
		var defaultEffort any
		if len(efforts) > 0 {
			defaultEffort = efforts[0]
			for _, effort := range efforts {
				if effort == "medium" {
					defaultEffort = "medium"
					break
				}
			}
		}
		inputModalities := []string{"text"}
		if copilot.SupportsVision(model) {
			inputModalities = append(inputModalities, "image")
		}
		contextWindow := copilot.ContextWindow(model)
		priority := 1000 - index
		if id == selected {
			priority = 2000
		}
		entry := map[string]any{
			"slug":                             id,
			"display_name":                     name,
			"description":                      "GitHub Copilot model",
			"max_tokens":                       nil,
			"context_window":                   contextWindow,
			"max_context_window":               contextWindow,
			"auto_compact_token_limit":         nil,
			"effective_context_window_percent": 95,
			"default_reasoning_level":          defaultEffort,
			"supported_reasoning_levels":       efforts,
			"supports_reasoning_summaries":     len(efforts) > 0,
			"default_reasoning_summary":        nil,
			"support_verbosity":                false,
			"supports_verbosity":               false,
			"default_verbosity":                nil,
			"supported_in_api":                 true,
			"supports_parallel_tool_calls":     copilot.SupportsTools(model),
			"supports_image_detail_original":   false,
			"supports_search_tool":             false,
			"input_modalities":                 inputModalities,
			"output_modalities":                []string{"text"},
			"shell_type":                       "default",
			"visibility":                       "list",
			"priority":                         priority,
			"base_instructions":                BaseInstructions,
			"model_messages":                   nil,
			"apply_patch_tool_type":            nil,
			"web_search_tool_type":             nil,
			"truncation_policy":                map[string]any{"mode": "bytes", "limit": 10000},
			"experimental_supported_tools":     []any{},
			"additional_speed_tiers":           []any{},
			"availability_nux":                 nil,
			"upgrade":                          nil,
		}
		if len(efforts) > 0 {
			entry["default_reasoning_summary"] = "auto"
		}
		entries = append(entries, entry)
	}
	payload := map[string]any{"models": entries}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
