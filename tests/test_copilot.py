from __future__ import annotations

import unittest

from ghc_launch_codex.copilot import codex_app_models


class CopilotModelFilterTests(unittest.TestCase):
    def test_codex_app_models_keeps_openai_responses_models(self) -> None:
        models = [
            {
                "id": "gpt-5.1-codex",
                "supported_endpoints": ["/v1/responses"],
                "model_picker_enabled": True,
            },
            {
                "id": "claude-sonnet-4.5",
                "supported_endpoints": ["/v1/messages"],
                "model_picker_enabled": True,
            },
            {
                "id": "gpt-5-mini",
                "supported_endpoints": ["/chat/completions"],
                "model_picker_enabled": True,
            },
            {
                "id": "gpt-5-disabled",
                "supported_endpoints": ["/responses"],
                "model_picker_enabled": True,
                "policy": {"state": "disabled"},
            },
        ]

        self.assertEqual([model["id"] for model in codex_app_models(models)], ["gpt-5.1-codex"])


if __name__ == "__main__":
    unittest.main()

