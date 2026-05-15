from __future__ import annotations

import json
import unittest

from ghc_launch_codex.proxy import _initiator


def body(payload: object) -> bytes:
    return json.dumps(payload).encode("utf-8")


class InitiatorTests(unittest.TestCase):
    def test_responses_user_turn(self) -> None:
        self.assertEqual(
            _initiator(
                body({"input": [{"role": "user", "content": [{"type": "input_text", "text": "hi"}]}]}),
                "/v1/responses",
            ),
            "user",
        )

    def test_responses_tool_or_assistant_continuation_is_agent(self) -> None:
        self.assertEqual(
            _initiator(
                body({"input": [{"role": "tool", "content": [{"type": "tool_result", "text": "ok"}]}]}),
                "/v1/responses",
            ),
            "agent",
        )
        self.assertEqual(
            _initiator(body({"input": [{"type": "function_call_output", "call_id": "1", "output": "ok"}]}), "/v1/responses"),
            "agent",
        )

    def test_chat_completions_user_turn(self) -> None:
        self.assertEqual(
            _initiator(body({"messages": [{"role": "user", "content": "hi"}]}), "/v1/chat/completions"),
            "user",
        )

    def test_invalid_or_unknown_body_defaults_user(self) -> None:
        self.assertEqual(_initiator(b"{", "/v1/responses"), "user")
        self.assertEqual(_initiator(body({}), "/v1/responses"), "user")


if __name__ == "__main__":
    unittest.main()

