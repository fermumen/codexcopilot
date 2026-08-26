package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fermumen/codexcopilot/internal/auth"
)

func TestInitiatorResponsesUserTurn(t *testing.T) {
	got := Initiator([]byte(`{"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`))
	if got != "user" {
		t.Fatalf("expected user, got %s", got)
	}
}

func TestInitiatorResponsesContinuation(t *testing.T) {
	cases := []string{
		`{"input":[{"role":"tool","content":[{"type":"tool_result","text":"ok"}]}]}`,
		`{"input":[{"type":"function_call_output","call_id":"1","output":"ok"}]}`,
	}
	for _, tc := range cases {
		got := Initiator([]byte(tc))
		if got != "agent" {
			t.Fatalf("expected agent for %s, got %s", tc, got)
		}
	}
}

func TestInitiatorUnknownDefaultsUser(t *testing.T) {
	for _, tc := range [][]byte{[]byte(`{`), []byte(`{}`)} {
		got := Initiator(tc)
		if got != "user" {
			t.Fatalf("expected user, got %s", got)
		}
	}
}

func TestStripUnsupportedToolsRemovesImageGenerationTools(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","tools":[{"type":"shell"},{"type":"image_generation"},{"type":"image_tool"}],"input":"hi"}`)
	got := stripUnsupportedTools(body)
	var payload struct {
		Tools []struct {
			Type string `json:"type"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Type != "shell" {
		t.Fatalf("unexpected tools after strip: %s", got)
	}
}

func TestStripUnsupportedToolsLeavesSupportedPayloadUnchanged(t *testing.T) {
	body := []byte(`{"tools":[{"type":"shell"}],"input":"hi"}`)
	got := stripUnsupportedTools(body)
	if string(got) != string(body) {
		t.Fatalf("expected unchanged body, got %s", got)
	}
}

func TestStripUnsupportedToolsLeavesInvalidJSONUnchanged(t *testing.T) {
	body := []byte(`{`)
	got := stripUnsupportedTools(body)
	if string(got) != string(body) {
		t.Fatalf("expected unchanged body, got %s", got)
	}
}

func TestInitiatorAnthropicToolResultContinuation(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":"ok"}]}]}`)
	if got := InitiatorForPath(body, "/v1/messages"); got != "agent" {
		t.Fatalf("expected agent, got %s", got)
	}
}

func TestInitiatorAnthropicUserTurn(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":"ok"},{"type":"text","text":"and now do this"}]}]}`)
	if got := InitiatorForPath(body, "/v1/messages"); got != "user" {
		t.Fatalf("expected user, got %s", got)
	}
}

func TestBodyHasAnthropicImageInToolResult(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"abc"}}]}]}]}`)
	if !bodyHasImage(body) {
		t.Fatal("expected Anthropic image to be detected")
	}
}

func TestStripAnthropicUnsupportedToolFields(t *testing.T) {
	body := []byte(`{"tools":[{"name":"bash","description":"run","input_schema":{"type":"object"},"eager_input_streaming":true}]}`)
	got := stripAnthropicUnsupportedToolFields(body)
	var payload struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload.Tools[0]["eager_input_streaming"]; ok {
		t.Fatalf("eager_input_streaming was not removed: %s", got)
	}
	if payload.Tools[0]["name"] != "bash" {
		t.Fatalf("tool fields were not preserved: %s", got)
	}
}

func TestFilterAnthropicBetas(t *testing.T) {
	got := filterAnthropicBetas("claude-code-20250219, interleaved-thinking-2025-05-14, context-management-2025-06-27, advanced-tool-use-2025-11-20, fine-grained-tool-streaming-2025-05-14, interleaved-thinking")
	want := "interleaved-thinking-2025-05-14,context-management-2025-06-27,advanced-tool-use-2025-11-20"
	if got != want {
		t.Fatalf("filterAnthropicBetas() = %q, want %q", got, want)
	}
}

func TestUpstreamPathPreservesAnthropicMessagesV1(t *testing.T) {
	cases := map[string]string{
		"/v1/messages":                     "/v1/messages",
		"/v1/messages?beta=true":           "/v1/messages?beta=true",
		"/v1/messages/count_tokens":        "/v1/messages/count_tokens",
		"/v1/responses":                    "/responses",
		"/v1/chat/completions?stream=true": "/chat/completions?stream=true",
		"/v1/models":                       "/models",
	}
	for input, want := range cases {
		if got := upstreamPath(input); got != want {
			t.Fatalf("upstreamPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAnthropicMessagesProxyCompatibility(t *testing.T) {
	var upstreamErr error
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/v1/messages?beta=true" {
			upstreamErr = fmt.Errorf("unexpected upstream path %s", r.URL.RequestURI())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer github-token" {
			upstreamErr = fmt.Errorf("unexpected Authorization %q", got)
		}
		if got := r.Header.Get("X-Api-Key"); got != "" {
			upstreamErr = fmt.Errorf("x-api-key leaked upstream: %q", got)
		}
		if got := r.Header.Get("X-Initiator"); got != "agent" {
			upstreamErr = fmt.Errorf("unexpected X-Initiator %q", got)
		}
		if got := r.Header.Get("Anthropic-Version"); got != "2023-06-01" {
			upstreamErr = fmt.Errorf("unexpected Anthropic-Version %q", got)
		}
		if got := r.Header.Get("Anthropic-Beta"); got != "interleaved-thinking-2025-05-14,advanced-tool-use-2025-11-20" {
			upstreamErr = fmt.Errorf("unexpected Anthropic-Beta %q", got)
		}
		if got := r.Header.Get("X-Github-Api-Version"); got != "2026-06-01" {
			upstreamErr = fmt.Errorf("unexpected X-GitHub-Api-Version %q", got)
		}
		if got := r.Header.Get("Copilot-Vision-Request"); got != "true" {
			upstreamErr = fmt.Errorf("unexpected Copilot-Vision-Request %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			upstreamErr = err
		} else if strings.Contains(string(body), "eager_input_streaming") {
			upstreamErr = fmt.Errorf("unsupported eager_input_streaming leaked upstream: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"type":"message","content":[]}`))
	}))
	defer upstream.Close()

	server := New(auth.Auth{AccessToken: "github-token"})
	server.baseURL = upstream.URL
	body := `{"model":"claude-sonnet-4","messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"abc"}}]}]}],"tools":[{"name":"bash","description":"run","input_schema":{"type":"object"},"eager_input_streaming":true}]}`
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/messages?beta=true", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer anthropic-token")
	req.Header.Set("X-Api-Key", "anthropic-key")
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Beta", "claude-code-20250219,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14,advanced-tool-use-2025-11-20")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)
	if upstreamErr != nil {
		t.Fatal(upstreamErr)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected proxy status %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAnthropicCountTokensPreservesPath(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":12}`))
	}))
	defer upstream.Close()

	server := New(auth.Auth{AccessToken: "github-token"})
	server.baseURL = upstream.URL
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/messages/count_tokens", strings.NewReader(`{"model":"claude-sonnet-4","messages":[]}`))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)
	if gotPath != "/v1/messages/count_tokens" {
		t.Fatalf("unexpected upstream path %q", gotPath)
	}
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"input_tokens":12`) {
		t.Fatalf("unexpected proxy response %d: %s", recorder.Code, recorder.Body.String())
	}
}
