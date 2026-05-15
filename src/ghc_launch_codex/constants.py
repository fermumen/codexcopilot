from __future__ import annotations

import os

APP_NAME = "ghc-launch-codex"
PROFILE_NAME = "githubcopilot-launch-codex-app"
PROVIDER_NAME = "githubcopilot-launch-codex-app"
CATALOG_FILENAME = "githubcopilot-launch-models.json"

DEFAULT_HOST = "127.0.0.1"
DEFAULT_PORT = 11435

# OpenCode's first-party GitHub Copilot provider uses this public device-flow
# client id. Keep it configurable in case GitHub rotates or scopes it.
DEFAULT_COPILOT_CLIENT_ID = os.environ.get(
    "GHC_COPILOT_CLIENT_ID",
    "Ov23li8tweQw6odWQebz",
)

DEFAULT_MODEL_HINTS = (
    "gpt-5.3-codex",
    "gpt-5.2-codex",
    "gpt-5.1-codex",
    "gpt-5-codex",
    "claude-sonnet-4.5",
    "gpt-5.1",
    "gpt-5",
)

