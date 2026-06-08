package proxy

import (
	"encoding/json"
	"testing"
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
