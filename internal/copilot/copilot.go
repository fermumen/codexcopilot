package copilot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fermumen/codexcopilot/internal/auth"
)

var DefaultModelHints = []string{
	"gpt-5.1-codex",
	"gpt-5-codex",
	"gpt-5.1",
	"gpt-5",
}

type gptVersion struct {
	Parts []int
	Codex bool
	ID    string
}

type Model map[string]any

func APIBase(a auth.Auth) string {
	if a.EnterpriseURL != "" {
		domain := a.EnterpriseURL
		if parsed, err := url.Parse(a.EnterpriseURL); err == nil && parsed.Hostname() != "" {
			domain = parsed.Hostname()
		}
		domain = strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(domain, "https://"), "http://"), "/")
		return "https://copilot-api." + domain
	}
	return "https://api.githubcopilot.com"
}

func Headers(a auth.Auth, initiator string, vision bool) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+a.AccessToken)
	h.Set("Accept", "application/json")
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", "codexcopilot/0.4.1")
	h.Set("Editor-Version", "codexcopilot/0.4.1")
	h.Set("Editor-Plugin-Version", "codexcopilot/0.4.1")
	h.Set("Copilot-Integration-Id", "vscode-chat")
	h.Set("Openai-Intent", "conversation-edits")
	h.Set("X-Initiator", initiator)
	if vision {
		h.Set("Copilot-Vision-Request", "true")
	}
	return h
}

func requestJSON(method, url string, headers http.Header, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return err
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("http %d from %s: %s", res.StatusCode, url, string(data))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func FetchModels(a auth.Auth) ([]Model, error) {
	var payload struct {
		Data []Model `json:"data"`
	}
	if err := requestJSON("GET", APIBase(a)+"/models", Headers(a, "user", false), nil, &payload); err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(payload.Data))
	for _, model := range payload.Data {
		if id, ok := model["id"].(string); ok && id != "" {
			models = append(models, model)
		}
	}
	return models, nil
}

func FetchModelsFromBaseURL(baseURL string) ([]Model, error) {
	url := strings.TrimRight(baseURL, "/") + "/models"
	var payload struct {
		Data []Model `json:"data"`
	}
	if err := requestJSON("GET", url, http.Header{"Accept": []string{"application/json"}}, nil, &payload); err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(payload.Data))
	for _, model := range payload.Data {
		if id, ok := model["id"].(string); ok && id != "" {
			models = append(models, model)
		}
	}
	return models, nil
}

func stringValue(model Model, key string) string {
	if value, ok := model[key].(string); ok {
		return value
	}
	return ""
}

func boolDefault(model Model, key string, fallback bool) bool {
	if value, ok := model[key].(bool); ok {
		return value
	}
	return fallback
}

func policyEnabled(model Model) bool {
	policy, ok := model["policy"].(map[string]any)
	if !ok {
		return true
	}
	return policy["state"] != "disabled"
}

func supportedEndpoints(model Model) []string {
	var result []string
	if endpoints, ok := model["supported_endpoints"].([]any); ok {
		for _, endpoint := range endpoints {
			if value, ok := endpoint.(string); ok {
				result = append(result, value)
			}
		}
	}
	capabilities, ok := model["capabilities"].(map[string]any)
	if ok {
		if endpoints, ok := capabilities["supported_endpoints"].([]any); ok {
			for _, endpoint := range endpoints {
				if value, ok := endpoint.(string); ok {
					result = append(result, value)
				}
			}
		}
	}
	return result
}

func SupportsResponsesAPI(model Model) bool {
	endpoints := supportedEndpoints(model)
	if len(endpoints) > 0 {
		for _, endpoint := range endpoints {
			endpoint = strings.TrimRight(endpoint, "/")
			if endpoint == "/responses" || endpoint == "/v1/responses" {
				return true
			}
		}
		return false
	}
	id := stringValue(model, "id")
	return strings.HasPrefix(id, "gpt-5") && !strings.HasPrefix(id, "gpt-5-mini")
}

func IsOpenAIModel(model Model) bool {
	id := strings.ToLower(stringValue(model, "id"))
	family := strings.ToLower(stringValue(model, "family"))
	vendor := strings.ToLower(stringValue(model, "vendor") + " " + stringValue(model, "publisher"))
	return strings.HasPrefix(id, "gpt-") ||
		strings.HasPrefix(id, "o1") ||
		strings.HasPrefix(id, "o3") ||
		strings.HasPrefix(id, "o4") ||
		strings.Contains(family, "openai") ||
		strings.Contains(vendor, "openai")
}

func CodexAppModels(models []Model) []Model {
	var selected []Model
	for _, model := range models {
		if !boolDefault(model, "model_picker_enabled", true) {
			continue
		}
		if !policyEnabled(model) {
			continue
		}
		if !IsOpenAIModel(model) {
			continue
		}
		if !SupportsResponsesAPI(model) {
			continue
		}
		selected = append(selected, model)
	}
	return selected
}

func parseGPTVersion(id string) (gptVersion, bool) {
	if !strings.HasPrefix(id, "gpt-") || strings.Contains(id, "-mini") {
		return gptVersion{}, false
	}
	rest := strings.TrimPrefix(id, "gpt-")
	codex := false
	if strings.Contains(rest, "-codex") {
		codex = true
		rest = strings.Replace(rest, "-codex", "", 1)
	}
	versionPart, _, _ := strings.Cut(rest, "-")
	pieces := strings.Split(versionPart, ".")
	parts := make([]int, 0, len(pieces))
	for _, piece := range pieces {
		if piece == "" {
			return gptVersion{}, false
		}
		value, err := strconv.Atoi(piece)
		if err != nil {
			return gptVersion{}, false
		}
		parts = append(parts, value)
	}
	if len(parts) == 0 {
		return gptVersion{}, false
	}
	return gptVersion{Parts: parts, Codex: codex, ID: id}, true
}

func compareGPTVersion(a, b gptVersion) int {
	max := len(a.Parts)
	if len(b.Parts) > max {
		max = len(b.Parts)
	}
	for i := 0; i < max; i++ {
		var av, bv int
		if i < len(a.Parts) {
			av = a.Parts[i]
		}
		if i < len(b.Parts) {
			bv = b.Parts[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}
	if a.Codex && !b.Codex {
		return 1
	}
	if !a.Codex && b.Codex {
		return -1
	}
	return 0
}

func latestGPTModel(models []Model) string {
	var best gptVersion
	found := false
	for _, model := range models {
		id := stringValue(model, "id")
		version, ok := parseGPTVersion(id)
		if !ok {
			continue
		}
		if !found || compareGPTVersion(version, best) > 0 {
			best = version
			found = true
		}
	}
	if !found {
		return ""
	}
	return best.ID
}

func ChooseModel(models []Model, requested string) (string, error) {
	ids := map[string]bool{}
	for _, model := range models {
		if id := stringValue(model, "id"); id != "" {
			ids[id] = true
		}
	}
	if requested != "" {
		if ids[requested] {
			return requested, nil
		}
		return "", fmt.Errorf("model %q was not returned by GitHub Copilot", requested)
	}
	if id := latestGPTModel(models); id != "" {
		return id, nil
	}
	for _, hint := range DefaultModelHints {
		if ids[hint] {
			return hint, nil
		}
	}
	for _, model := range models {
		if id := stringValue(model, "id"); id != "" {
			return id, nil
		}
	}
	return "", fmt.Errorf("GitHub Copilot returned no usable models")
}

func ContextWindow(model Model) int {
	if capabilities, ok := model["capabilities"].(map[string]any); ok {
		if limits, ok := capabilities["limits"].(map[string]any); ok {
			for _, key := range []string{"max_context_window_tokens", "max_prompt_tokens", "context_window"} {
				if value, ok := limits[key].(float64); ok && value > 0 {
					return int(value)
				}
			}
		}
	}
	for _, key := range []string{"max_context_window_tokens", "max_prompt_tokens", "context_window"} {
		if value, ok := model[key].(float64); ok && value > 0 {
			return int(value)
		}
	}
	return 128000
}

func SupportsVision(model Model) bool {
	if value, ok := model["supports_vision"].(bool); ok && value {
		return true
	}
	if capabilities, ok := model["capabilities"].(map[string]any); ok {
		if supports, ok := capabilities["supports"].(map[string]any); ok {
			return supports["vision"] == true || supports["image_input"] == true
		}
	}
	return false
}

func SupportsTools(model Model) bool {
	if value, ok := model["supports_tool_calls"].(bool); ok && value {
		return true
	}
	if capabilities, ok := model["capabilities"].(map[string]any); ok {
		if supports, ok := capabilities["supports"].(map[string]any); ok {
			return supports["tool_calls"] == true
		}
	}
	return false
}

func ReasoningEfforts(model Model) []string {
	id := strings.ToLower(stringValue(model, "id"))
	if capabilities, ok := model["capabilities"].(map[string]any); ok {
		if supports, ok := capabilities["supports"].(map[string]any); ok {
			if efforts, ok := supports["reasoning_effort"].([]any); ok {
				out := make([]string, 0, len(efforts))
				for _, effort := range efforts {
					if value, ok := effort.(string); ok {
						out = append(out, value)
					}
				}
				return out
			}
			if supports["reasoning"] == true {
				return defaultReasoningEfforts(id)
			}
		}
	}
	if strings.HasPrefix(id, "gpt-5") || strings.HasPrefix(id, "o") {
		return defaultReasoningEfforts(id)
	}
	return nil
}

func defaultReasoningEfforts(id string) []string {
	if strings.HasPrefix(id, "gpt-5.4") {
		return []string{"low", "medium", "high", "xhigh"}
	}
	return []string{"low", "medium", "high"}
}
