package proxy

import "testing"

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
