from __future__ import annotations

from typing import Any
from urllib.parse import urlparse

from .auth import CopilotAuth
from .constants import DEFAULT_MODEL_HINTS
from .http_json import request_json


def _enterprise_domain(enterprise_url: str) -> str:
    parsed = urlparse(enterprise_url)
    return parsed.hostname or enterprise_url.rstrip("/")


def copilot_api_base(auth: CopilotAuth) -> str:
    if auth.enterprise_url:
        return f"https://copilot-api.{_enterprise_domain(auth.enterprise_url)}"
    return "https://api.githubcopilot.com"


def copilot_headers(
    auth: CopilotAuth,
    *,
    initiator: str = "agent",
    vision: bool = False,
) -> dict[str, str]:
    headers = {
        "Authorization": f"Bearer {auth.access_token}",
        "Accept": "application/json",
        "Content-Type": "application/json",
        "User-Agent": "ghc-launch-codex/0.1.0",
        "Editor-Version": "ghc-launch-codex/0.1.0",
        "Editor-Plugin-Version": "ghc-launch-codex/0.1.0",
        "Copilot-Integration-Id": "vscode-chat",
        "Openai-Intent": "conversation-edits",
        "X-Initiator": initiator,
    }
    if vision:
        headers["Copilot-Vision-Request"] = "true"
    return headers


def fetch_models(auth: CopilotAuth) -> list[dict[str, Any]]:
    payload = request_json(
        "GET",
        f"{copilot_api_base(auth)}/models",
        headers=copilot_headers(auth),
    )
    data = payload.get("data", []) if isinstance(payload, dict) else []
    return [item for item in data if isinstance(item, dict) and item.get("id")]


def _policy_enabled(model: dict[str, Any]) -> bool:
    policy = model.get("policy")
    if not isinstance(policy, dict):
        return True
    return policy.get("state") != "disabled"


def _supported_endpoints(model: dict[str, Any]) -> list[str]:
    endpoints = model.get("supported_endpoints")
    if isinstance(endpoints, list):
        return [str(item) for item in endpoints]
    capabilities = model.get("capabilities") if isinstance(model.get("capabilities"), dict) else {}
    endpoints = capabilities.get("supported_endpoints")
    if isinstance(endpoints, list):
        return [str(item) for item in endpoints]
    return []


def supports_responses_api(model: dict[str, Any]) -> bool:
    endpoints = _supported_endpoints(model)
    if endpoints:
        return any(endpoint.rstrip("/") in {"/responses", "/v1/responses"} for endpoint in endpoints)
    model_id = str(model.get("id", ""))
    return model_id.startswith("gpt-5") and not model_id.startswith("gpt-5-mini")


def is_openai_model(model: dict[str, Any]) -> bool:
    model_id = str(model.get("id", "")).lower()
    family = str(model.get("family", "")).lower()
    vendor = str(model.get("vendor", "") or model.get("publisher", "")).lower()
    return (
        model_id.startswith(("gpt-", "o1", "o3", "o4"))
        or "openai" in family
        or "openai" in vendor
    )


def codex_app_models(models: list[dict[str, Any]]) -> list[dict[str, Any]]:
    selected = []
    for model in models:
        if not model.get("model_picker_enabled", True):
            continue
        if not _policy_enabled(model):
            continue
        if not is_openai_model(model):
            continue
        if not supports_responses_api(model):
            continue
        selected.append(model)
    return selected


def choose_model(models: list[dict[str, Any]], requested: str | None = None) -> str:
    ids = [str(model["id"]) for model in models if model.get("id")]
    if requested:
        if requested in ids:
            return requested
        raise ValueError(f"Model {requested!r} was not returned by GitHub Copilot.")
    for hint in DEFAULT_MODEL_HINTS:
        if hint in ids:
            return hint
    for model in models:
        if model.get("model_picker_enabled", True) and model.get("id"):
            return str(model["id"])
    if ids:
        return ids[0]
    raise ValueError("GitHub Copilot returned no usable models.")


def model_context_window(model: dict[str, Any]) -> int:
    capabilities = model.get("capabilities") if isinstance(model.get("capabilities"), dict) else {}
    limits = capabilities.get("limits") if isinstance(capabilities.get("limits"), dict) else {}
    for key in ("max_context_window_tokens", "max_prompt_tokens", "context_window"):
        value = limits.get(key) or model.get(key)
        if isinstance(value, int) and value > 0:
            return value
    return 128000


def model_supports_reasoning(model: dict[str, Any]) -> list[str]:
    capabilities = model.get("capabilities") if isinstance(model.get("capabilities"), dict) else {}
    supports = capabilities.get("supports") if isinstance(capabilities.get("supports"), dict) else {}
    efforts = supports.get("reasoning_effort")
    if isinstance(efforts, list):
        return [str(item) for item in efforts]
    if supports.get("reasoning"):
        return ["low", "medium", "high"]
    return []


def model_supports_vision(model: dict[str, Any]) -> bool:
    capabilities = model.get("capabilities") if isinstance(model.get("capabilities"), dict) else {}
    supports = capabilities.get("supports") if isinstance(capabilities.get("supports"), dict) else {}
    return bool(
        model.get("supports_vision")
        or supports.get("vision")
        or supports.get("image_input")
    )


def model_supports_tools(model: dict[str, Any]) -> bool:
    capabilities = model.get("capabilities") if isinstance(model.get("capabilities"), dict) else {}
    supports = capabilities.get("supports") if isinstance(capabilities.get("supports"), dict) else {}
    return bool(supports.get("tool_calls") or model.get("supports_tool_calls"))
